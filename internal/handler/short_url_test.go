// Copyright 2026 The MinURL Authors

package handler_test

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/min0625/minurl/internal/handler"
	"github.com/min0625/minurl/internal/httpserver"
	"github.com/min0625/minurl/internal/middleware"
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

	// Pins that OriginalURL.Schema reaches the published document, and that the absence of
	// a length limit is deliberate rather than an omission someone should "fix".
	originalURL := schema.Properties["original_url"]
	if originalURL == nil {
		t.Fatal("original_url property not found")
	}

	if originalURL.MinLength == nil || *originalURL.MinLength != 1 {
		t.Fatalf("original_url minLength = %v, want 1", originalURL.MinLength)
	}

	if originalURL.MaxLength != nil {
		t.Fatalf("original_url maxLength = %d, want none", *originalURL.MaxLength)
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

	// ShortURL is both the request body and the response, so a nullable id would tell clients
	// a response may carry `"id": null`, which the server never sends. Input stays lenient
	// without it: huma skips null for any non-required property, pinned by "null id" in
	// TestRegisterCreateShortURLValidatesRequestBody.
	if id := shortURL.Properties["id"]; id == nil || id.Nullable {
		t.Error("ShortURL.id nullable = true, want false")
	}

	// The quantifiers differ on purpose. An omitted, null or empty body id all mean "server,
	// pick one", so the body pattern accepts the empty string rather than 422-ing the Go zero
	// value; an empty path segment is rejected by huma before the pattern runs, so it keeps "+".
	schemas := map[string]struct {
		schema     *huma.Schema
		quantifier string
	}{
		"ShortURL.id":   {schema: shortURL.Properties["id"], quantifier: "*"},
		"get {id}":      {schema: pathParamSchema(t, spec, "/api/v1/urls/{id}"), quantifier: "+"},
		"redirect {id}": {schema: pathParamSchema(t, spec, "/api/v1/urls/{id}:redirect"), quantifier: "+"},
	}

	for name, tt := range schemas {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if tt.schema == nil {
				t.Fatal("schema not found")
			}

			if tt.schema.MaxLength == nil {
				t.Fatalf("maxLength = nil, want %d", service.MaxShortURLIDLen)
			}

			if *tt.schema.MaxLength != service.MaxShortURLIDLen {
				t.Fatalf("maxLength = %d, want %d", *tt.schema.MaxLength, service.MaxShortURLIDLen)
			}

			wantPattern := "^[" + service.Base58Alphabet + "]" + tt.quantifier + "$"
			if tt.schema.Pattern != wantPattern {
				t.Fatalf("pattern = %q, want %q", tt.schema.Pattern, wantPattern)
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

// TestRegisterDeclaresEveryReachableErrorStatus drives a request to every error each
// operation returns and checks the published document declares exactly those statuses, plus
// the catch-all `default`. An undeclared status is one the document misdescribes, and a
// declared one with no case here was never shown to be reachable.
func TestRegisterDeclaresEveryReachableErrorStatus(t *testing.T) {
	t.Parallel()

	const createPath = "/api/v1/urls"

	// The router the server runs, middleware included, so a status the middleware answers
	// before huma cannot pass here unnoticed.
	newAPI := func(store *testhelpers.Storage) *chi.Mux {
		r, _ := httpserver.BuildAPI(newHandlerTestService(t, store), "test")

		return r
	}
	storageFails := errors.New("storage unavailable")
	validBody := `{"original_url":"https://example.com/"}`

	tests := []struct {
		name        string
		api         *chi.Mux
		method      string
		target      string
		contentType string
		encoding    string
		body        io.Reader
		wantStatus  int
	}{
		{
			name: "create: missing body", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodPost, target: createPath,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "create: malformed JSON", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodPost, target: createPath, body: strings.NewReader(`{`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "create: corrupt gzip body", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodPost, target: createPath,
			encoding: "gzip", body: strings.NewReader("\x1f\x8b\x08\x00corrupt"),
			wantStatus: http.StatusBadRequest,
		},
		{
			// A recorder has no connection to set a deadline on, so the timeout is faked here;
			// TestBuildAPIAnswersAStalledBody in httpserver stalls a real one.
			name: "create: body read timeout", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodPost, target: createPath, body: iotest.ErrReader(os.ErrDeadlineExceeded),
			wantStatus: http.StatusRequestTimeout,
		},
		{
			name: "create: id taken", api: newAPI(newStoreWithEntry(t, "taken", "https://example.com/")),
			method: http.MethodPost, target: createPath,
			body:       strings.NewReader(`{"original_url":"https://example.com/","id":"taken"}`),
			wantStatus: http.StatusConflict,
		},
		{
			name: "create: body over the limit", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodPost, target: createPath, body: strings.NewReader(originalURLBodyOfLen(1 << 20)),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			// Same status as the body limit, but through ErrOriginalURLTooLong: without this
			// case, dropping that error from create's list would still reach a 413 above.
			name:   "create: original URL too long for storage",
			api:    newAPI(testhelpers.NewStorage().WithCreateError(service.ErrOriginalURLTooLong)),
			method: http.MethodPost, target: createPath, body: strings.NewReader(validBody),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name: "create: unsupported content type", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodPost, target: createPath, contentType: "text/plain", body: strings.NewReader(validBody),
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name: "create: unsupported content encoding", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodPost, target: createPath, encoding: "br", body: strings.NewReader(validBody),
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name: "create: invalid original URL", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodPost, target: createPath,
			body:       strings.NewReader(`{"original_url":"javascript:alert(1)"}`),
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "create: storage fails", api: newAPI(testhelpers.NewStorage().WithCreateError(storageFails)),
			method: http.MethodPost, target: createPath, body: strings.NewReader(validBody),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "get: not found", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodGet, target: "/api/v1/urls/abc123",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "get: malformed id", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodGet, target: "/api/v1/urls/bad*id",
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "get: storage fails", api: newAPI(testhelpers.NewStorage().WithGetError(storageFails)),
			method: http.MethodGet, target: "/api/v1/urls/abc123",
			wantStatus: http.StatusInternalServerError,
		},
		{
			// Known to errorResponses (409) but not listed by get: answering 409 would return a
			// status the document does not publish for this operation.
			name:   "get: error the operation does not list",
			api:    newAPI(testhelpers.NewStorage().WithGetError(service.ErrShortURLIDConflict)),
			method: http.MethodGet, target: "/api/v1/urls/abc123",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "redirect: not found", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodGet, target: "/api/v1/urls/abc123:redirect",
			wantStatus: http.StatusNotFound,
		},
		{
			name: "redirect: malformed id", api: newAPI(testhelpers.NewStorage()),
			method: http.MethodGet, target: "/api/v1/urls/bad*id:redirect",
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "redirect: storage fails", api: newAPI(testhelpers.NewStorage().WithGetError(storageFails)),
			method: http.MethodGet, target: "/api/v1/urls/abc123:redirect",
			wantStatus: http.StatusInternalServerError,
		},
	}

	reached := map[string][]string{}

	for _, tt := range tests {
		req := httptest.NewRequestWithContext(context.Background(), tt.method, tt.target, tt.body)
		if tt.body != nil {
			req.Header.Set("Content-Type", cmp.Or(tt.contentType, "application/json"))
		}

		if tt.encoding != "" {
			req.Header.Set("Content-Encoding", tt.encoding)
		}

		resp := httptest.NewRecorder()
		tt.api.ServeHTTP(resp, req)

		if resp.Code != tt.wantStatus {
			t.Errorf("%s: status = %d, want %d (body %s)", tt.name, resp.Code, tt.wantStatus, resp.Body.String())
		}

		// Every one of these is documented as an ErrorModel, so a plain-text answer is a
		// status the document misdescribes too.
		if got := resp.Header().Get("Content-Type"); got != "application/problem+json" {
			t.Errorf("%s: content type = %q, want application/problem+json", tt.name, got)
		}

		if strings.Contains(resp.Body.String(), storageFails.Error()) {
			t.Errorf("%s: body leaks the storage error: %s", tt.name, resp.Body.String())
		}

		// Keyed by the route the request matched, as the document keys operations by method and
		// path, so a target that lands on another operation cannot vouch for this one. The status
		// is the one the server answered, not the one wanted, so a case that stops returning its
		// status is reported against what it really reaches.
		op := tt.method + " " + tt.api.Find(chi.NewRouteContext(), tt.method, req.URL.Path)
		reached[op] = append(reached[op], strconv.Itoa(resp.Code))
	}

	// The document make gen publishes, not a stand-in built here with its own config.
	for key, op := range testhelpers.Operations(httpserver.BuildOpenAPISpec("test")) {
		want := slices.Concat(reached[key], []string{"default"})
		slices.Sort(want)
		want = slices.Compact(want)

		if got := testhelpers.ErrorResponses(op); !slices.Equal(got, want) {
			t.Errorf("%s: declared error responses = %v, reached = %v", key, got, want)
		}

		delete(reached, key)
	}

	for key := range reached {
		t.Errorf("%s: cases target an operation the document does not have", key)
	}
}

// TestRegisterLogsUnexpectedErrors pins that a 500 never carries the error behind it, which
// can name tables and columns, and that the log line replacing it carries the request's
// attributes, since it is the only record of the failure. It swaps the global slog default
// and so must not call t.Parallel: Go holds every parallel test until the sequential ones
// have finished.
func TestRegisterLogsUnexpectedErrors(t *testing.T) {
	var logs bytes.Buffer

	origLogger, origWriter, origFlags := slog.Default(), log.Writer(), log.Flags()

	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	// SetDefault also points the log package at the new handler, and restoring a default
	// logger does not undo that.
	t.Cleanup(func() {
		slog.SetDefault(origLogger)
		log.SetOutput(origWriter)
		log.SetFlags(origFlags)
	})

	storeErr := errors.New(`relation "short_urls" does not exist`)
	createBody := `{"original_url":"https://example.com/"}`

	tests := []struct {
		name         string
		store        *testhelpers.Storage
		method       string
		target       string
		body         string
		wantStatus   int
		wantLevel    string // empty: nothing is logged
		wantUnlisted bool
	}{
		{
			name: "create", store: testhelpers.NewStorage().WithCreateError(storeErr),
			method: http.MethodPost, target: "/api/v1/urls", body: createBody,
			wantStatus: http.StatusInternalServerError, wantLevel: "ERROR",
		},
		{
			name: "get", store: testhelpers.NewStorage().WithGetError(storeErr),
			method: http.MethodGet, target: "/api/v1/urls/abc123",
			wantStatus: http.StatusInternalServerError, wantLevel: "ERROR",
		},
		{
			name: "redirect", store: testhelpers.NewStorage().WithGetError(storeErr),
			method: http.MethodGet, target: "/api/v1/urls/abc123:redirect",
			wantStatus: http.StatusInternalServerError, wantLevel: "ERROR",
		},
		{
			// The client went away: not a server fault, so logged below ERROR. The response is
			// still a 500, so the access log and the span count it as one.
			name:   "client went away",
			store:  testhelpers.NewStorage().WithGetError(fmt.Errorf("%w: %w", storeErr, context.Canceled)),
			method: http.MethodGet, target: "/api/v1/urls/abc123",
			wantStatus: http.StatusInternalServerError, wantLevel: "WARN",
		},
		{
			// A missing list entry, not a server fault, so the log says which it is.
			name: "error the operation does not list",
			store: testhelpers.NewStorage().
				WithGetError(fmt.Errorf("%w: %w", storeErr, service.ErrShortURLIDConflict)),
			method:       http.MethodGet,
			target:       "/api/v1/urls/abc123",
			wantStatus:   http.StatusInternalServerError,
			wantLevel:    "ERROR",
			wantUnlisted: true,
		},
		{
			name: "listed error", store: testhelpers.NewStorage(),
			method: http.MethodGet, target: "/api/v1/urls/abc123",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs.Reset()

			r, _ := newTestAPI(t, tt.store)

			ctx := middleware.WithLoggerAttrs(context.Background(), []slog.Attr{slog.String("request_id", "req-1")})
			req := httptest.NewRequestWithContext(ctx, tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)

			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", resp.Code, tt.wantStatus, resp.Body.String())
			}

			if strings.Contains(resp.Body.String(), "short_urls") {
				t.Fatalf("body leaks the storage error: %s", resp.Body.String())
			}

			if tt.wantLevel == "" {
				if logs.Len() != 0 {
					t.Fatalf("logged %s, want nothing", logs.String())
				}

				return
			}

			var record struct {
				Level         string `json:"level"`
				Error         string `json:"error"`
				RequestID     string `json:"request_id"`
				UnlistedError bool   `json:"unlisted_error"`
			}

			if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
				t.Fatalf("decode log %q: %v", logs.String(), err)
			}

			if record.Level != tt.wantLevel || record.RequestID != "req-1" ||
				!strings.Contains(record.Error, "short_urls") || record.UnlistedError != tt.wantUnlisted {
				t.Fatalf(
					"log = %s, want level %s with the storage error, request_id req-1 and unlisted_error %v",
					logs.String(),
					tt.wantLevel,
					tt.wantUnlisted,
				)
			}
		})
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
			// A Go client without omitzero/omitempty serializes an unset id as "", so the
			// empty string has to mean the same as an omitted or null one: generate an id.
			name:       "empty id",
			body:       `{"original_url":"https://example.com/x","id":""}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "null id",
			body:       `{"original_url":"https://example.com/x","id":null}`,
			wantStatus: http.StatusOK,
		},
		{
			// There is deliberately no length limit. Pinned so that reintroducing one
			// as a constant, rather than as the planned config value, fails here.
			name:       "very long URL",
			body:       originalURLBodyOfLen(64 * 1024),
			wantStatus: http.StatusOK,
		},
		{
			// huma validates the exact "id" key, but encoding/json matches keys
			// case-insensitively and keeps the last one, so without strict casing the
			// unvalidated "ID" is what gets stored.
			name:       "case-variant id key",
			body:       `{"original_url":"https://example.com/x","id":"abc","ID":"bad*id-way-too-long"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			// Same gap, and OriginalURL.Resolve returns nil for "" because it trusts
			// minLength to have run on this value; without strict casing it had not.
			name:       "case-variant original_url key",
			body:       `{"original_url":"https://example.com/x","ORIGINAL_URL":""}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			// url.Parse screens control bytes only before the "#", so without the explicit
			// check a CRLF here would be stored, and net/http would then rewrite it to a
			// space (HTTP/1.1) or drop the Location header altogether (HTTP/2).
			name:       "crlf in fragment",
			body:       `{"original_url":"https://example.com/#\r\nSet-Cookie: pwned=1"}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			// encoding/json substitutes U+FFFD for the invalid byte while decoding, so by
			// the time any resolver runs the body is valid UTF-8 and a utf8.ValidString
			// check can never fire. Without the RuneError screen this is a 200 that stores
			// and later redirects to a URL the client never sent.
			name:       "invalid utf-8 byte",
			body:       "{\"original_url\":\"https://example.com/a\x80b\"}",
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, _ := newTestAPI(t, testhelpers.NewStorage())

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

	r, _ := newTestAPI(t, testhelpers.NewStorage())

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

	r, _ := newTestAPI(t, testhelpers.NewStorage())

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
// characters long.
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

// newTestAPI registers every short URL route over store and returns the router to drive
// requests against, plus the service the handlers hold — the handlers keep the pointer, so
// a test may seed rows through it after this returns.
func newTestAPI(
	t *testing.T,
	store service.ShortURLStorage,
) (http.Handler, *service.ShortURLService) {
	t.Helper()

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	svc := newHandlerTestService(t, store)
	handler.Register(api, svc)

	return r, svc
}

func contains(values []string, want string) bool {
	return testhelpers.StringSliceContains(values, want)
}

func TestRegisterRedirectRouteRedirectsToOriginalURL(t *testing.T) {
	t.Parallel()

	r, svc := newTestAPI(t, testhelpers.NewStorage())

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

	r, _ := newTestAPI(t, testhelpers.NewStorage())

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

	r, _ := newTestAPI(t, testhelpers.NewStorage())

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

func TestRegisterRedirectRouteReturns404ForExpiredShortURL(t *testing.T) {
	t.Parallel()

	r, svc := newTestAPI(t, testhelpers.NewStorage())

	past := time.Now().UTC().Add(-time.Hour)

	_, err := svc.Create(context.Background(), service.ShortURL{
		ID:          "expired1",
		OriginalURL: "https://example.com/gone",
		ExpireTime:  &past,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

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

	r, svc := newTestAPI(t, testhelpers.NewStorage())

	past := time.Now().UTC().Add(-time.Hour)

	_, err := svc.Create(context.Background(), service.ShortURL{
		ID:          "expired2",
		OriginalURL: "https://example.com/also-gone",
		ExpireTime:  &past,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

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

// TestRegisterCreateShortURLReturns413WhenOriginalURLTooLong pins that a URL the storage
// backend cannot hold is reported as a client error. The API sets no length limit, but
// MySQL's original_url column is TEXT, so the request would otherwise answer 500.
func TestRegisterCreateShortURLReturns413WhenOriginalURLTooLong(t *testing.T) {
	t.Parallel()

	// The store wraps the driver error, which names the table column and the MySQL error
	// code. The handler must not pass it to huma, which would serialize it into the body.
	storeErr := fmt.Errorf(
		"create short url: %w: %w",
		service.ErrOriginalURLTooLong,
		errors.New("Error 1406 (22001): Data too long for column 'original_url' at row 1"),
	)
	r, _ := newTestAPI(t, testhelpers.NewStorage().WithCreateError(storeErr))

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/api/v1/urls",
		strings.NewReader(originalURLBodyOfLen(64*1024)),
	)
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d (body %s)", resp.Code, http.StatusRequestEntityTooLarge, resp.Body.String())
	}

	for _, leak := range []string{"1406", "original_url", "Data too long", "row 1"} {
		if strings.Contains(resp.Body.String(), leak) {
			t.Fatalf("413 body leaks %q to the client: %s", leak, resp.Body.String())
		}
	}
}

// newAPIWithLegacyEntry registers the routes over newStoreWithEntry.
func newAPIWithLegacyEntry(t *testing.T, id, originalURL string) http.Handler {
	t.Helper()

	r, _ := newTestAPI(t, newStoreWithEntry(t, id, originalURL))

	return r
}

// newStoreWithEntry returns a store holding one row written straight to storage, bypassing
// the service so it looks like an entry created before the allowlist existed.
func newStoreWithEntry(t *testing.T, id, originalURL string) *testhelpers.Storage {
	t.Helper()

	store := testhelpers.NewStorage()

	created, err := store.CreateIfAbsent(context.Background(), service.ShortURL{
		ID:          id,
		OriginalURL: service.OriginalURL(originalURL),
		CreateTime:  time.Now().UTC(),
	})
	if err != nil || !created {
		t.Fatalf("CreateIfAbsent() = %v, %v, want true, nil", created, err)
	}

	return store
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
