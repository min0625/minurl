// Copyright 2024 The MinURL Authors

package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/min0625/minurl/internal/service"
	"github.com/min0625/minurl/internal/store"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type appConfig struct {
	HTTPAddr    string
	IDSeed      string
	StoragePath string
}

func defaultAppConfig() appConfig {
	return appConfig{
		HTTPAddr:    ":8888",
		StoragePath: "minurl.sqlite3",
	}
}

func loadAppConfig(cmd *cobra.Command, configPath string) (appConfig, error) {
	cfg := defaultAppConfig()

	v := viper.New()
	v.SetEnvPrefix("MINURL")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	v.SetDefault("http-addr", cfg.HTTPAddr)
	v.SetDefault("storage-path", cfg.StoragePath)

	if err := bindConfigFlags(v, cmd); err != nil {
		return appConfig{}, err
	}

	if configPath != "" {
		v.SetConfigFile(configPath)

		if err := v.ReadInConfig(); err != nil {
			return appConfig{}, fmt.Errorf("read config file %q: %w", configPath, err)
		}
	}

	cfg.HTTPAddr = v.GetString("http-addr")
	cfg.IDSeed = strings.TrimSpace(v.GetString("id-seed"))
	cfg.StoragePath = strings.TrimSpace(v.GetString("storage-path"))

	if cfg.HTTPAddr == "" {
		return appConfig{}, fmt.Errorf("http-addr must not be empty")
	}

	if cfg.IDSeed != "" {
		if _, err := parseUint32(cfg.IDSeed); err != nil {
			return appConfig{}, fmt.Errorf("parse id-seed: %w", err)
		}
	}

	if cfg.StoragePath == "" {
		return appConfig{}, fmt.Errorf(
			"storage-path must not be empty",
		)
	}

	return cfg, nil
}

func bindConfigFlags(v *viper.Viper, cmd *cobra.Command) error {
	for _, key := range []string{"http-addr", "id-seed", "storage-path"} {
		f := lookupFlag(cmd, key)
		if f == nil {
			return fmt.Errorf("lookup flag %q: not found", key)
		}

		if err := v.BindPFlag(key, f); err != nil {
			return fmt.Errorf("bind flag %q: %w", key, err)
		}
	}

	for _, key := range []string{"http-addr", "id-seed", "storage-path"} {
		if err := v.BindEnv(key); err != nil {
			return fmt.Errorf("bind env %q: %w", key, err)
		}
	}

	return nil
}

func lookupFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if f := cmd.Flags().Lookup(name); f != nil {
		return f
	}

	if f := cmd.PersistentFlags().Lookup(name); f != nil {
		return f
	}

	if f := cmd.InheritedFlags().Lookup(name); f != nil {
		return f
	}

	return nil
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

	var storage service.ShortURLStorage

	var counter service.ShortURLCounter

	var closer io.Closer

	sqliteStore, sqliteCounter, sqliteCloser, err := store.NewSQLiteBackends(cfg.StoragePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite backends: %w", err)
	}

	storage = sqliteStore
	counter = sqliteCounter
	closer = sqliteCloser

	svc := service.NewShortURLServiceWithAllDependencies(storage, counter, idGen)

	return svc, closer, nil
}

func parseUint32(raw string) (uint32, error) {
	if raw == "" {
		return 0, fmt.Errorf("empty value")
	}

	v, err := strconv.ParseUint(raw, 0, 32)
	if err != nil {
		return 0, err
	}

	return uint32(v), nil
}
