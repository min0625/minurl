// Copyright 2024 The MinURL Authors

package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
)

// shortURLCounterName is the name used to identify the short URL counter row
// in the counters table. Shared across all storage backends.
const shortURLCounterName = "short_url"

// DBPoolConfig holds database connection pool settings.
// These settings apply to the PostgreSQL backend only.
// SQLite always uses a single connection regardless of these settings.
type DBPoolConfig struct {
	// MaxOpenConns sets the maximum number of open connections to the database.
	// 0 means unlimited (not recommended for PostgreSQL).
	MaxOpenConns int
	// MaxIdleConns sets the maximum number of idle connections retained in the pool.
	// 0 means no idle connections are retained.
	MaxIdleConns int
	// ConnMaxLifetime sets the maximum duration a connection may be reused.
	// 0 means no limit (connections are never closed due to age).
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime sets the maximum duration a connection may sit idle.
	// 0 means no limit (idle connections are never closed).
	ConnMaxIdleTime time.Duration
}

// runMigrations applies all pending up migrations using the provided source
// and database drivers. ErrNoChange is treated as success.
//
// m.Close() is intentionally not called: when using WithInstance the database
// driver wraps a caller-owned *sql.DB, and calling Close() would close that
// shared connection. The source driver (iofs/embed) holds no I/O resources.
func runMigrations(sourceDriver source.Driver, dbDriver database.Driver, dbName string) error {
	m, err := migrate.NewWithInstance("iofs", sourceDriver, dbName, dbDriver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
