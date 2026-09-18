// Copyright 2026 The MinURL Authors

package middleware_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/min0625/minurl/internal/middleware"
)

func TestAccessLogMiddlewareDefaultsStatusOK(t *testing.T) {
	var buf bytes.Buffer

	orig := slog.Default()

	t.Cleanup(func() {
		slog.SetDefault(orig)
	})

	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{})))

	h := middleware.AccessLog(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)

	if !strings.Contains(buf.String(), "status=200") {
		t.Fatalf("expected access log to contain status=200, got %q", buf.String())
	}
}
