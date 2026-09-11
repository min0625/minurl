package httpserver_test

import (
	"net"
	"slices"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/httpserver"
)

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
