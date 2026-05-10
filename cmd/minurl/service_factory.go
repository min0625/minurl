// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"

	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/store"
)

const (
	backendSQLite   = "sqlite"
	backendPostgres = "postgres"
)

// detectStorageBackend infers the backend type from the DSN string.
// Returns an error for unrecognised schemes.
//
// Recognised forms:
//
//	sqlite3://path  → "sqlite"
//	postgres://...  → "postgres"
func detectStorageBackend(dsn string) (string, error) {
	switch {
	case strings.HasPrefix(dsn, "postgres://"):
		return backendPostgres, nil
	case strings.HasPrefix(dsn, "sqlite3://"):
		return backendSQLite, nil
	default:
		scheme := dsn
		if idx := strings.Index(dsn, "://"); idx >= 0 {
			scheme = dsn[:idx]
		}

		return "", fmt.Errorf(
			"unsupported storage DSN scheme %q: use sqlite3:// or postgres://",
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

func newShortURLServiceFromConfig(cfg appConfig) (*service.ShortURLService, io.Closer, error) {
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
		closer     io.Closer
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
