// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"
	"io"

	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/store"
)

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

	sqliteStore, sqliteCounter, sqliteCloser, err := store.NewSQLiteBackends(cfg.StoragePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite backends: %w", err)
	}

	svc, err := service.NewShortURLServiceWithAllDependencies(sqliteStore, sqliteCounter, idGen)
	if err != nil {
		_ = sqliteCloser.Close()

		return nil, nil, fmt.Errorf("create short url service: %w", err)
	}

	return svc, sqliteCloser, nil
}
