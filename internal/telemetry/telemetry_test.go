// Copyright 2026 The MinURL Authors

package telemetry_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/min0625/minurl/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWrapHTTPHandlerNamesSpanByMethodAndPath(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider()
	tp.RegisterSpanProcessor(spanRecorder)

	origProvider := otel.GetTracerProvider()
	origPropagator := otel.GetTextMapPropagator()

	t.Cleanup(func() {
		otel.SetTracerProvider(origProvider)
		otel.SetTextMapPropagator(origPropagator)

		if err := tp.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	cfg := telemetry.Config{Enabled: true, ServiceName: "minurl"}

	h := telemetry.WrapHTTPHandler(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		cfg,
	)

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/urls/abc123",
		nil,
	)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)

	spans := spanRecorder.Ended()
	if len(spans) == 0 {
		t.Fatal("expected at least one ended span")
	}

	if spans[0].Name() != "GET /api/v1/urls/abc123" {
		t.Fatalf("span name = %q, want %q", spans[0].Name(), "GET /api/v1/urls/abc123")
	}
}

// TestWrapHTTPHandlerClosesTheConnectionAfterABodyReadError guards against request desync.
// Once a body read fails, what the client sends next on the connection is the rest of that
// body, so the server must close the connection rather than parse it as a new request.
func TestWrapHTTPHandlerClosesTheConnectionAfterABodyReadError(t *testing.T) {
	t.Parallel()

	h := telemetry.WrapHTTPHandler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Read and close the body the way huma does, then answer the failure.
			_, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()

			w.WriteHeader(http.StatusBadRequest)
		}),
		telemetry.Config{Enabled: true},
	)

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	// "zz" is not a hex chunk size, so reading the body fails.
	_, err = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: minurl\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\n")
	if err != nil {
		t.Fatalf("write request: %v", err)
	}

	br := bufio.NewReader(conn)

	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	_ = resp.Body.Close()

	// The server may already have closed the connection, so a write error is expected.
	_, _ = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: minurl\r\n\r\n")

	resp, err = http.ReadResponse(br, nil)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("second request on the connection answered %d; want the connection closed", resp.StatusCode)
	}

	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		t.Fatal("connection still open after a failed body read; want it closed")
	}
}
