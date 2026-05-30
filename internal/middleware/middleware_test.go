package middleware_test

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

	"github.com/min0625/minurl/internal/middleware"
)

const gzipEncoding = "gzip"

func TestAccessLogMiddlewareDefaultsStatusOK(t *testing.T) {
	var buf bytes.Buffer

	orig := slog.Default()

	t.Cleanup(func() {
		slog.SetDefault(orig)
	})

	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{})))

	h := middleware.AccessLog(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
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
			contentEncoding: gzipEncoding,
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
			contentEncoding: gzipEncoding,
			body:            func() io.Reader { return strings.NewReader("not-gzip-data") },
			wantStatus:      http.StatusBadRequest,
			wantBody:        "",
		},
		{
			name:            "decompressed body exceeding limit returns 413",
			contentEncoding: gzipEncoding,
			body: func() io.Reader {
				// Produce a payload that decompresses to just over MaxDecompressedBodySize.
				oversized := strings.Repeat("a", middleware.MaxDecompressedBodySize+1)
				return gzipBody(t, oversized)
			},
			wantStatus: http.StatusRequestEntityTooLarge,
			wantBody:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				gotBody          string
				gotContentLength int64
			)

			inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				gotContentLength = r.ContentLength
			})

			h := middleware.RequestDecompress(inner)

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

			if tc.wantBody != "" && gotContentLength != int64(len(tc.wantBody)) {
				t.Fatalf(
					"expected ContentLength %d, got %d",
					len(tc.wantBody),
					gotContentLength,
				)
			}
		})
	}
}
