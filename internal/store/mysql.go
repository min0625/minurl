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

// mysqlErrDataTooLong is MySQL error 1406 (ER_DATA_TOO_LONG): a value was longer than
// the column it was written to.
const mysqlErrDataTooLong = 1406

// mysqlMaxOriginalURLBytes is the capacity of the TEXT original_url column.
// TestMySQLOriginalURLColumnMatchesMaxBytes pins it to the column it is copied from,
// so widening that column without updating this leaves a failing test rather than a
// limit that silently rejects URLs the column can now hold.
const mysqlMaxOriginalURLBytes = 65535

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
	cfg, err := parseMySQLDSN(dsn)
	if err != nil {
		return nil, err
	}

	connector, err := mysqldriver.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("open mysql database: %w", err)
	}

	db := sql.OpenDB(connector)

	db.SetMaxOpenConns(pool.MaxOpenConns)
	db.SetMaxIdleConns(pool.MaxIdleConns)
	db.SetConnMaxLifetime(pool.ConnMaxLifetime)
	db.SetConnMaxIdleTime(pool.ConnMaxIdleTime)

	if err := migrateMySQL(cfg); err != nil {
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
func migrateMySQL(cfg *mysqldriver.Config) error {
	migCfg := cfg.Clone()
	migCfg.MultiStatements = true

	connector, err := mysqldriver.NewConnector(migCfg)
	if err != nil {
		return fmt.Errorf("open mysql migration connection: %w", err)
	}

	migDB := sql.OpenDB(connector)

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

// parseMySQLDSN converts a mysql:// URL to a driver config for
// github.com/go-sql-driver/mysql, which does not accept URLs itself.
//
// mysql:// is MySQL's own URI-like connection string scheme, but this function implements
// only its shape, not its attribute vocabulary: MySQL reserves the query string for
// connection attributes (ssl-mode, connect-timeout, …) and forbids server variables there,
// while here it is the reverse. README — MySQL DSN query parameters spells that out for
// operators; do not "fix" a caller-supplied ssl-mode by making it work here without
// updating both.
//
// URL → config mapping:
//
//	mysql://user:pass@localhost:3306/dbname          → tcp(localhost:3306)/dbname
//	mysql://user:pass@localhost/dbname               → tcp(localhost:3306)/dbname, port defaulted
//	mysql://user:pass@localhost:3306/dbname?tls=true → tcp(localhost:3306)/dbname, TLS on
//
// parseTime and loc=UTC are always enforced; a caller-supplied value for either is
// dropped without error.
//
// The result is handed to mysqldriver.NewConnector rather than formatted back into a
// DSN string. FormatDSN writes cfg.Params into the query string, and re-parsing that
// string promotes any driver-level name among them — charset, interpolateParams,
// multiStatements, allowCleartextPasswords, and every other name ParseDSN recognises
// (all 24 of them in driver v1.10.0) — from a server variable back
// into a driver flag. Keeping the config means an unrecognised param stays what it is
// meant to be, a system variable sent as SET k = v, so a caller-supplied driver flag is
// rejected by the server at connect time instead of silently changing how the pool talks
// to MySQL.
func parseMySQLDSN(dsn string) (*mysqldriver.Config, error) {
	if !strings.HasPrefix(dsn, "mysql://") {
		return nil, fmt.Errorf("mysql dsn must start with mysql://: %q", dsn)
	}

	// Replace scheme so url.Parse can handle it correctly.
	raw := "https://" + strings.TrimPrefix(dsn, "mysql://")

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse mysql dsn %q: %w", dsn, err)
	}

	cfg := mysqldriver.NewConfig()

	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Passwd, _ = u.User.Password()
	}

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("mysql dsn missing host: %q", dsn)
	}

	port := u.Port()
	if port == "" {
		port = "3306"
	}

	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(host, port)

	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	if cfg.DBName == "" {
		return nil, fmt.Errorf("mysql dsn missing database name: %q", dsn)
	}

	cfg.ParseTime = true
	cfg.Loc = time.UTC

	// Forward any extra query parameters from the original URL.
	// tls is mapped to a Config field; the rest go into Params, which the driver sends
	// as SET k = v on connect — an unknown name fails the connection rather than being
	// silently dropped.
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

	return cfg, nil
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

	// original_url is TEXT, so it holds 65535 bytes. The limit is checked here rather
	// than left to the server: MySQL only raises ER_DATA_TOO_LONG in strict SQL mode, and
	// a server without STRICT_TRANS_TABLES silently truncates the value and reports the
	// row as inserted — the response would carry the full URL while the redirect served a
	// truncated one. Widening the column is a separate migration; until then an oversized
	// URL answers 4xx rather than 500.
	if len(entry.OriginalURL) > mysqlMaxOriginalURLBytes {
		return false, fmt.Errorf(
			"create short url: %w: %d bytes exceeds the %d byte column limit",
			service.ErrOriginalURLTooLong,
			len(entry.OriginalURL),
			mysqlMaxOriginalURLBytes,
		)
	}

	// ON DUPLICATE KEY UPDATE id = id suppresses the duplicate-key error and nothing else,
	// which is what INSERT IGNORE got wrong: IGNORE downgrades every error to a warning, so
	// an original_url longer than TEXT was stored silently truncated while the response
	// carried the full value, and any other insert failure surfaced as a bogus
	// "id already exists".
	//
	// Unlike the ON CONFLICT (id) DO NOTHING used by postgres.go and sqlite.go, which is
	// scoped to one index, ODKU fires on any unique key and MySQL offers no way to scope
	// it. The two are equivalent only while id is the sole unique key on short_urls, which
	// TestMySQLShortURLStorageHasNoSecondUniqueKey pins. A migration adding a second UNIQUE
	// index would make a violation of it return created=false, and the service would answer
	// 409 for a row that never conflicted on id; that migration must also switch this to a
	// plain INSERT plus a check that error 1062 names PRIMARY.
	//
	// Affected rows: 1 when inserted, 0 when the row already existed (id = id changes nothing).
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO short_urls (id, original_url, create_time, expire_time)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE id = id`,
		entry.ID,
		entry.OriginalURL,
		entry.CreateTime.UTC(),
		expireTime,
	)
	if err != nil {
		// 1406 means the value did not fit the column: a client input problem, not a
		// server fault. original_url is pre-checked above, so this is the backstop for
		// the other columns — an id longer than the VARCHAR(255) it is stored in, which
		// the request schema caps at MaxShortURLIDLen but the store does not enforce.
		//
		// The sentinel still names original_url, so a 1406 on id is answered with the
		// wrong message. Tolerated because the handler rejects an over-long id long
		// before the store sees one: reaching this line with one means a caller went
		// around the API. Give id its own sentinel if that validation ever moves.
		mysqlErr, ok := errors.AsType[*mysqldriver.MySQLError](err)
		if ok && mysqlErr.Number == mysqlErrDataTooLong {
			return false, fmt.Errorf("create short url: %w: %w", service.ErrOriginalURLTooLong, err)
		}

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
