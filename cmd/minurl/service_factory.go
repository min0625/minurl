// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"
	"io"
	"log/slog"
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
		if strings.Contains(cfg.StorageDSN, "sslmode=disable") {
			slog.Warn(
				"PostgreSQL DSN contains sslmode=disable: " +
					"SSL is disabled and connections are unencrypted. " +
					"Use sslmode=require or sslmode=verify-full in production.",
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
