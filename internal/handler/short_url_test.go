// Copyright 2024 The MinURL Authors

package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/min0625/minurl/internal/handler"
	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/testhelpers"
)

func TestRegisterGeneratesShortURLSchemaWithRequiredOriginalURL(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))

	handler.Register(api, newHandlerTestService(t, testhelpers.NewStorage()))

	schema := api.OpenAPI().Components.Schemas.Map()["ShortURL"]
	if schema == nil {
		t.Fatal("ShortURL schema not found")
	}

	if !contains(schema.Required, "original_url") {
		t.Fatalf("ShortURL required fields = %v, want to include original_url", schema.Required)
	}

	// OriginalURL.Schema reads MaxOriginalURLLen directly, so this pins that the type's
	// schema actually reaches the published document.
	originalURL := schema.Properties["original_url"]
	if originalURL == nil {
		t.Fatal("original_url property not found")
	}

	if originalURL.MaxLength == nil {
		t.Fatalf("original_url maxLength = nil, want %d", service.MaxOriginalURLLen)
	}

	if *originalURL.MaxLength != service.MaxOriginalURLLen {
		t.Fatalf("original_url maxLength = %d, want %d", *originalURL.MaxLength, service.MaxOriginalURLLen)
	}

	if api.OpenAPI().Paths["/api/v1/urls"] == nil {
		t.Fatal("POST /api/v1/urls path not found")
	}

	if api.OpenAPI().Paths["/api/v1/urls/{id}"] == nil {
		t.Fatal("GET /api/v1/urls/{id} path not found")
	}

	if api.OpenAPI().Paths["/api/v1/urls/{id}:redirect"] == nil {
		t.Fatal("GET /api/v1/urls/{id}:redirect path not found")
	}
}

// TestRegisterPublishesShortIDConstraints pins the maxLength and pattern tags on the body
// field and both {id} path params to the service constants they repeat. The pattern spells
// out Base58Alphabet rather than abbreviating it into ranges, so comparing the two as
// strings catches any character added to or dropped from either side.
func TestRegisterPublishesShortIDConstraints(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))

	handler.Register(api, newHandlerTestService(t, testhelpers.NewStorage()))

	spec := api.OpenAPI()

	shortURL := spec.Components.Schemas.Map()["ShortURL"]
	if shortURL == nil {
		t.Fatal("ShortURL schema not found")
	}

	schemas := map[string]*huma.Schema{
		"ShortURL.id":   shortURL.Properties["id"],
		"get {id}":      pathParamSchema(t, spec, "/api/v1/urls/{id}"),
		"redirect {id}": pathParamSchema(t, spec, "/api/v1/urls/{id}:redirect"),
	}

	for name, schema := range schemas {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if schema == nil {
				t.Fatal("schema not found")
			}

			if schema.MaxLength == nil {
				t.Fatalf("maxLength = nil, want %d", service.MaxShortURLIDLen)
			}

			if *schema.MaxLength != service.MaxShortURLIDLen {
				t.Fatalf("maxLength = %d, want %d", *schema.MaxLength, service.MaxShortURLIDLen)
			}

			wantPattern := "^[" + service.Base58Alphabet + "]+$"
			if schema.Pattern != wantPattern {
				t.Fatalf("pattern = %q, want %q", schema.Pattern, wantPattern)
			}
		})
	}
}

func pathParamSchema(t *testing.T, spec *huma.OpenAPI, path string) *huma.Schema {
	t.Helper()

	item := spec.Paths[path]
	if item == nil || item.Get == nil {
		t.Fatalf("GET %s not found", path)
	}

	for _, param := range item.Get.Parameters {
		if param.Name == "id" {
			return param.Schema
		}
	}

	t.Fatalf("GET %s has no id parameter", path)

	return nil
}

func TestRegisterGetShortURLReturns500WhenStorageFails(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	store := testhelpers.NewStorage().WithGetError(errors.New("storage unavailable"))
	svc := newHandlerTestService(t, store)
	handler.Register(api, svc)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/abc123",
		nil,
	)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
}

