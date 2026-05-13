// Copyright 2024 The MinURL Authors

package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/min0625/minurl/internal/service"
)

//go:embed migrations/mysql/*.sql
var mysqlMigrations embed.FS

// mysqlDBCloser wraps a shared sql.DB and closes it on Close.
type mysqlDBCloser struct {
	db *sql.DB
}

func (c *mysqlDBCloser) Close() error {
	return c.db.Close()
}

func (c *mysqlDBCloser) PingContext(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// NewMySQLBackends opens a MySQL connection pool and returns storage and
// counter backends that share the same pool.
//
// dsn must be a mysql:// URL:
//
//	mysql://user:password@localhost:3306/minurl
//	mysql://user:password@localhost:3306/minurl?tls=skip-verify
func NewMySQLBackends(
	dsn string,
	pool DBPoolConfig,
) (*MySQLShortURLStorage, *MySQLShortURLCounter, CloserPinger, error) {
	db, err := openMySQLDB(dsn, pool)
	if err != nil {
		return nil, nil, nil, err
	}

	storage := &MySQLShortURLStorage{db: db}
	counter := &MySQLShortURLCounter{db: db}
	closer := &mysqlDBCloser{db: db}

	return storage, counter, closer, nil
}

func openMySQLDB(dsn string, pool DBPoolConfig) (*sql.DB, error) {
	driverDSN, err := parseMySQLDSN(dsn)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql database: %w", err)
	}

	db.SetMaxOpenConns(pool.MaxOpenConns)
	db.SetMaxIdleConns(pool.MaxIdleConns)
	db.SetConnMaxLifetime(pool.ConnMaxLifetime)
	db.SetConnMaxIdleTime(pool.ConnMaxIdleTime)

	if err := migrateMySQL(driverDSN); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("migrate mysql database: %w", err)
	}

	return db, nil
}

// migrateMySQL applies pending migrations against the given MySQL database.
//
// golang-migrate's MySQL driver requires multiStatements=true on the connection
// used to execute migration files. We open a short-lived, dedicated connection
// with that flag set, run migrations, then close it. The main application pool
// (opened in openMySQLDB) deliberately omits multiStatements to avoid the
// security risks associated with that flag.
func migrateMySQL(driverDSN string) error {
	cfg, err := mysqldriver.ParseDSN(driverDSN)
	if err != nil {
		return fmt.Errorf("parse mysql dsn for migration: %w", err)
	}

	cfg.MultiStatements = true

	migDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("open mysql migration connection: %w", err)
	}

	defer migDB.Close() //nolint:errcheck // best-effort close of migration-only connection

	sourceDriver, err := iofs.New(mysqlMigrations, "migrations/mysql")
	if err != nil {
		return fmt.Errorf("create mysql migration source: %w", err)
	}

	dbDriver, err := migratemysql.WithInstance(migDB, &migratemysql.Config{})
	if err != nil {
		return fmt.Errorf("create mysql migration driver: %w", err)
	}

	return runMigrations(sourceDriver, dbDriver, "mysql")
}

// parseMySQLDSN converts a mysql:// URL to the driver DSN accepted by
// github.com/go-sql-driver/mysql.
//
// URL → driver DSN mapping:
//
//	mysql://user:pass@localhost:3306/dbname         → user:pass@tcp(localhost:3306)/dbname?parseTime=true
//	mysql://user:pass@localhost/dbname              → user:pass@tcp(localhost:3306)/dbname?parseTime=true
//	mysql://user:pass@localhost:3306/dbname?tls=true → user:pass@tcp(localhost:3306)/dbname?tls=true&parseTime=true
//
// parseTime and loc=UTC are always enforced.
func parseMySQLDSN(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "mysql://") {
		return "", fmt.Errorf("mysql dsn must start with mysql://: %q", dsn)
	}

	// Replace scheme so url.Parse can handle it correctly.
	raw := "https://" + strings.TrimPrefix(dsn, "mysql://")

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse mysql dsn %q: %w", dsn, err)
	}

	cfg := mysqldriver.NewConfig()

	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Passwd, _ = u.User.Password()
	}

	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("mysql dsn missing host: %q", dsn)
	}

	port := u.Port()
	if port == "" {
		port = "3306"
	}

	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(host, port)

	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	if cfg.DBName == "" {
		return "", fmt.Errorf("mysql dsn missing database name: %q", dsn)
	}

	cfg.ParseTime = true
	cfg.Loc = time.UTC

	// Forward any extra query parameters from the original URL.
	// Known driver-level params are mapped to Config fields; the rest go into Params.
	if u.RawQuery != "" {
		q := u.Query()
		extra := make(map[string]string, len(q))

		for k, v := range q {
			if len(v) == 0 {
				continue
			}

			switch k {
			case "tls":
				cfg.TLSConfig = v[0]
			case "parseTime", "loc":
				// Controlled by cfg.ParseTime / cfg.Loc — ignore caller-supplied values.
			default:
				extra[k] = v[0]
			}
		}

		if len(extra) > 0 {
			cfg.Params = extra
		}
	}

	return cfg.FormatDSN(), nil
}

// MySQLShortURLStorage is a MySQL-backed short URL storage.
type MySQLShortURLStorage struct {
	db *sql.DB
}

// CreateIfAbsent inserts the entry if the id does not already exist.
// Returns true if the row was inserted, false if it already existed.
func (s *MySQLShortURLStorage) CreateIfAbsent(
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
		`INSERT IGNORE INTO short_urls (id, original_url, create_time, expire_time)
		 VALUES (?, ?, ?, ?)`,
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
func (s *MySQLShortURLStorage) GetByID(
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
		`SELECT id, original_url, create_time, expire_time FROM short_urls WHERE id = ?`,
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

// MySQLShortURLCounter is a MySQL-backed monotonic counter.
type MySQLShortURLCounter struct {
	db *sql.DB
}

// Next returns the next counter value using an atomic upsert with LAST_INSERT_ID().
// A transaction is used to ensure both queries execute on the same connection so
// that the session-scoped LAST_INSERT_ID() value is visible to the SELECT.
func (c *MySQLShortURLCounter) Next(ctx context.Context) (uint64, error) {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}

	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO counters (name, value) VALUES (?, LAST_INSERT_ID(1))
		 ON DUPLICATE KEY UPDATE value = LAST_INSERT_ID(value + 1)`,
		shortURLCounterName,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert counter: %w", err)
	}

	var value int64

	if err := tx.QueryRowContext(ctx, `SELECT LAST_INSERT_ID()`).Scan(&value); err != nil {
		return 0, fmt.Errorf("read last insert id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	if value <= 0 {
		return 0, errCounterExhausted
	}

	return uint64(value), nil
}
