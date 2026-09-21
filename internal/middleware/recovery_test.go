// Copyright 2026 The MinURL Authors

package middleware_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/middleware"
	"github.com/min0625/minurl/internal/testhelpers"
)

// TestPanicRecovery pins the 500 a panic answers with: the ErrorModel body toHTTPError
// writes for its own 500s, not text/plain, and nothing over a response already started.
func TestPanicRecovery(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantStatus  int
		wantType    string
		wantProblem bool
	}{
		{
			name:        "panic before the response",
			handler:     func(http.ResponseWriter, *http.Request) { panic("boom") },
			wantStatus:  http.StatusInternalServerError,
			wantType:    "application/problem+json",
			wantProblem: true,
		},
		{
			name: "panic after setting a Content-Length",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "5")
				panic("boom")
			},
			wantStatus:  http.StatusInternalServerError,
			wantType:    "application/problem+json",
			wantProblem: true,
		},
		{
			name: "panic after the response started",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/csv")
				w.WriteHeader(http.StatusAccepted)
				panic("boom")
			},
			wantStatus: http.StatusAccepted,
			wantType:   "text/csv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer

			orig := slog.Default()

			t.Cleanup(func() { slog.SetDefault(orig) })
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			res := httptest.NewRecorder()

			middleware.PanicRecovery(tt.handler).ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.Code, tt.wantStatus)
			}

			if got := res.Header().Get("Content-Type"); got != tt.wantType {
				t.Fatalf("Content-Type = %q, want %q", got, tt.wantType)
			}

			if !strings.Contains(logs.String(), `msg="panic recovered"`) {
				t.Fatalf("log = %q, want a panic recovered line", logs.String())
			}

			if !tt.wantProblem {
				if res.Body.Len() != 0 {
					t.Fatalf("body = %q, want nothing written over the started response", res.Body.String())
				}

				return
			}

			// Result, not Header: what was sent, not what was set after it.
			if got := res.Result().Header.Get("Content-Length"); got != "" {
				t.Fatalf("Content-Length = %q, want none: it would cut the body short", got)
			}

			got, err := testhelpers.DecodeErrorModel(res.Body.Bytes())
			if err != nil {
				t.Fatalf("decode ErrorModel from %q: %v", res.Body.String(), err)
			}

			want := huma.ErrorModel{
				Title:  http.StatusText(http.StatusInternalServerError),
				Status: http.StatusInternalServerError,
				Detail: http.StatusText(http.StatusInternalServerError),
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ErrorModel = %+v, want %+v", got, want)
			}
		})
	}
}
