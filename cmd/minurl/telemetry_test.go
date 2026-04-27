package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWrapHTTPHandlerWithTelemetryNamesSpanByMethodAndPath(t *testing.T) {
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

	h := wrapHTTPHandlerWithTelemetry(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		appConfig{OTELEnabled: true, OTELServiceName: "minurl"},
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
