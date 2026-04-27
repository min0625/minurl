// Copyright 2024 The MinURL Authors

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type responseWriter struct {
	http.ResponseWriter
	wroteHeader  bool
	statusCode   int
	bytesWritten int
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.wroteHeader = true
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n

	return n, err
}

func requestLoggerMiddleware(next http.Handler) http.Handler {
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

		next.ServeHTTP(w, r.WithContext(withLoggerAttrs(r.Context(), attrs)))
	})
}

func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		attrs := append(loggerAttrsFromContext(r.Context()),
			slog.Int("status", rw.statusCode),
			slog.Int("bytes_written", rw.bytesWritten),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
		slog.With(attrsToAny(attrs)...).InfoContext(r.Context(), "access log")
	})
}

type loggerAttrsContextKey struct{}

func withLoggerAttrs(ctx context.Context, attrs []slog.Attr) context.Context {
	return context.WithValue(ctx, loggerAttrsContextKey{}, attrs)
}

func loggerAttrsFromContext(ctx context.Context) []slog.Attr {
	if attrs, ok := ctx.Value(loggerAttrsContextKey{}).([]slog.Attr); ok {
		return attrs
	}

	return nil
}

func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &responseWriter{ResponseWriter: w}

		defer func() {
			if rec := recover(); rec != nil {
				attrs := append(loggerAttrsFromContext(r.Context()),
					slog.String("panic", fmt.Sprint(rec)),
					slog.String("stack", string(debug.Stack())),
				)
				slog.With(attrsToAny(attrs)...).ErrorContext(r.Context(), "panic recovered")

				if !rw.wroteHeader {
					http.Error(
						rw,
						http.StatusText(http.StatusInternalServerError),
						http.StatusInternalServerError,
					)
				}
			}
		}()

		next.ServeHTTP(rw, r)
	})
}

func attrsToAny(attrs []slog.Attr) []any {
	anyAttrs := make([]any, len(attrs))
	for i, attr := range attrs {
		anyAttrs[i] = attr
	}

	return anyAttrs
}

func configureDefaultLogger(format string) {
	opts := &slog.HandlerOptions{}

	var handler slog.Handler

	switch format {
	case logFormatJSON:
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
}
