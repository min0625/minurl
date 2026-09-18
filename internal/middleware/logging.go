// Copyright 2026 The MinURL Authors

package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// RequestLogger injects per-request log attributes (method, path, remote addr, trace IDs)
// into the request context so downstream middleware and handlers can emit correlated logs.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_addr", r.RemoteAddr),
		}
		if reqID := r.Header.Get("X-Request-Id"); reqID != "" {
			attrs = append(attrs, slog.String("request_id", reqID))
		}

		sc := trace.SpanContextFromContext(r.Context())
		if sc.IsValid() {
			attrs = append(attrs,
				slog.String("trace_id", sc.TraceID().String()),
				slog.String("span_id", sc.SpanID().String()),
			)
		}

		next.ServeHTTP(w, r.WithContext(WithLoggerAttrs(r.Context(), attrs)))
	})
}

// AccessLog logs an access-log entry after each request, including status, bytes, and duration.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &ResponseWriter{ResponseWriter: w, StatusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		attrs := append(LoggerAttrsFromContext(r.Context()),
			slog.Int("status", rw.StatusCode),
			slog.Int("bytes_written", rw.BytesWritten),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
		slog.With(AttrsToAny(attrs)...).InfoContext(r.Context(), "access log")
	})
}

// loggerAttrsContextKey is the context key type for logger attributes.
type loggerAttrsContextKey struct{}

// WithLoggerAttrs stores slog attributes in ctx for retrieval by AccessLog and other middleware.
func WithLoggerAttrs(ctx context.Context, attrs []slog.Attr) context.Context {
	return context.WithValue(ctx, loggerAttrsContextKey{}, attrs)
}

// LoggerAttrsFromContext retrieves slog attributes previously stored by WithLoggerAttrs.
func LoggerAttrsFromContext(ctx context.Context) []slog.Attr {
	if attrs, ok := ctx.Value(loggerAttrsContextKey{}).([]slog.Attr); ok {
		return attrs
	}

	return nil
}

// AttrsToAny converts a slice of slog.Attr to []any for use with slog.With.
func AttrsToAny(attrs []slog.Attr) []any {
	anyAttrs := make([]any, len(attrs))
	for i, attr := range attrs {
		anyAttrs[i] = attr
	}

	return anyAttrs
}

// ConfigureDefaultLogger sets the global slog default handler based on the format string.
// Accepted values: "json" → JSON handler; anything else → text handler.
func ConfigureDefaultLogger(format string) {
	opts := &slog.HandlerOptions{}

	var h slog.Handler

	switch format {
	case "json":
		h = slog.NewJSONHandler(os.Stderr, opts)
	default:
		h = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(h))
}
