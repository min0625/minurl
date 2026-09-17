// Copyright 2026 The MinURL Authors

package httpserver_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/httpserver"
	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/testhelpers"
)

// TestBuildAPIAnswersAStalledBody pins huma's body read timeout. huma sets its
// deadline through the ResponseWriter, so it fires only if every middleware wrapper
// unwraps to the connection, which a ResponseRecorder cannot show. RequestDecompress reads
// a gzip body before huma would set the deadline, so it sets the deadline itself.
func TestBuildAPIAnswersAStalledBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers string
		partial string
		want    int
	}{
		{name: "uncompressed", partial: `{"original_url"`, want: http.StatusRequestTimeout},
		// The first bytes of a gzip header: the gzip reader waits for the rest.
		{
			name: "gzip", headers: "Content-Encoding: gzip\r\n", partial: "\x1f\x8b\x08",
			want: http.StatusRequestTimeout,
		},
		// Refused unread, but net/http reads what is left of a small body before answering.
		{
			name: "unsupported encoding", headers: "Content-Encoding: br\r\n", partial: "abc",
			want: http.StatusUnsupportedMediaType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, err := service.NewShortURLServiceWithAllDependencies(
				testhelpers.NewStorage(), testhelpers.NewCounter(), nil,
			)
			if err != nil {
				t.Fatalf("NewShortURLServiceWithAllDependencies() error = %v", err)
			}

			r, _ := httpserver.BuildAPI(svc, "test")
			srv := httptest.NewServer(r)
			t.Cleanup(srv.Close)

			conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", srv.Listener.Addr().String())
			if err != nil {
				t.Fatalf("dial: %v", err)
			}

			defer func() { _ = conn.Close() }()

			// Promise 100 bytes, send a few, then stall.
			_, err = io.WriteString(conn, "POST /api/v1/urls HTTP/1.1\r\nHost: minurl\r\n"+
				"Content-Type: application/json\r\n"+tt.headers+"Content-Length: 100\r\n\r\n"+tt.partial)
			if err != nil {
				t.Fatalf("write request: %v", err)
			}

			// The body read timeout is 5s; no response by 10s means the deadline never fired.
			if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
				t.Fatalf("set read deadline: %v", err)
			}

			resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

// TestBuildAPIServesAGzipRequestPastTheBodyReadTimeout guards the other side of that
// deadline. RequestDecompress reads a gzip body to EOF before huma sets it, and a read
// deadline set after EOF cancels the request context when it fires, so a handler still
// running at 5s failed with a 500.
func TestBuildAPIServesAGzipRequestPastTheBodyReadTimeout(t *testing.T) {
	t.Parallel()

	r, _ := httpserver.BuildAPI(slowServicer{}, "test")
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	var body bytes.Buffer

	zw := gzip.NewWriter(&body)
	if _, err := io.WriteString(zw, `{"original_url":"https://example.com"}`); err != nil {
		t.Fatalf("gzip write: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/v1/urls", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

// slowServicer creates a short URL only after huma's 5s body read timeout has passed,
// and fails like a store would if the request context is cancelled first.
type slowServicer struct {
	service.ShortURLServicer
}

func (slowServicer) Create(ctx context.Context, entry service.ShortURL) (*service.ShortURL, error) {
	select {
	case <-time.After(6 * time.Second):
		return &entry, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestListenLogValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		addr          net.Addr
		wantBoundAddr string
		wantDocsURL   string
	}{
		{
			name:          "ipv4 tcp address",
			addr:          &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8888},
			wantBoundAddr: "127.0.0.1:8888",
			wantDocsURL:   "http://localhost:8888/docs",
		},
		{
			name:          "ipv6 tcp address",
			addr:          &net.TCPAddr{IP: net.ParseIP("::"), Port: 9000},
			wantBoundAddr: "[::]:9000",
			wantDocsURL:   "http://localhost:9000/docs",
		},
		{
			name:          "non tcp style address",
			addr:          mockAddr{network: "unix", value: "/tmp/minurl.sock"},
			wantBoundAddr: "/tmp/minurl.sock",
			wantDocsURL:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotBoundAddr, gotDocsURL := httpserver.ListenLogValues(tt.addr)
			if gotBoundAddr != tt.wantBoundAddr {
				t.Fatalf("bound addr = %q, want %q", gotBoundAddr, tt.wantBoundAddr)
			}

			if gotDocsURL != tt.wantDocsURL {
				t.Fatalf("docs url = %q, want %q", gotDocsURL, tt.wantDocsURL)
			}
		})
	}
}

type mockAddr struct {
	network string
	value   string
}

func (a mockAddr) Network() string {
	return a.network
}

func (a mockAddr) String() string {
	return a.value
}

// TestBuildOpenAPISpecMatchesRuntimeRoutes guards the reason both documents are
// built by BuildAPI: the generated spec must describe exactly the operations the
// runtime serves, and must not quietly become empty.
func TestBuildOpenAPISpecMatchesRuntimeRoutes(t *testing.T) {
	t.Parallel()

	want := []string{"create-short-url", "get-short-url", "redirect-short-url"}

	generated := operationIDs(httpserver.BuildOpenAPISpec("test"))
	if !slices.Equal(generated, want) {
		t.Fatalf("generated spec operations = %v, want %v", generated, want)
	}

	_, api := httpserver.BuildAPI(nil, "test")

	if runtime := operationIDs(api.OpenAPI()); !slices.Equal(runtime, generated) {
		t.Fatalf("runtime operations = %v, generated spec operations = %v", runtime, generated)
	}
}

// operationIDs returns every operation ID in the document, sorted.
func operationIDs(spec *huma.OpenAPI) []string {
	var ids []string

	for _, p := range spec.Paths {
		for _, op := range []*huma.Operation{p.Get, p.Post, p.Put, p.Delete, p.Options, p.Head, p.Patch, p.Trace} {
			if op != nil {
				ids = append(ids, op.OperationID)
			}
		}
	}

	slices.Sort(ids)

	return ids
}
