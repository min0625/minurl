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
	"github.com/min0625/minurl/internal/service"
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
// the router together with the Huma API instance.
//
// This is the only place the API is assembled, so the document written by
// BuildOpenAPISpec cannot drift from the routes and config the runtime serves.
// servers is the single deliberate difference: the runtime passes none and stays
// host-relative, while the generated document pins a URL.
func BuildAPI(svc service.ShortURLServicer, version string, servers ...*huma.Server) (*chi.Mux, huma.API) {
	r := NewRouter()

	cfg := huma.DefaultConfig("MinURL API", version)
	cfg.Servers = servers

	api := humachi.New(r, cfg)

	handler.Register(api, svc)

	return r, api
}

// BuildOpenAPISpec builds the OpenAPI document from the same routes and config
// the runtime serves. The service is nil: registration reads only operation
// metadata and handler signatures, and the router is discarded rather than
// served, so no handler ever runs.
func BuildOpenAPISpec(version string) *huma.OpenAPI {
	// Pinned because huma derives the $schema example URLs from it at
	// registration time, and it becomes the generated Kiota client's base URL.
	_, api := BuildAPI(nil, version, &huma.Server{URL: "http://localhost:8888"})

	return api.OpenAPI()
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
