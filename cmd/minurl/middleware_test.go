package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessLogMiddlewareDefaultsStatusOK(t *testing.T) {
	var buf bytes.Buffer

	orig := slog.Default()

	t.Cleanup(func() {
		slog.SetDefault(orig)
	})

	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{})))

	h := accessLogMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)

	if !strings.Contains(buf.String(), "status=200") {
		t.Fatalf("expected access log to contain status=200, got %q", buf.String())
	}
}

func gzipBody(t *testing.T, data string) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	w := gzip.NewWriter(&buf)

	_, err := w.Write([]byte(data))
	if err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	return &buf
}

func TestRequestDecompressMiddleware(t *testing.T) {
	const payload = `{"original_url":"https://example.com/very/long/path?query=value"}`

	tests := []struct {
		name            string
		contentEncoding string
		body            func() io.Reader
		wantStatus      int
		wantBody        string
	}{
		{
			name:            "no Content-Encoding passes through",
			contentEncoding: "",
			body:            func() io.Reader { return strings.NewReader(payload) },
			wantStatus:      http.StatusOK,
			wantBody:        payload,
		},
		{
			name:            "Content-Encoding: gzip decompresses body",
			contentEncoding: "gzip",
			body:            func() io.Reader { return gzipBody(t, payload) },
			wantStatus:      http.StatusOK,
			wantBody:        payload,
		},
		{
			name:            "unsupported encoding returns 415",
			contentEncoding: "br",
			body:            func() io.Reader { return strings.NewReader(payload) },
			wantStatus:      http.StatusUnsupportedMediaType,
			wantBody:        "",
		},
		{
			name:            "invalid gzip body returns 400",
			contentEncoding: "gzip",
			body:            func() io.Reader { return strings.NewReader("not-gzip-data") },
			wantStatus:      http.StatusBadRequest,
			wantBody:        "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody string

			inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
			})

			h := requestDecompressMiddleware(inner)

			req := httptest.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				"/",
				tc.body(),
			)
			if tc.contentEncoding != "" {
				req.Header.Set("Content-Encoding", tc.contentEncoding)
			}

			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)

			if res.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, res.Code)
			}

			if tc.wantBody != "" && gotBody != tc.wantBody {
				t.Fatalf("expected body %q, got %q", tc.wantBody, gotBody)
			}
		})
	}
}
