// Copyright 2024 The MinURL Authors

// Package store provides persistence backends for the MinURL service.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"strings"

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
//
// dsn must be a sqlite3:// URL:
//
//	sqlite3://minurl.sqlite3
//	sqlite3://var/data/minurl.sqlite3
//	sqlite3:///absolute/path/minurl.sqlite3
//	sqlite3://minurl.sqlite3?cache=shared
func NewSQLiteBackends(
	dsn string,
) (*SQLiteShortURLStorage, *SQLiteShortURLCounter, io.Closer, error) {
	db, err := openSQLiteDB(dsn)
	if err != nil {
		return nil, nil, nil, err
	}

	storage := &SQLiteShortURLStorage{db: db}
	counter := &SQLiteShortURLCounter{db: db}
	closer := &sqliteDBCloser{db: db}

	return storage, counter, closer, nil
}

func openSQLiteDB(dsn string) (*sql.DB, error) {
	path, err := parseSQLiteDSN(dsn)
	if err != nil {
		return nil, err
	}

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

	// Add expire_time column for existing databases.
	// Error is intentionally ignored: the only expected failure is "duplicate column name"
	// when the column already exists. A proper migration strategy will be addressed later.
	_, _ = db.ExecContext(ctx, `ALTER TABLE short_urls ADD COLUMN expire_time TEXT`)

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

// parseSQLiteDSN converts a sqlite3:// URL to the driver path accepted by
// modernc.org/sqlite.
//
// URL → driver path mapping:
//
//	sqlite3://minurl.sqlite3            → minurl.sqlite3
//	sqlite3://var/data/minurl.sqlite3   → var/data/minurl.sqlite3
//	sqlite3:///absolute/path/minurl.db  → /absolute/path/minurl.db
//	sqlite3://minurl.sqlite3?cache=shared → file:minurl.sqlite3?cache=shared
func parseSQLiteDSN(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "sqlite3://") {
		return "", fmt.Errorf("sqlite3 dsn must start with sqlite3://: %q", dsn)
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse sqlite3 dsn %q: %w", dsn, err)
	}

	// u.Host holds the leading segment for relative paths:
	//   sqlite3://dir/file → host="dir", path="/file" → "dir/file"
	// For absolute paths (sqlite3:///abs):
	//   host="", path="/abs" → "/abs"
	filePath := u.Host + u.Path
	if filePath == "" {
		return "", fmt.Errorf("sqlite3 dsn has empty path: %q", dsn)
	}

	if u.RawQuery != "" {
		return "file:" + filePath + "?" + u.RawQuery, nil
	}

	return filePath, nil
}
