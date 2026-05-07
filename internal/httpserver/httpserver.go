// Copyright 2024 The MinURL Authors

// Package httpserver provides helpers for building the MinURL HTTP server and router.
package httpserver

import (
	"net"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/min0625/minurl/internal/handler"
	"github.com/min0625/minurl/internal/middleware"
)

// NewRouter creates a chi router with the standard middleware stack applied.
func NewRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.PanicRecovery)
	r.Use(middleware.RequestLogger)
	r.Use(middleware.AccessLog)
	r.Use(middleware.RequestDecompress)

	return r
}

// BuildAPI creates a chi router with all MinURL handlers registered and returns
// the router together with the Huma API instance for use at runtime.
func BuildAPI(svc handler.ShortURLService, version string) (*chi.Mux, huma.API) {
	r := NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", version))

	handler.Register(api, svc)

	return r, api
}

// BuildOpenAPIRouter creates a chi router suitable for OpenAPI schema generation
// without requiring a live service implementation.
func BuildOpenAPIRouter(version string) (*chi.Mux, huma.API) {
	r := NewRouter()
	cfg := huma.DefaultConfig("MinURL API", version)
	cfg.Servers = []*huma.Server{{URL: "http://localhost:8888"}}
	api := humachi.New(r, cfg)
	handler.RegisterOpenAPI(api)

	return r, api
}

// ListenLogValues derives a human-readable bound address and docs URL from a net.Addr.
// The docs URL is empty when the address does not have a port component.
func ListenLogValues(addr net.Addr) (string, string) {
	boundAddr := addr.String()

	_, port, err := net.SplitHostPort(boundAddr)
	if err != nil || port == "" {
		return boundAddr, ""
	}

	return boundAddr, "http://" + net.JoinHostPort("localhost", port) + "/docs"
}
