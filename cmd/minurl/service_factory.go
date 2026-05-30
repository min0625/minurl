// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/store"
)

const (
	backendSQLite   = "sqlite"
	backendPostgres = "postgres"
	backendMySQL    = "mysql"
)

// detectStorageBackend infers the backend type from the DSN string.
// Returns an error for unrecognised schemes.
//
// Recognised forms:
//
//	sqlite3://path  → "sqlite"
//	postgres://...  → "postgres"
//	mysql://...     → "mysql"
func detectStorageBackend(dsn string) (string, error) {
	switch {
	case strings.HasPrefix(dsn, "postgres://"):
		return backendPostgres, nil
	case strings.HasPrefix(dsn, "sqlite3://"):
		return backendSQLite, nil
	case strings.HasPrefix(dsn, "mysql://"):
		return backendMySQL, nil
	default:
		scheme := dsn
		if before, _, ok := strings.Cut(dsn, "://"); ok {
			scheme = before
		}

		return "", fmt.Errorf(
			"unsupported storage DSN scheme %q: use sqlite3://, postgres://, or mysql://",
			scheme,
		)
	}
}

// postgresDSNSSLMode parses the PostgreSQL DSN as a URL and returns the value
// of the sslmode query parameter. Returns an empty string if the DSN cannot be
// parsed or if sslmode is not present.
func postgresDSNSSLMode(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}

	return u.Query().Get("sslmode")
}

// mysqlDSNTLSValue parses the MySQL DSN and returns the value of the tls
// query parameter. Returns an empty string if the DSN cannot be parsed or if
// tls is not present.
func mysqlDSNTLSValue(dsn string) string {
	// Replace scheme so url.Parse handles it as a URL.
	raw := "https://" + strings.TrimPrefix(dsn, "mysql://")

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return u.Query().Get("tls")
}

func newShortURLServiceFromConfig(
	cfg appConfig,
) (*service.ShortURLService, store.CloserPinger, error) {
	var idGen service.IDGenerator

	if cfg.IDSeed != "" {
		seed, err := parseUint32(cfg.IDSeed)
		if err != nil {
			return nil, nil, fmt.Errorf("parse id-seed: %w", err)
		}

		idGen = service.NewFeistelIDGeneratorWithSeed(seed)
	} else {
		idGen = service.NewDefaultFeistelIDGenerator()
	}

	var (
		svcStore   service.ShortURLStorage
		svcCounter service.ShortURLCounter
		closer     store.CloserPinger
	)

	backend, err := detectStorageBackend(cfg.StorageDSN)
	if err != nil {
		return nil, nil, err
	}

	switch backend {
	case backendPostgres:
		if strings.EqualFold(postgresDSNSSLMode(cfg.StorageDSN), "disable") {
			slog.Warn(
				"PostgreSQL DSN sslmode=disable: " +
					"SSL is disabled and connections are unencrypted. " +
					"Use sslmode=verify-full in production.",
			)
		}

		dbPool := store.DBPoolConfig{
			MaxOpenConns:    cfg.DBMaxOpenConns,
			MaxIdleConns:    cfg.DBMaxIdleConns,
			ConnMaxLifetime: cfg.DBConnMaxLifetime,
			ConnMaxIdleTime: cfg.DBConnMaxIdleTime,
		}

		pgStore, pgCounter, pgCloser, err := store.NewPostgresBackends(cfg.StorageDSN, dbPool)
		if err != nil {
			return nil, nil, fmt.Errorf("open postgres backends: %w", err)
		}

		svcStore, svcCounter, closer = pgStore, pgCounter, pgCloser
	case backendMySQL:
		if tlsVal := mysqlDSNTLSValue(cfg.StorageDSN); tlsVal == "false" || tlsVal == "" {
			slog.Warn(
				"MySQL DSN: TLS is not enabled — connections are unencrypted. " +
					"Set tls=true (or configure a named custom CA via RegisterTLSConfig) " +
					"in production. " +
					"tls=skip-verify encrypts traffic but does not verify the server " +
					"certificate and should only be used as a last resort.",
			)
		}

		dbPool := store.DBPoolConfig{
			MaxOpenConns:    cfg.DBMaxOpenConns,
			MaxIdleConns:    cfg.DBMaxIdleConns,
			ConnMaxLifetime: cfg.DBConnMaxLifetime,
			ConnMaxIdleTime: cfg.DBConnMaxIdleTime,
		}

		mysqlStore, mysqlCounter, mysqlCloser, err := store.NewMySQLBackends(cfg.StorageDSN, dbPool)
		if err != nil {
			return nil, nil, fmt.Errorf("open mysql backends: %w", err)
		}

		svcStore, svcCounter, closer = mysqlStore, mysqlCounter, mysqlCloser
	default: // sqlite
		sqliteStore, sqliteCounter, sqliteCloser, err := store.NewSQLiteBackends(cfg.StorageDSN)
		if err != nil {
			return nil, nil, fmt.Errorf("open sqlite backends: %w", err)
		}

		svcStore, svcCounter, closer = sqliteStore, sqliteCounter, sqliteCloser
	}

	svc, err := service.NewShortURLServiceWithAllDependencies(svcStore, svcCounter, idGen)
	if err != nil {
		_ = closer.Close()

		return nil, nil, fmt.Errorf("create short url service: %w", err)
	}

	return svc, closer, nil
}
