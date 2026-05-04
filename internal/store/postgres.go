// Copyright 2024 The MinURL Authors

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // register pgx driver
	"github.com/min0625/minurl/internal/model"
)

// postgresDBCloser wraps a shared sql.DB and closes it on Close.
type postgresDBCloser struct {
	db *sql.DB
}

func (c *postgresDBCloser) Close() error {
	return c.db.Close()
}

// NewPostgresBackends opens a PostgreSQL connection pool and returns storage
// and counter backends that share the same pool.
func NewPostgresBackends(
	dsn string,
) (*PostgresShortURLStorage, *PostgresShortURLCounter, io.Closer, error) {
	db, err := openPostgresDB(dsn)
	if err != nil {
		return nil, nil, nil, err
	}

	storage := &PostgresShortURLStorage{db: db}
	counter := &PostgresShortURLCounter{db: db}
	closer := &postgresDBCloser{db: db}

	return storage, counter, closer, nil
}

func openPostgresDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}

	if err := migratePostgres(context.Background(), db); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("migrate postgres database: %w", err)
	}

	return db, nil
}

func migratePostgres(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS short_urls (
			id           TEXT PRIMARY KEY,
			original_url TEXT NOT NULL,
			create_time  TIMESTAMPTZ NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS counters (
			name  TEXT PRIMARY KEY,
			value BIGINT NOT NULL
		)
	`)

	return err
}

// PostgresShortURLStorage is a PostgreSQL-backed short URL storage.
type PostgresShortURLStorage struct {
	db *sql.DB
}

// CreateIfAbsent inserts the entry if the id does not already exist.
// Returns true if the row was inserted, false if it already existed.
func (s *PostgresShortURLStorage) CreateIfAbsent(
	ctx context.Context,
	entry model.ShortURL,
) (bool, error) {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO short_urls (id, original_url, create_time)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`,
		entry.ID,
		entry.OriginalURL,
		entry.CreateTime.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("create short url: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}

	return affected > 0, nil
}

// GetByID fetches a short URL by its id.
// Returns (entry, true, nil) when found, or (zero, false, nil) when not found.
func (s *PostgresShortURLStorage) GetByID(
	ctx context.Context,
	id string,
) (model.ShortURL, bool, error) {
	var (
		entry      model.ShortURL
		createTime time.Time
	)

	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, original_url, create_time FROM short_urls WHERE id = $1`,
		id,
	).Scan(&entry.ID, &entry.OriginalURL, &createTime)

	if errors.Is(err, sql.ErrNoRows) {
		return model.ShortURL{}, false, nil
	}

	if err != nil {
		return model.ShortURL{}, false, fmt.Errorf("get short url: %w", err)
	}

	entry.CreateTime = createTime.UTC()

	return entry, true, nil
}

// PostgresShortURLCounter is a PostgreSQL-backed monotonic counter.
type PostgresShortURLCounter struct {
	db *sql.DB
}

// errCounterExhausted is returned when the counter exceeds the uint32 maximum.
var errCounterExhausted = errors.New("short url counter exhausted")

// Next returns the next counter value using an atomic upsert.
func (c *PostgresShortURLCounter) Next(ctx context.Context) (uint32, error) {
	var value int64

	err := c.db.QueryRowContext(
		ctx,
		`INSERT INTO counters (name, value) VALUES ($1, 1)
		 ON CONFLICT (name) DO UPDATE SET value = counters.value + 1
		 RETURNING value`,
		shortURLCounterName,
	).Scan(&value)
	if err != nil {
		return 0, fmt.Errorf("next counter: %w", err)
	}

	if value > math.MaxUint32 {
		return 0, errCounterExhausted
	}

	return uint32(value), nil //nolint:gosec // range checked above
}
