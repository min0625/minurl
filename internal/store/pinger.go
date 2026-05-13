// Copyright 2024 The MinURL Authors

package store

import (
	"context"
	"io"
)

// Pinger can check database connectivity.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// CloserPinger combines io.Closer with Pinger.
// Both SQLite and Postgres storage backends implement this interface.
type CloserPinger interface {
	io.Closer
	Pinger
}
