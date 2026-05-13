// Copyright 2024 The MinURL Authors

package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/min0625/minurl/internal/telemetry"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	logFormatText = "text"
	logFormatJSON = "json"
)

// configKeys lists all configuration keys that should be bound from flags and environment variables.
var configKeys = []string{
	"http-addr",
	"id-seed",
	"storage-dsn",
	"log-format",
	"otel.enabled",
	"otel.service-name",
	"otel.exporter",
	"otel.endpoint",
	"otel.insecure",
	"db.max-open-conns",
	"db.max-idle-conns",
	"db.conn-max-lifetime",
	"db.conn-max-idle-time",
}

type appConfig struct {
	HTTPAddr        string
	IDSeed          string
	StorageDSN      string
	LogFormat       string
	OTELEnabled     bool
	OTELServiceName string
	OTELExporter    string
	OTELEndpoint    string
	OTELInsecure    bool
	// DB pool settings. These apply to the PostgreSQL and MySQL backends.
	// SQLite always uses a single connection regardless of these settings.
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	DBConnMaxIdleTime time.Duration
}

func defaultAppConfig() appConfig {
	return appConfig{
		HTTPAddr:        ":8888",
		StorageDSN:      "sqlite3://minurl.sqlite3",
		LogFormat:       logFormatText,
		OTELEnabled:     false,
		OTELServiceName: "minurl",
		OTELExporter:    telemetry.ExporterStdout,
		OTELEndpoint:    "",
		OTELInsecure:    true,
		// PostgreSQL connection pool defaults.
		// SQLite always uses 1 connection; these values are ignored for SQLite.
		DBMaxOpenConns:    25,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 30 * time.Minute,
		DBConnMaxIdleTime: 10 * time.Minute,
	}
}

func loadAppConfig(cmd *cobra.Command, configPath string) (appConfig, error) {
	cfg := defaultAppConfig()

	v := viper.New()
	v.SetEnvPrefix("MINURL")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	v.SetDefault("http-addr", cfg.HTTPAddr)
	v.SetDefault("storage-dsn", cfg.StorageDSN)
	v.SetDefault("log-format", cfg.LogFormat)
	v.SetDefault("otel.enabled", cfg.OTELEnabled)
	v.SetDefault("otel.service-name", cfg.OTELServiceName)
	v.SetDefault("otel.exporter", cfg.OTELExporter)
	v.SetDefault("otel.endpoint", cfg.OTELEndpoint)
	v.SetDefault("otel.insecure", cfg.OTELInsecure)
	v.SetDefault("db.max-open-conns", cfg.DBMaxOpenConns)
	v.SetDefault("db.max-idle-conns", cfg.DBMaxIdleConns)
	v.SetDefault("db.conn-max-lifetime", cfg.DBConnMaxLifetime.String())
	v.SetDefault("db.conn-max-idle-time", cfg.DBConnMaxIdleTime.String())

	if err := bindConfigFlags(v, cmd); err != nil {
		return appConfig{}, err
	}

	if configPath != "" {
		v.SetConfigFile(configPath)

		if err := v.ReadInConfig(); err != nil {
			return appConfig{}, fmt.Errorf("read config file %q: %w", configPath, err)
		}

		applyHyphenatedOTelConfigKeys(v, cmd)
		applyHyphenatedDBConfigKeys(v, cmd)
	}

	cfg.HTTPAddr = v.GetString("http-addr")
	cfg.IDSeed = strings.TrimSpace(v.GetString("id-seed"))
	cfg.StorageDSN = strings.TrimSpace(v.GetString("storage-dsn"))
	cfg.LogFormat = strings.ToLower(strings.TrimSpace(v.GetString("log-format")))
	cfg.OTELEnabled = v.GetBool("otel.enabled")
	cfg.OTELServiceName = strings.TrimSpace(v.GetString("otel.service-name"))
	cfg.OTELExporter = strings.ToLower(strings.TrimSpace(v.GetString("otel.exporter")))
	cfg.OTELEndpoint = strings.TrimSpace(v.GetString("otel.endpoint"))
	cfg.OTELInsecure = v.GetBool("otel.insecure")
	cfg.DBMaxOpenConns = v.GetInt("db.max-open-conns")
	cfg.DBMaxIdleConns = v.GetInt("db.max-idle-conns")

	dbConnMaxLifetime, err := parseDurationConfig(
		v.GetString("db.conn-max-lifetime"),
		"db.conn-max-lifetime",
	)
	if err != nil {
		return appConfig{}, err
	}

	dbConnMaxIdleTime, err := parseDurationConfig(
		v.GetString("db.conn-max-idle-time"),
		"db.conn-max-idle-time",
	)
	if err != nil {
		return appConfig{}, err
	}

	cfg.DBConnMaxLifetime = dbConnMaxLifetime
	cfg.DBConnMaxIdleTime = dbConnMaxIdleTime

	if cfg.HTTPAddr == "" {
		return appConfig{}, fmt.Errorf("http-addr must not be empty")
	}

	if cfg.IDSeed != "" {
		if _, err := parseUint32(cfg.IDSeed); err != nil {
			return appConfig{}, fmt.Errorf("parse id-seed: %w", err)
		}
	}

	if cfg.StorageDSN == "" {
		return appConfig{}, fmt.Errorf("storage-dsn must not be empty")
	}

	if _, err := detectStorageBackend(cfg.StorageDSN); err != nil {
		return appConfig{}, fmt.Errorf("storage-dsn: %w", err)
	}

	if cfg.DBMaxOpenConns < 0 {
		return appConfig{}, fmt.Errorf("db.max-open-conns must be >= 0, got %d", cfg.DBMaxOpenConns)
	}

	if cfg.DBMaxIdleConns < 0 {
		return appConfig{}, fmt.Errorf("db.max-idle-conns must be >= 0, got %d", cfg.DBMaxIdleConns)
	}

	switch cfg.LogFormat {
	case "", logFormatText, logFormatJSON:
		if cfg.LogFormat == "" {
			cfg.LogFormat = logFormatText
		}
	default:
		return appConfig{}, fmt.Errorf(
			"invalid log-format %q: expected %s or %s",
			cfg.LogFormat,
			logFormatText,
			logFormatJSON,
		)
	}

	if cfg.OTELEnabled {
		switch cfg.OTELExporter {
		case telemetry.ExporterStdout, telemetry.ExporterOTLP:
			if cfg.OTELExporter == telemetry.ExporterOTLP && cfg.OTELEndpoint == "" {
				return appConfig{}, fmt.Errorf("otel.endpoint must be set when otel.exporter=otlp")
			}
		default:
			return appConfig{}, fmt.Errorf(
				"invalid otel.exporter %q: expected %s or %s",
				cfg.OTELExporter,
				telemetry.ExporterStdout,
				telemetry.ExporterOTLP,
			)
		}
	}

	if cfg.OTELServiceName == "" {
		cfg.OTELServiceName = "minurl"
	}

	return cfg, nil
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

// parseDurationConfig parses a duration string from configuration.
// An empty string or "0" returns a zero duration (no limit).
// Returns an error if the string is not a valid Go duration or is negative.
func parseDurationConfig(raw, key string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf(
			"%s: invalid duration %q (expected a Go duration string, e.g. 30m, 1h): %w",
			key,
			raw,
			err,
		)
	}

	if d < 0 {
		return 0, fmt.Errorf("%s: duration must be >= 0, got %q", key, raw)
	}

	return d, nil
}
