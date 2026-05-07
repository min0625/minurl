// Copyright 2024 The MinURL Authors

package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))
	handler.Register(api, newHandlerTestService(t, testhelpers.NewStorage()))

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/api/v1/urls",
		strings.NewReader(`{"original_url":"invalid-url","id":"bad*id"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
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

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
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
	originalURL := "https://example.com/very/long/url"
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

	if location := resp.Result().Header.Get("Location"); location != originalURL {
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

func TestRegisterRedirectRouteReturnsBadRequestForInvalidID(t *testing.T) {
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

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
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
