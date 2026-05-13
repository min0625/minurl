// Copyright 2024 The MinURL Authors

// Package store provides persistence backends for the MinURL service.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/min0625/minurl/internal/service"
	_ "modernc.org/sqlite" // register sqlite driver
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

type sqliteDBCloser struct {
	db *sql.DB
}

func (c *sqliteDBCloser) Close() error {
	return c.db.Close()
}

func (c *sqliteDBCloser) PingContext(ctx context.Context) error {
	return c.db.PingContext(ctx)
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
) (*SQLiteShortURLStorage, *SQLiteShortURLCounter, CloserPinger, error) {
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

	if err := migrateSQLite(db); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("migrate sqlite database: %w", err)
	}

	return db, nil
}

func migrateSQLite(db *sql.DB) error {
	sourceDriver, err := iofs.New(sqliteMigrations, "migrations/sqlite")
	if err != nil {
		return fmt.Errorf("create sqlite migration source: %w", err)
	}

	dbDriver, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	if err != nil {
		return fmt.Errorf("create sqlite migration driver: %w", err)
	}

	return runMigrations(sourceDriver, dbDriver, "sqlite")
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

// SQLiteShortURLStorage is a SQLite-backed storage implementation.
type SQLiteShortURLStorage struct {
	db *sql.DB
}

// CreateIfAbsent stores the entry if the ID does not already exist.
// Returns true if the entry was inserted, false if it already existed.
func (s *SQLiteShortURLStorage) CreateIfAbsent(
	ctx context.Context,
	entry service.ShortURL,
) (bool, error) {
	var expireTimeStr *string

	if entry.ExpireTime != nil {
		formatted := entry.ExpireTime.UTC().Format(time.RFC3339Nano)
		expireTimeStr = &formatted
	}

	result, err := s.db.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO short_urls (id, original_url, create_time, expire_time) VALUES (?, ?, ?, ?)`,
		entry.ID,
		entry.OriginalURL,
		entry.CreateTime.UTC().Format(time.RFC3339Nano),
		expireTimeStr,
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
) (service.ShortURL, bool, error) {
	var entry service.ShortURL

	var createTimeStr string

	var expireTimeStr *string

	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, original_url, create_time, expire_time FROM short_urls WHERE id = ?`,
		id,
	).Scan(&entry.ID, &entry.OriginalURL, &createTimeStr, &expireTimeStr)

	if errors.Is(err, sql.ErrNoRows) {
		return service.ShortURL{}, false, nil
	}

	if err != nil {
		return service.ShortURL{}, false, fmt.Errorf("query short url: %w", err)
	}

	entry.CreateTime, err = time.Parse(time.RFC3339Nano, createTimeStr)
	if err != nil {
		return service.ShortURL{}, false, fmt.Errorf("parse create_time %q: %w", createTimeStr, err)
	}

	if expireTimeStr != nil {
		t, parseErr := time.Parse(time.RFC3339Nano, *expireTimeStr)
		if parseErr != nil {
			return service.ShortURL{}, false, fmt.Errorf(
				"parse expire_time %q: %w",
				*expireTimeStr,
				parseErr,
			)
		}

		entry.ExpireTime = &t
	}

	return entry, true, nil
}

// Close releases the database connection.
func (s *SQLiteShortURLStorage) Close() error {
	return s.db.Close()
}

// SQLiteShortURLCounter is a SQLite-backed counter implementation.
type SQLiteShortURLCounter struct {
	db *sql.DB
}

// Close releases the database connection.
func (c *SQLiteShortURLCounter) Close() error {
	return c.db.Close()
}

// Next returns the next monotonic sequence value.
func (c *SQLiteShortURLCounter) Next(ctx context.Context) (uint64, error) {
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
) (uint64, bool, error) {
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

	if current == math.MaxUint64 {
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

	return next, true, nil
}