func TestRegisterCreateShortURLValidatesRequestBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "https", body: `{"original_url":"https://example.com/x"}`, wantStatus: http.StatusOK},
		{
			// Pairs with "invalid id" below: without an accepted case, an id pattern that
			// rejects everything would still pass every other assertion here.
			name:       "valid custom id",
			body:       `{"original_url":"https://example.com/x","id":"abc123"}`,
			wantStatus: http.StatusOK,
		},
		{name: "not a URL", body: `{"original_url":"invalid-url"}`, wantStatus: http.StatusUnprocessableEntity},
		{
			name:       "javascript scheme",
			body:       `{"original_url":"javascript:alert(1)"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "data scheme",
			body:       `{"original_url":"data:text/html,<h1>hi</h1>"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "file scheme",
			body:       `{"original_url":"file:///etc/passwd"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "ftp scheme",
			body:       `{"original_url":"ftp://example.com/x"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "userinfo",
			body:       `{"original_url":"https://www.example.com@evil.example.org/"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{name: "empty", body: `{"original_url":""}`, wantStatus: http.StatusUnprocessableEntity},
		{
			name:       "invalid id",
			body:       `{"original_url":"https://example.com/x","id":"bad*id"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			// IsValidOriginalURL ignores length, so the maxLength tag is the only thing
			// keeping an over-long URL out of storage. Straddle the limit: a rune-vs-byte
			// or off-by-one slip only shows up at the boundary itself.
			name:       "at max length",
			body:       originalURLBodyOfLen(service.MaxOriginalURLLen),
			wantStatus: http.StatusOK,
		},
		{
			name:       "over max length",
			body:       originalURLBodyOfLen(service.MaxOriginalURLLen + 1),
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			// url.Parse screens control bytes only before the "#", so without the explicit
			// check a CRLF here would be stored and replayed into the Location header.
			name:       "crlf in fragment",
			body:       `{"original_url":"https://example.com/#\r\nSet-Cookie: pwned=1"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := chi.NewRouter()
			api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
			handler.Register(api, newHandlerTestService(t, testhelpers.NewStorage()))

			req := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				"/api/v1/urls",
				strings.NewReader(tt.body),
			)
			req.Header.Set("Content-Type", "application/json")

			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)

			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", resp.Code, tt.wantStatus, resp.Body.String())
			}
		})
	}
}

// TestRegisterCreateShortURLReportsEmptyOriginalURLOnce pins the empty-value guard in
// OriginalURL.Resolve: huma runs a value-typed field's resolver even when the key is
// absent, so without the guard an empty body would report both the schema's minLength
// error and the resolver's "required" error for the same field.
func TestRegisterCreateShortURLReportsEmptyOriginalURLOnce(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	handler.Register(api, newHandlerTestService(t, testhelpers.NewStorage()))

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/api/v1/urls",
		strings.NewReader(`{"original_url":""}`),
	)
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	var body struct {
		Errors []struct {
			Message  string `json:"message"`
			Location string `json:"location"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %s: %v", resp.Body.String(), err)
	}

	if len(body.Errors) != 1 {
		t.Fatalf("errors = %+v, want exactly one", body.Errors)
	}

	if body.Errors[0].Location != "body.original_url" {
		t.Fatalf("error location = %q, want %q", body.Errors[0].Location, "body.original_url")
	}
}

func TestRegisterGetShortURLValidatesRequestPath(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	handler.Register(api, newHandlerTestService(t, testhelpers.NewStorage()))

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/bad*id",
		nil,
	)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnprocessableEntity)
	}
}

// originalURLBodyOfLen returns a create request body whose original_url is exactly n
// characters long, so the maxLength boundary can be straddled without hand-counting.
func originalURLBodyOfLen(n int) string {
	const prefix = "https://example.com/"

	return `{"original_url":"` + prefix + strings.Repeat("a", n-len(prefix)) + `"}`
}

func newHandlerTestService(t *testing.T, store service.ShortURLStorage) *service.ShortURLService {
	t.Helper()

	svc, err := service.NewShortURLServiceWithAllDependencies(store, testhelpers.NewCounter(), nil)
	if err != nil {
		t.Fatalf("NewShortURLServiceWithAllDependencies() error = %v", err)
	}

	return svc
}

func contains(values []string, want string) bool {
	return testhelpers.StringSliceContains(values, want)
}

