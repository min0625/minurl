// Copyright 2024 The MinURL Authors

package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/min0625/minurl/internal/handler"
	"github.com/min0625/minurl/internal/model"
	"github.com/min0625/minurl/internal/service"
)

func TestRegisterGeneratesShortURLSchemaWithRequiredID(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("MinURL API", "0.1.0"))

	handler.Register(api, service.NewShortURLService())

	schema := api.OpenAPI().Components.Schemas.Map()["ShortURL"]
	if schema == nil {
		t.Fatal("ShortURL schema not found")
	}

	if !contains(schema.Required, "id") {
		t.Fatalf("ShortURL required fields = %v, want to include id", schema.Required)
	}

	if api.OpenAPI().Paths["/short-urls"] == nil {
		t.Fatal("POST /short-urls path not found")
	}

	if api.OpenAPI().Paths["/short-urls/{id}"] == nil {
		t.Fatal("GET /short-urls/{id} path not found")
	}
}

func TestRegisterGetShortURLReturns500WhenStorageFails(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("MinURL API", "0.1.0"))
	svc := service.NewShortURLServiceWithStorage(&handlerTestStorage{
		getErr: errors.New("storage unavailable"),
	})
	handler.Register(api, svc)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/short-urls/abc123",
		nil,
	)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
}

type handlerTestStorage struct {
	getErr error
}

func (s *handlerTestStorage) CreateIfAbsent(
	_ context.Context,
	_ model.ShortURL,
) (bool, error) {
	return true, nil
}

func (s *handlerTestStorage) GetByID(
	_ context.Context,
	_ string,
) (model.ShortURL, bool, error) {
	if s.getErr != nil {
		return model.ShortURL{}, false, s.getErr
	}

	return model.ShortURL{}, false, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
