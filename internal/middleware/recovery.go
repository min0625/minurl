// Copyright 2026 The MinURL Authors

package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// PanicRecovery catches panics in downstream handlers, logs them, and answers 500 with
// an ErrorModel body unless the handler already started its response.
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
					WriteError(rw, http.StatusInternalServerError)
				}
			}
		}()

		next.ServeHTTP(rw, r)
	})
}
