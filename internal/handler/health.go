// Copyright 2024 The MinURL Authors

package handler

import (
	"context"
	"time"

	"github.com/alexliesenfeld/health"
	"github.com/go-chi/chi/v5"
)

// DBPinger abstracts the database connectivity check used by health probes.
type DBPinger interface {
	PingContext(ctx context.Context) error
}

// RegisterHealthHandlers mounts /livez, /readyz, and /startupz on r.
//
//   - /livez  — liveness: HTTP server is responding (no DB check)
//   - /readyz — readiness: DB is reachable (traffic can be served)
//   - /startupz — startup: same as readiness; gates K8s liveness/readiness probes
//
// The pinger is used to verify database connectivity for /readyz and /startupz.
func RegisterHealthHandlers(r chi.Router, pinger DBPinger) {
	liveness := health.NewChecker()

	readiness := health.NewChecker(
		health.WithCacheDuration(2*time.Second),
		health.WithTimeout(5*time.Second),
		health.WithCheck(health.Check{
			Name:    "database",
			Timeout: 3 * time.Second,
			Check:   pinger.PingContext,
		}),
	)

	r.Handle("/livez", health.NewHandler(liveness))
	r.Handle("/readyz", health.NewHandler(readiness))
	r.Handle("/startupz", health.NewHandler(readiness))
}
