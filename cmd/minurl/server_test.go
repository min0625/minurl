package main

import (
	"net"
	"testing"
)

func TestServerListenLogValues(t *testing.T) {
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotBoundAddr, gotDocsURL := serverListenLogValues(tt.addr)
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
