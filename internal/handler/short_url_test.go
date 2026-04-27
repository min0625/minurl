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
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/min0625/minurl/internal/handler"
	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/testhelpers"
)

func TestRegisterGeneratesShortURLSchemaWithRequiredOriginalURL(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("MinURL API", "0.1.0"))

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
}

func TestRegisterGetShortURLReturns500WhenStorageFails(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("MinURL API", "0.1.0"))
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
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
}

func TestRegisterCreateShortURLValidatesRequestBody(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("MinURL API", "0.1.0"))
	handler.Register(api, newHandlerTestService(t, testhelpers.NewStorage()))

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/api/v1/urls",
		strings.NewReader(`{"original_url":"invalid-url","id":"bad*id"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

func TestRegisterGetShortURLValidatesRequestPath(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("MinURL API", "0.1.0"))
	handler.Register(api, newHandlerTestService(t, testhelpers.NewStorage()))

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/bad*id",
		nil,
	)

	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

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
