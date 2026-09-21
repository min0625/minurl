// Copyright 2026 The MinURL Authors

package middleware_test

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/min0625/minurl/internal/middleware"
)

const gzipEncoding = "gzip"

func gzipBody(t *testing.T, data string) *bytes.Buffer {
	t.Helper()

	return gzipBodyWithExtra(t, data, nil)
}

// gzipBodyOfSize gzips data into a body of exactly size bytes, padding it with the gzip
// header's extra field, which adds nothing to what it decompresses to.
func gzipBodyOfSize(t *testing.T, data string, size int) *bytes.Buffer {
	t.Helper()

	// The extra field is a 2-byte length and the data. Not the name field: Go's reader
	// refuses a name of 512 bytes or more.
	buf := gzipBodyWithExtra(t, data, make([]byte, size-gzipBody(t, data).Len()-2))
	if buf.Len() != size {
		t.Fatalf("gzip body is %d bytes, want %d", buf.Len(), size)
	}

	return buf
}

func gzipBodyWithExtra(t *testing.T, data string, extra []byte) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	w := gzip.NewWriter(&buf)
	w.Extra = extra

	_, err := w.Write([]byte(data))
	if err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	return &buf
}

// corruptTrailer flips the last byte of a gzip body, its decompressed size.
func corruptTrailer(buf *bytes.Buffer) *bytes.Buffer {
	buf.Bytes()[buf.Len()-1] ^= 0xff

	return buf
}

type echoBody struct {
	Message string `json:"message"`
}

type echoIO struct {
	Body echoBody
}

const echoMaxBodyBytes = 1024

// newDecompressTestAPI returns an API behind RequestDecompress with an operation that echoes
// the body it receives, after an operation middleware calls humachi.Unwrap on its context,
// and one that takes no body.
func newDecompressTestAPI() http.Handler {
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("test", "0.0.0"))
	api.UseMiddleware(middleware.RequestDecompress(api))

	huma.Register(api, huma.Operation{
		Method:       http.MethodPost,
		Path:         "/echo",
		MaxBodyBytes: echoMaxBodyBytes,
		// An operation middleware gets the context RequestDecompress hands on, and
		// humachi.Unwrap panics unless that unwraps to humachi's own.
		Middlewares: huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
			humachi.Unwrap(ctx)
			next(ctx)
		}},
	}, func(_ context.Context, in *echoIO) (*echoIO, error) {
		return in, nil
	})
	huma.Get(api, "/no-body", func(context.Context, *struct{}) (*struct{}, error) {
		return &struct{}{}, nil
	})

	return r
}

func TestRequestDecompressMiddleware(t *testing.T) {
	t.Parallel()

	const payload = `{"message":"hello"}`

	tests := []struct {
		name            string
		method          string
		target          string
		contentEncoding string
		body            io.Reader
		wantStatus      int
	}{
		{
			name:       "no Content-Encoding passes through",
			body:       strings.NewReader(payload),
			wantStatus: http.StatusOK,
		},
		{
			name:            "Content-Encoding: gzip decompresses body",
			contentEncoding: gzipEncoding,
			body:            gzipBody(t, payload),
			wantStatus:      http.StatusOK,
		},
		{
			name:            "unsupported encoding returns 415",
			contentEncoding: "br",
			body:            strings.NewReader(payload),
			wantStatus:      http.StatusUnsupportedMediaType,
		},
		{
			name:            "invalid gzip body returns 400",
			contentEncoding: gzipEncoding,
			body:            strings.NewReader("not-gzip-data"),
			wantStatus:      http.StatusBadRequest,
		},
		{
			// A recorder has no connection to set a deadline on, so the timeout is faked here;
			// httpserver's TestBuildAPIAnswersAStalledBody stalls a real one.
			name:            "gzip body read timeout returns 408",
			contentEncoding: gzipEncoding,
			body:            iotest.ErrReader(os.ErrDeadlineExceeded),
			wantStatus:      http.StatusRequestTimeout,
		},
		{
			// The trailer is corrupt, so decompressing past the limit would be a 400.
			name:            "decompressed body reaching the limit returns 413",
			contentEncoding: gzipEncoding,
			body:            corruptTrailer(gzipBody(t, strings.Repeat("a", 64*echoMaxBodyBytes))),
			wantStatus:      http.StatusRequestEntityTooLarge,
		},
		{
			// huma's own limit, which the gzip cases below copy for the body as sent: if a huma
			// upgrade moves it, these fail and the gzip ones still pass. Spaces pad the JSON.
			name:       "plain body reaching the limit returns 413",
			body:       strings.NewReader(payload + strings.Repeat(" ", echoMaxBodyBytes-len(payload))),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "plain body just under the limit passes",
			body:       strings.NewReader(payload + strings.Repeat(" ", echoMaxBodyBytes-1-len(payload))),
			wantStatus: http.StatusOK,
		},
		{
			// The same limit as the plain body above, for a gzip body as sent.
			name:            "body reaching the limit as sent returns 413",
			contentEncoding: gzipEncoding,
			body:            gzipBodyOfSize(t, payload, echoMaxBodyBytes),
			wantStatus:      http.StatusRequestEntityTooLarge,
		},
		{
			name:            "body just under the limit as sent passes",
			contentEncoding: gzipEncoding,
			body:            gzipBodyOfSize(t, payload, echoMaxBodyBytes-1),
			wantStatus:      http.StatusOK,
		},
		{
			// Empty gzip members decompress to nothing, so only the size as sent can refuse them.
			name:            "body over the limit as sent returns 413",
			contentEncoding: gzipEncoding,
			body: io.MultiReader(
				strings.NewReader(strings.Repeat(gzipBody(t, "").String(), echoMaxBodyBytes)),
				gzipBody(t, payload),
			),
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			// huma never reads the body of an operation without one, so its encoding is no error.
			name:            "operation without a body ignores Content-Encoding",
			method:          http.MethodGet,
			target:          "/no-body",
			contentEncoding: "br",
			body:            strings.NewReader(payload),
			wantStatus:      http.StatusNoContent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(
				context.Background(),
				cmp.Or(tc.method, http.MethodPost),
				cmp.Or(tc.target, "/echo"),
				tc.body,
			)
			req.Header.Set("Content-Type", "application/json")

			if tc.contentEncoding != "" {
				req.Header.Set("Content-Encoding", tc.contentEncoding)
			}

			res := httptest.NewRecorder()
			newDecompressTestAPI().ServeHTTP(res, req)

			if res.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", res.Code, tc.wantStatus, res.Body.String())
			}

			switch {
			case res.Code == http.StatusOK:
				var got echoBody
				if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil || got.Message != "hello" {
					t.Fatalf("echoed body = %s (%v), want the message %q", res.Body.String(), err, "hello")
				}
			case res.Code >= http.StatusBadRequest:
				// The errors every other huma status uses, not net/http's plain text.
				if ct := res.Header().Get("Content-Type"); ct != "application/problem+json" {
					t.Fatalf("Content-Type = %q, want %q", ct, "application/problem+json")
				}
			}
		})
	}
}
