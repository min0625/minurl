// Copyright 2024 The MinURL Authors

// Package store provides persistence backends for the MinURL service.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/min0625/minurl/internal/model"
	_ "modernc.org/sqlite" // register sqlite driver
)

const shortURLCounterName = "short_url"

// SQLiteShortURLStorage is a SQLite-backed storage implementation.
type SQLiteShortURLStorage struct {
	db *sql.DB
}

// SQLiteShortURLCounter is a SQLite-backed counter implementation.
type SQLiteShortURLCounter struct {
	db *sql.DB
}

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

// NewSQLiteShortURLStorage opens (or creates) a SQLite database at the given path
// and runs the schema migration.
func NewSQLiteShortURLStorage(path string) (*SQLiteShortURLStorage, error) {
	db, err := openSQLiteDB(path)
	if err != nil {
		return nil, err
	}

	return &SQLiteShortURLStorage{db: db}, nil
}

// NewSQLiteShortURLCounter opens (or creates) a SQLite database at the given
// path and runs the schema migration.
func NewSQLiteShortURLCounter(path string) (*SQLiteShortURLCounter, error) {
	db, err := openSQLiteDB(path)
	if err != nil {
		return nil, err
	}

	return &SQLiteShortURLCounter{db: db}, nil
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

// Close releases the database connection.
func (c *SQLiteShortURLCounter) Close() error {
	return c.db.Close()
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

// CreateIfAbsent stores the entry if the ID does not already exist.
// Returns true if the entry was inserted, false if it already existed.
func (s *SQLiteShortURLStorage) CreateIfAbsent(
	ctx context.Context,
	entry model.ShortURL,
) (bool, error) {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO short_urls (id, original_url, create_time) VALUES (?, ?, ?)`,
		entry.ID,
		entry.OriginalURL,
		entry.CreateTime.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, fmt.Errorf("insert short url: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}

	return rows > 0, nil
}

// GetByID returns the short URL with the given ID.
func (s *SQLiteShortURLStorage) GetByID(
	ctx context.Context,
	id string,
) (model.ShortURL, bool, error) {
	var entry model.ShortURL

	var createTimeStr string

	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, original_url, create_time FROM short_urls WHERE id = ?`,
		id,
	).Scan(&entry.ID, &entry.OriginalURL, &createTimeStr)

	if errors.Is(err, sql.ErrNoRows) {
		return model.ShortURL{}, false, nil
	}

	if err != nil {
		return model.ShortURL{}, false, fmt.Errorf("query short url: %w", err)
	}

	entry.CreateTime, err = time.Parse(time.RFC3339Nano, createTimeStr)
	if err != nil {
		return model.ShortURL{}, false, fmt.Errorf("parse create_time %q: %w", createTimeStr, err)
	}

	return entry, true, nil
}

// Close releases the database connection.
func (s *SQLiteShortURLStorage) Close() error {
	return s.db.Close()
}

// Next returns the next monotonic sequence value.
func (c *SQLiteShortURLCounter) Next(ctx context.Context) (uint32, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}

		tx, err := c.db.BeginTx(ctx, nil)
		if err != nil {
			return 0, fmt.Errorf("begin tx: %w", err)
		}

		next, committed, err := c.nextInTx(ctx, tx)
		if err != nil {
			_ = tx.Rollback()

			return 0, err
		}

		if !committed {
			_ = tx.Rollback()

			continue
		}

		if err := tx.Commit(); err != nil {
			return 0, fmt.Errorf("commit tx: %w", err)
		}

		return next, nil
	}
}

func (c *SQLiteShortURLCounter) nextInTx(
	ctx context.Context,
	tx *sql.Tx,
) (uint32, bool, error) {
	var current uint64

	err := tx.QueryRowContext(
		ctx,
		`SELECT value FROM counters WHERE name = ?`,
		shortURLCounterName,
	).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		result, insertErr := tx.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO counters (name, value) VALUES (?, 1)`,
			shortURLCounterName,
		)
		if insertErr != nil {
			return 0, false, fmt.Errorf("initialize counter row: %w", insertErr)
		}

		affectedRows, affectedErr := result.RowsAffected()
		if affectedErr != nil {
			return 0, false, fmt.Errorf("rows affected: %w", affectedErr)
		}

		if affectedRows == 0 {
			return 0, false, nil
		}

		return 1, true, nil
	}

	if err != nil {
		return 0, false, fmt.Errorf("read counter value: %w", err)
	}

	if current >= uint64(math.MaxUint32) {
		return 0, false, fmt.Errorf("short id sequence exhausted")
	}

	next := current + 1

	result, err := tx.ExecContext(
		ctx,
		`UPDATE counters SET value = ? WHERE name = ? AND value = ?`,
		next,
		shortURLCounterName,
		current,
	)
	if err != nil {
		return 0, false, fmt.Errorf("update counter value: %w", err)
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("rows affected: %w", err)
	}

	if affectedRows == 0 {
		return 0, false, nil
	}

	return uint32(next), true, nil
}
