// Copyright 2026 The MinURL Authors

package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

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
