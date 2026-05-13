// Copyright 2024 The MinURL Authors

package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // register pgx driver
	"github.com/min0625/minurl/internal/service"
)

//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

// postgresDBCloser wraps a shared sql.DB and closes it on Close.
type postgresDBCloser struct {
	db *sql.DB
}

func (c *postgresDBCloser) Close() error {
	return c.db.Close()
}

func (c *postgresDBCloser) PingContext(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// NewPostgresBackends opens a PostgreSQL connection pool and returns storage
// and counter backends that share the same pool.
func NewPostgresBackends(
	dsn string,
	pool DBPoolConfig,
) (*PostgresShortURLStorage, *PostgresShortURLCounter, CloserPinger, error) {
	db, err := openPostgresDB(dsn, pool)
	if err != nil {
		return nil, nil, nil, err
	}

	storage := &PostgresShortURLStorage{db: db}
	counter := &PostgresShortURLCounter{db: db}
	closer := &postgresDBCloser{db: db}

	return storage, counter, closer, nil
}

func openPostgresDB(dsn string, pool DBPoolConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}

	db.SetMaxOpenConns(pool.MaxOpenConns)
	db.SetMaxIdleConns(pool.MaxIdleConns)
	db.SetConnMaxLifetime(pool.ConnMaxLifetime)
	db.SetConnMaxIdleTime(pool.ConnMaxIdleTime)

	if err := migratePostgres(db); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("migrate postgres database: %w", err)
	}

	return db, nil
}

func migratePostgres(db *sql.DB) error {
	sourceDriver, err := iofs.New(postgresMigrations, "migrations/postgres")
	if err != nil {
		return fmt.Errorf("create postgres migration source: %w", err)
	}

	dbDriver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{})
	if err != nil {
		return fmt.Errorf("create postgres migration driver: %w", err)
	}

	return runMigrations(sourceDriver, dbDriver, "postgres")
}

// PostgresShortURLStorage is a PostgreSQL-backed short URL storage.
type PostgresShortURLStorage struct {
	db *sql.DB
}

// CreateIfAbsent inserts the entry if the id does not already exist.
// Returns true if the row was inserted, false if it already existed.
func (s *PostgresShortURLStorage) CreateIfAbsent(
	ctx context.Context,
	entry service.ShortURL,
) (bool, error) {
	var expireTime *time.Time

	if entry.ExpireTime != nil {
		t := entry.ExpireTime.UTC()
		expireTime = &t
	}

	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO short_urls (id, original_url, create_time, expire_time)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO NOTHING`,
		entry.ID,
		entry.OriginalURL,
		entry.CreateTime.UTC(),
		expireTime,
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
) (service.ShortURL, bool, error) {
	var (
		entry      service.ShortURL
		createTime time.Time
		expireTime *time.Time
	)

	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, original_url, create_time, expire_time FROM short_urls WHERE id = $1`,
		id,
	).Scan(&entry.ID, &entry.OriginalURL, &createTime, &expireTime)

	if errors.Is(err, sql.ErrNoRows) {
		return service.ShortURL{}, false, nil
	}

	if err != nil {
		return service.ShortURL{}, false, fmt.Errorf("get short url: %w", err)
	}

	entry.CreateTime = createTime.UTC()

	if expireTime != nil {
		t := expireTime.UTC()
		entry.ExpireTime = &t
	}

	return entry, true, nil
}

// PostgresShortURLCounter is a PostgreSQL-backed monotonic counter.
type PostgresShortURLCounter struct {
	db *sql.DB
}

// errCounterExhausted is returned when the counter value is zero or negative,
// indicating data corruption or a BIGINT (int64) overflow in the database.
var errCounterExhausted = errors.New("short url counter exhausted")

// Next returns the next counter value using an atomic upsert.
func (c *PostgresShortURLCounter) Next(ctx context.Context) (uint64, error) {
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

	if value <= 0 {
		return 0, errCounterExhausted
	}

	return uint64(value), nil
}
