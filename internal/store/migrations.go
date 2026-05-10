// Copyright 2024 The MinURL Authors

package store

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

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
