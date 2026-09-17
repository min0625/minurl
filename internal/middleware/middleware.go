// Copyright 2026 The MinURL Authors

// Package middleware provides reusable HTTP middleware for the MinURL service.
package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.opentelemetry.io/otel/trace"
)

// unwrapper is what http.ResponseController and huma.SetReadDeadline follow through a
// wrapping ResponseWriter to reach the connection. net/http declares it unexported as
// rwUnwrapper and huma inlines it, so it is restated here for the compile-time checks.
//
//   - Convention: https://pkg.go.dev/net/http#NewResponseController
//   - net/http: https://github.com/golang/go/blob/go1.26.8/src/net/http/responsecontroller.go#L42-L44
//   - huma: https://github.com/danielgtaylor/huma/blob/v2.37.3/huma.go#L60-L71
type unwrapper interface {
	Unwrap() http.ResponseWriter
}

// ResponseWriter wraps http.ResponseWriter to track response status code and bytes written.
type ResponseWriter struct {
	http.ResponseWriter
	WroteHeader  bool
	StatusCode   int
	BytesWritten int
}

var (
	_ http.Flusher = (*ResponseWriter)(nil)
	_ unwrapper    = (*ResponseWriter)(nil)
)

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

// Unwrap implements [unwrapper], so http.ResponseController and huma.SetReadDeadline
// reach the connection through AccessLog and PanicRecovery. Without it the body read
// timeout huma sets never fires:
// https://github.com/danielgtaylor/huma/blob/v2.37.3/huma.go#L908-L914
func (w *ResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
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

// RequestDecompress returns a huma middleware that decompresses a request body sent with
// Content-Encoding: gzip. It runs inside the API, not on the router, for two reasons: its
// errors are api's ErrorModel, with the statuses huma itself returns while reading a body,
// and it can skip an operation without a body, whose Content-Encoding huma ignores along
// with the body itself.
//
// Any other encoding is a 415. A gzip body that is corrupt is a 400, and one that stalls past
// the operation's body read timeout a 408. The operation's MaxBodyBytes applies to the body
// both as sent and decompressed: over it as sent is a 413 here, and reaching it decompressed
// is huma's own 413, before the body expands further. So gzip never raises the limit, though
// for a body that does not compress it lowers it by gzip's overhead.
func RequestDecompress(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		op := ctx.Operation()

		encoding := strings.TrimSpace(ctx.Header("Content-Encoding"))
		if encoding == "" || op.RequestBody == nil {
			next(ctx)
			return
		}

		// huma sets the body read deadline once its middlewares have run, too late for the
		// read below, so set it here the same way. The 415 needs it too: before writing a
		// response, net/http reads what is left of a small body, and would wait for a stalled
		// one until the server's ReadTimeout, by when its WriteTimeout has passed as well.
		// https://github.com/danielgtaylor/huma/blob/v2.37.3/huma.go#L908-L914
		if op.BodyReadTimeout > 0 {
			_ = ctx.SetReadDeadline(time.Now().Add(op.BodyReadTimeout))
		} else if op.BodyReadTimeout < 0 {
			_ = ctx.SetReadDeadline(time.Time{})
		}

		if !strings.EqualFold(encoding, "gzip") {
			_ = huma.WriteErr(api, ctx, http.StatusUnsupportedMediaType,
				"unsupported Content-Encoding, only gzip is accepted")

			return
		}

		data, err := readGzip(ctx.BodyReader(), op.MaxBodyBytes)
		netErr, _ := errors.AsType[net.Error](err)
		_, tooLarge := errors.AsType[*http.MaxBytesError](err)

		switch {
		case netErr != nil && netErr.Timeout():
			_ = huma.WriteErr(api, ctx, http.StatusRequestTimeout, "request body read timeout")
		case tooLarge:
			_ = huma.WriteErr(api, ctx, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body is too large limit=%d bytes", op.MaxBodyBytes))
		case err != nil:
			_ = huma.WriteErr(api, ctx, http.StatusBadRequest, "invalid gzip request body")
		default:
			next(decompressedContext{humaContext: ctx, body: bytes.NewReader(data)})
		}
	}
}

// readGzip decompresses a gzip body. A positive maxBytes, as huma treats it, limits the body
// as sent, since a gzip member can take bytes without producing any, and stops the read once
// it decompresses to maxBytes, where huma answers with its own 413.
func readGzip(body io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes > 0 {
		// No ResponseWriter to flag: only the *http.MaxBytesError is wanted.
		body = http.MaxBytesReader(nil, io.NopCloser(body), maxBytes)
	}

	// Not closed: Close only returns an error the read has already returned.
	gr, err := gzip.NewReader(body)
	if err != nil {
		return nil, err
	}

	var r io.Reader = gr
	if maxBytes > 0 {
		r = io.LimitReader(gr, maxBytes)
	}

	return io.ReadAll(r)
}

// humaContext renames huma.Context for embedding: a field named Context would hide its
// Context method.
type humaContext huma.Context

// decompressedContext is handed downstream once RequestDecompress has read the whole body.
// It covers what huma reads a JSON body through, not a multipart form: humachi parses that
// from the raw request, which the middleware has already drained, so a gzip multipart
// request would fail with a 422. No operation takes a multipart form.
type decompressedContext struct {
	humaContext

	body io.Reader
}

var _ interface{ Unwrap() huma.Context } = decompressedContext{}

// Unwrap returns the context it wraps, as huma's own wrapper does, so humachi.Unwrap reaches
// the request through it.
func (c decompressedContext) Unwrap() huma.Context {
	return c.humaContext
}

// BodyReader returns the decompressed body.
func (c decompressedContext) BodyReader() io.Reader {
	return c.body
}

// SetReadDeadline ignores the deadline huma sets before reading the body, which is already
// read. When a body reaches EOF, net/http starts a background read on the connection, and a
// read deadline set after that cancels the request context when it fires, so a gzip request
// still in its handler at the body read timeout would fail with context.Canceled.
//
//   - EOF starts the background read: https://github.com/golang/go/blob/go1.26.8/src/net/http/server.go#L2053-L2057
//   - which clears the deadline first: https://github.com/golang/go/blob/go1.26.8/src/net/http/server.go#L687-L699
//   - a timeout on it cancels the context: https://github.com/golang/go/blob/go1.26.8/src/net/http/server.go#L729-L733
//     and https://github.com/golang/go/blob/go1.26.8/src/net/http/server.go#L769-L777
func (decompressedContext) SetReadDeadline(time.Time) error {
	return nil
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