func TestRegisterRedirectRouteRedirectsToOriginalURL(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	store := testhelpers.NewStorage()
	svc := newHandlerTestService(t, store)

	// Create a short URL in storage
	originalURL := service.OriginalURL("https://example.com/very/long/url")
	entry := service.ShortURL{
		ID:          "abc123",
		OriginalURL: originalURL,
	}

	_, err := svc.Create(context.Background(), entry)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	handler.Register(api, svc)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/abc123:redirect",
		nil,
	)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusFound)
	}

	if location := resp.Result().Header.Get("Location"); location != string(originalURL) {
		t.Fatalf("Location header = %q, want %q", location, originalURL)
	}
}

func TestRegisterRedirectRouteReturns404WhenShortURLNotFound(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	svc := newHandlerTestService(t, testhelpers.NewStorage())
	handler.Register(api, svc)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/xyz789:redirect",
		nil,
	)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}

func TestRegisterRedirectRouteRejectsInvalidID(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	svc := newHandlerTestService(t, testhelpers.NewStorage())
	handler.Register(api, svc)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/bad!id:redirect",
		nil,
	)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnprocessableEntity)
	}
}

func TestRegisterRedirectRouteReturns500WhenStorageFails(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	store := testhelpers.NewStorage().WithGetError(errors.New("storage unavailable"))
	svc := newHandlerTestService(t, store)
	handler.Register(api, svc)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/abc123:redirect",
		nil,
	)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
}

func TestRegisterRedirectRouteReturns404ForExpiredShortURL(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	store := testhelpers.NewStorage()
	svc := newHandlerTestService(t, store)

	past := time.Now().UTC().Add(-time.Hour)

	_, err := svc.Create(context.Background(), service.ShortURL{
		ID:          "expired1",
		OriginalURL: "https://example.com/gone",
		ExpireTime:  &past,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	handler.Register(api, svc)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/expired1:redirect",
		nil,
	)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}

func TestRegisterGetShortURLReturns404ForExpiredShortURL(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	store := testhelpers.NewStorage()
	svc := newHandlerTestService(t, store)

	past := time.Now().UTC().Add(-time.Hour)

	_, err := svc.Create(context.Background(), service.ShortURL{
		ID:          "expired2",
		OriginalURL: "https://example.com/also-gone",
		ExpireTime:  &past,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	handler.Register(api, svc)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/expired2",
		nil,
	)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}

// newAPIWithLegacyEntry registers the routes over a store holding one row written straight
// to storage, bypassing the service so it looks like an entry created before the allowlist
// existed.
func newAPIWithLegacyEntry(t *testing.T, id, originalURL string) http.Handler {
	t.Helper()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	store := testhelpers.NewStorage()

	created, err := store.CreateIfAbsent(context.Background(), service.ShortURL{
		ID:          id,
		OriginalURL: service.OriginalURL(originalURL),
		CreateTime:  time.Now().UTC(),
	})
	if err != nil || !created {
		t.Fatalf("CreateIfAbsent() = %v, %v, want true, nil", created, err)
	}

	handler.Register(api, newHandlerTestService(t, store))

	return r
}

func TestRegisterRedirectRouteReturns404ForNonHTTPStoredURL(t *testing.T) {
	t.Parallel()

	r := newAPIWithLegacyEntry(t, "abc123", "javascript:alert(1)")

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/abc123:redirect",
		nil,
	)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
	}

	if location := resp.Result().Header.Get("Location"); location != "" {
		t.Fatalf("Location header = %q, want empty", location)
	}
}

// TestRegisterGetShortURLReturnsNonHTTPStoredURL pins the asymmetry README documents on
// purpose: :redirect refuses a legacy non-http(s) row, but GET /{id} still hands it back so
// the row can be found and fixed. Nothing else stops that promise from being quietly dropped.
func TestRegisterGetShortURLReturnsNonHTTPStoredURL(t *testing.T) {
	t.Parallel()

	const originalURL = "javascript:alert(1)"

	r := newAPIWithLegacyEntry(t, "abc123", originalURL)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/abc123",
		nil,
	)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}

	var body struct {
		OriginalURL string `json:"original_url"`
	}

	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %s: %v", resp.Body.String(), err)
	}

	if body.OriginalURL != originalURL {
		t.Fatalf("original_url = %q, want %q", body.OriginalURL, originalURL)
	}
}
