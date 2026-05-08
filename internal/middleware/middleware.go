// Copyright 2024 The MinURL Authors

// Package middleware provides reusable HTTP middleware for the MinURL service.
package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// ResponseWriter wraps http.ResponseWriter to track response status code and bytes written.
type ResponseWriter struct {
	http.ResponseWriter
	WroteHeader  bool
	StatusCode   int
	BytesWritten int
}

// WriteHeader records the status code and delegates to the underlying ResponseWriter.
func (w *ResponseWriter) WriteHeader(statusCode int) {
	w.WroteHeader = true
	w.StatusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write delegates to the underlying ResponseWriter and accumulates bytes written.
func (w *ResponseWriter) Write(b []byte) (int, error) {
	if !w.WroteHeader {
		w.WriteHeader(http.StatusOK)
	}

	n, err := w.ResponseWriter.Write(b)
	w.BytesWritten += n

	return n, err
}

// Flush implements http.Flusher by delegating to the underlying ResponseWriter if it supports it.
func (w *ResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

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

// MaxDecompressedBodySize is the maximum allowed size of a decompressed gzip request body.
// This prevents zip bomb attacks where a small compressed payload expands to an enormous size.
const MaxDecompressedBodySize = 1 << 20 // 1 MiB

// RequestDecompress decompresses request bodies that use Content-Encoding: gzip,
// allowing clients to send compressed payloads.
// Unsupported encoding values result in a 415 Unsupported Media Type response.
// Decompressed bodies are explicitly capped at MaxDecompressedBodySize:
// if the decompressed content exceeds this limit, the handler returns 413 Request Entity Too Large
// before invoking the next handler, preventing both zip bomb attacks and silent partial processing.
func RequestDecompress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding := strings.TrimSpace(r.Header.Get("Content-Encoding"))
		if encoding == "" {
			next.ServeHTTP(w, r)
			return
		}

		if !strings.EqualFold(encoding, "gzip") {
			http.Error(
				w,
				http.StatusText(http.StatusUnsupportedMediaType),
				http.StatusUnsupportedMediaType,
			)

			return
		}

		gr, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		defer func() {
			if closeErr := gr.Close(); closeErr != nil {
				slog.WarnContext(r.Context(), "closing gzip reader", "error", closeErr)
			}
		}()

		// Read at most MaxDecompressedBodySize+1 bytes. Attempting to read one byte
		// beyond the cap lets us distinguish "exactly at limit" from "over limit"
		// without consuming unbounded memory.
		data, readErr := io.ReadAll(io.LimitReader(gr, MaxDecompressedBodySize+1))
		if readErr != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		if int64(len(data)) > MaxDecompressedBodySize {
			http.Error(
				w,
				http.StatusText(http.StatusRequestEntityTooLarge),
				http.StatusRequestEntityTooLarge,
			)

			return
		}

		r2 := r.Clone(r.Context())
		r2.Body = io.NopCloser(bytes.NewReader(data))
		r2.ContentLength = int64(len(data))
		r2.Header.Del("Content-Encoding")
		r2.Header.Del("Content-Length")

		next.ServeHTTP(w, r2)
	})
}

// PanicRecovery catches panics in downstream handlers, logs them, and returns 500.
func PanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &ResponseWriter{ResponseWriter: w}

		defer func() {
			if rec := recover(); rec != nil {
				attrs := append(LoggerAttrsFromContext(r.Context()),
					slog.String("panic", fmt.Sprint(rec)),
					slog.String("stack", string(debug.Stack())),
				)
				slog.With(AttrsToAny(attrs)...).ErrorContext(r.Context(), "panic recovered")

				if !rw.WroteHeader {
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
