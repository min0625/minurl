// Copyright 2024 The MinURL Authors

// Package store provides persistence backends for the MinURL service.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	_ "modernc.org/sqlite" // register sqlite driver
)

const shortURLCounterName = "short_url"

type sqliteDBCloser struct {
	db *sql.DB
}

func (c *sqliteDBCloser) Close() error {
	return c.db.Close()
}

// NewSQLiteBackends opens a single SQLite connection and returns storage and
// counter backends that share the same database.
func NewSQLiteBackends(
	path string,
) (*SQLiteShortURLStorage, *SQLiteShortURLCounter, io.Closer, error) {
	db, err := openSQLiteDB(path)
	if err != nil {
		return nil, nil, nil, err
	}

	storage := &SQLiteShortURLStorage{db: db}
	counter := &SQLiteShortURLCounter{db: db}
	closer := &sqliteDBCloser{db: db}

	return storage, counter, closer, nil
}

func openSQLiteDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	// SQLite writes are serialized; keep a single connection to avoid lock contention.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := migrateSQLite(context.Background(), db); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("migrate sqlite database: %w", err)
	}

	return db, nil
}

func migrateSQLite(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS short_urls (
			id           TEXT PRIMARY KEY,
			original_url TEXT NOT NULL,
			create_time  TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS counters (
			name  TEXT PRIMARY KEY,
			value INTEGER NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO counters (name, value) VALUES (?, 0)`,
		shortURLCounterName,
	)

	return err
}
