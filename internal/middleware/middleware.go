// Copyright 2026 The MinURL Authors

// Package middleware provides reusable HTTP middleware for the MinURL service.
//
// This file holds the ResponseWriter every middleware in the package wraps a response in,
// and WriteError; the middlewares themselves live in logging.go, recovery.go and decompress.go.
package middleware

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
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

// WriteError answers status with the problem details body huma writes for its own errors,
// for the responses huma never sees: a recovered panic, and a request the router matches
// to no operation. Every operation publishes a default ErrorModel response, so a generated
// client decodes these bodies too.
//
// The body comes from huma.NewError and huma's own JSON format, as toHTTPError's does, so
// both 500s read the same. It lacks only $schema, which huma's response transformer adds
// and the schema does not require.
func WriteError(w http.ResponseWriter, status int) {
	err := huma.NewError(status, http.StatusText(status))

	// A handler that panicked may have set a Content-Length for the body it never sent,
	// which would cut this one short. http.Error drops it for the same reason.
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(err.GetStatus())

	// Only writing to the connection can fail here, after the status is sent: nothing is
	// left to report to.
	_ = huma.DefaultJSONFormat.Marshal(w, err)
}
