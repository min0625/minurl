// Copyright 2026 The MinURL Authors

// Package httpserver provides helpers for building the MinURL HTTP server and router.
package httpserver

import (
	"cmp"
	"net"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/min0625/minurl/internal/handler"
	"github.com/min0625/minurl/internal/middleware"
	"github.com/min0625/minurl/internal/service"
)

// allowMethods are the methods chi routes, in the order a 405's Allow header lists them.
var allowMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
	http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace,
}

// NewRouter creates a chi router with the standard middleware stack applied.
//
// A request that matches no operation never reaches huma, so the router answers it with
// an ErrorModel itself, instead of chi's text/plain 404 and bodiless 405.
func NewRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.PanicRecovery)
	r.Use(middleware.RequestLogger)
	r.Use(middleware.AccessLog)

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		middleware.WriteError(w, http.StatusNotFound)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		// chi writes Allow only in its own 405 handler and passes a custom one nothing,
		// but a 405 must list the allowed methods (RFC 9110 §15.5.6), so ask the router.
		// The path is the one chi routes on, as long as no middleware rewrites RoutePath.
		path := cmp.Or(req.URL.RawPath, req.URL.Path)

		// chi sends a method it does not know here before it looks the path up, so a path
		// no method matches is still a 404.
		status := http.StatusNotFound

		for _, method := range allowMethods {
			if r.Match(chi.NewRouteContext(), method, path) {
				w.Header().Add("Allow", method)

				status = http.StatusMethodNotAllowed
			}
		}

		middleware.WriteError(w, status)
	})

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
	// Before Register: huma captures the API's middlewares when an operation is registered.
	api.UseMiddleware(middleware.RequestDecompress(api))

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
