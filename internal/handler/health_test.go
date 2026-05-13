// Copyright 2024 The MinURL Authors

package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/min0625/minurl/internal/handler"
)

// mockPinger is a test double for handler.DBPinger.
type mockPinger struct {
	err error
}

func (m *mockPinger) PingContext(_ context.Context) error {
	return m.err
}

func TestRegisterHealthHandlersLivezAlwaysUp(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	handler.RegisterHealthHandlers(r, &mockPinger{})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("/livez status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRegisterHealthHandlersReadyzUpWhenDBHealthy(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	handler.RegisterHealthHandlers(r, &mockPinger{})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("/readyz status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRegisterHealthHandlersReadyzDownWhenDBUnhealthy(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	handler.RegisterHealthHandlers(r, &mockPinger{err: context.DeadlineExceeded})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestRegisterHealthHandlersStartupzBehavesLikeReadyz(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	handler.RegisterHealthHandlers(r, &mockPinger{})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/startupz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("/startupz status = %d, want %d", w.Code, http.StatusOK)
	}
}
