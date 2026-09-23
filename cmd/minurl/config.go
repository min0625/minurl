// Copyright 2026 The MinURL Authors

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/min0625/minurl/internal/telemetry"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	yaml "go.yaml.in/yaml/v3"
)

const (
	appName          = "minurl"
	defaultSQLiteDSN = "sqlite3://minurl.sqlite3"
	logFormatText    = "text"
	logFormatJSON    = "json"
)

// configKeys lists every setting. Each is a flag name, its MINURL_* env var and its config file key.
var configKeys = []string{
	"http-addr",
	"id-seed",
	"storage-dsn",
	"log-format",
	"otel-enabled",
	"otel-service-name",
	"otel-exporter",
	"otel-endpoint",
	"otel-insecure",
	"db-max-open-conns",
	"db-max-idle-conns",
	"db-conn-max-lifetime",
	"db-conn-max-idle-time",
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
		StorageDSN:      defaultSQLiteDSN,
		LogFormat:       logFormatText,
		OTELEnabled:     false,
		OTELServiceName: appName,
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
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	v.SetDefault("http-addr", cfg.HTTPAddr)
	v.SetDefault("storage-dsn", cfg.StorageDSN)
	v.SetDefault("log-format", cfg.LogFormat)
	v.SetDefault("otel-enabled", cfg.OTELEnabled)
	v.SetDefault("otel-service-name", cfg.OTELServiceName)
	v.SetDefault("otel-exporter", cfg.OTELExporter)
	v.SetDefault("otel-endpoint", cfg.OTELEndpoint)
	v.SetDefault("otel-insecure", cfg.OTELInsecure)
	v.SetDefault("db-max-open-conns", cfg.DBMaxOpenConns)
	v.SetDefault("db-max-idle-conns", cfg.DBMaxIdleConns)
	v.SetDefault("db-conn-max-lifetime", cfg.DBConnMaxLifetime.String())
	v.SetDefault("db-conn-max-idle-time", cfg.DBConnMaxIdleTime.String())

	if err := bindConfigFlags(v, cmd); err != nil {
		return appConfig{}, err
	}

	if configPath != "" {
		if err := readConfigFile(v, configPath); err != nil {
			return appConfig{}, err
		}
	}

	cfg.HTTPAddr = v.GetString("http-addr")
	cfg.IDSeed = strings.TrimSpace(v.GetString("id-seed"))
	cfg.StorageDSN = strings.TrimSpace(v.GetString("storage-dsn"))
	cfg.LogFormat = strings.ToLower(strings.TrimSpace(v.GetString("log-format")))
	cfg.OTELServiceName = strings.TrimSpace(v.GetString("otel-service-name"))
	cfg.OTELExporter = strings.ToLower(strings.TrimSpace(v.GetString("otel-exporter")))
	cfg.OTELEndpoint = strings.TrimSpace(v.GetString("otel-endpoint"))

	// viper's GetInt / GetBool turn a value they cannot parse into 0 / false without an
	// error, and both mean something here (no connection limit, no idle connections,
	// tracing off), so a typo in an env var or config file must fail startup instead.
	var err error

	if cfg.OTELEnabled, err = parseBoolConfig(v.GetString("otel-enabled"), "otel-enabled"); err != nil {
		return appConfig{}, err
	}

	if cfg.OTELInsecure, err = parseBoolConfig(v.GetString("otel-insecure"), "otel-insecure"); err != nil {
		return appConfig{}, err
	}

	if cfg.DBMaxOpenConns, err = parseIntConfig(v.GetString("db-max-open-conns"), "db-max-open-conns"); err != nil {
		return appConfig{}, err
	}

	if cfg.DBMaxIdleConns, err = parseIntConfig(v.GetString("db-max-idle-conns"), "db-max-idle-conns"); err != nil {
		return appConfig{}, err
	}

	dbConnMaxLifetime, err := parseDurationConfig(
		v.GetString("db-conn-max-lifetime"),
		"db-conn-max-lifetime",
	)
	if err != nil {
		return appConfig{}, err
	}

	dbConnMaxIdleTime, err := parseDurationConfig(
		v.GetString("db-conn-max-idle-time"),
		"db-conn-max-idle-time",
	)
	if err != nil {
		return appConfig{}, err
	}

	cfg.DBConnMaxLifetime = dbConnMaxLifetime
	cfg.DBConnMaxIdleTime = dbConnMaxIdleTime

	if cfg.HTTPAddr == "" {
		return appConfig{}, errors.New("http-addr must not be empty")
	}

	if cfg.IDSeed != "" {
		if _, err := parseUint32(cfg.IDSeed); err != nil {
			return appConfig{}, fmt.Errorf("parse id-seed: %w", err)
		}
	}

	if cfg.StorageDSN == "" {
		return appConfig{}, errors.New("storage-dsn must not be empty")
	}

	if _, err := detectStorageBackend(cfg.StorageDSN); err != nil {
		return appConfig{}, fmt.Errorf("storage-dsn: %w", err)
	}

	if cfg.DBMaxOpenConns < 0 {
		return appConfig{}, fmt.Errorf("db-max-open-conns must be >= 0, got %d", cfg.DBMaxOpenConns)
	}

	if cfg.DBMaxIdleConns < 0 {
		return appConfig{}, fmt.Errorf("db-max-idle-conns must be >= 0, got %d", cfg.DBMaxIdleConns)
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
				return appConfig{}, errors.New("otel-endpoint must be set when otel-exporter=otlp")
			}
		default:
			return appConfig{}, fmt.Errorf(
				"invalid otel-exporter %q: expected %s or %s",
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

// readConfigFile reads the config file once, checks its keys, and hands the same bytes to
// v, so the file that is checked is the file that is loaded.
func readConfigFile(v *viper.Viper, configPath string) error {
	if ext := filepath.Ext(configPath); ext != ".yaml" && ext != ".yml" {
		return fmt.Errorf("config file %q: must be YAML (.yaml or .yml)", configPath)
	}

	raw, err := os.ReadFile(configPath) //nolint:gosec // the operator names the file with --config
	if err != nil {
		return fmt.Errorf("read config file %q: %w", configPath, err)
	}

	if err := checkConfigFileKeys(raw); err != nil {
		return fmt.Errorf("config file %q: %w", configPath, err)
	}

	v.SetConfigType("yaml")

	if err := v.ReadConfig(bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("read config file %q: %w", configPath, err)
	}

	return nil
}

// checkConfigFileKeys fails on a config file key that is not a setting, so a typo (idseed,
// db-max-open-con) fails startup instead of leaving the default in place. A key is a flag
// name without "--" (db-max-open-conns); a nested key (db: {max-open-conns: 25}) or a dotted
// one (db.max-open-conns), which viper reads as the same nested key, is unknown. An unknown
// key is rejected even when null, and so is a list or mapping where a setting takes one
// value. Keys are lowercase: viper matches them case-insensitively, so ID-Seed would read
// as id-seed and a null ID-SEED: would replace id-seed: 1. A key given twice is rejected, as
// the YAML node tree does not catch it. A merge key (<<) is rejected too: viper expands it,
// and the check would have to do the same.
func checkConfigFileKeys(raw []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}

	// An empty file sets nothing; a document that is not a mapping is viper's to report.
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}

	var problems []string

	seen := make(map[string]int, len(configKeys))
	root := doc.Content[0]

	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		name := key.Value

		switch line, repeated := seen[name]; {
		case !slices.Contains(configKeys, name):
			problems = append(problems, fmt.Sprintf("line %d: %s", key.Line, unknownKey(key)))
		case repeated:
			problems = append(problems, fmt.Sprintf("line %d: %s is already set on line %d", key.Line, name, line))
		default:
			seen[name] = key.Line

			if value.Kind == yaml.MappingNode || value.Kind == yaml.SequenceNode {
				problems = append(problems, fmt.Sprintf("line %d: %s takes a single value", key.Line, name))
			}
		}
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}

	return nil
}

// unknownKey describes a config file key that is not a setting.
func unknownKey(key *yaml.Node) string {
	if key.Tag == "!!merge" {
		return "merge keys (<<) are not supported; write the keys out"
	}

	lower := strings.ToLower(key.Value)
	if slices.Contains(configKeys, lower) {
		return fmt.Sprintf("unknown key %q (keys are lowercase: %s)", key.Value, lower)
	}

	// otel: {enabled: true} and db.max-open-conns look like settings, so name one that is.
	flat := strings.ReplaceAll(lower, ".", "-")
	for _, setting := range configKeys {
		if setting == flat || strings.HasPrefix(setting, flat+"-") {
			return fmt.Sprintf("unknown key %q (settings are flat keys, such as %s)", key.Value, setting)
		}
	}

	return fmt.Sprintf("unknown key %q", key.Value)
}

// parseUint32 parses id-seed with the same Go integer literal rules as parseIntConfig.
func parseUint32(raw string) (uint32, error) {
	if raw == "" {
		return 0, errors.New("empty value")
	}

	v, err := strconv.ParseUint(raw, 0, 32)
	if err != nil {
		return 0, err
	}

	return uint32(v), nil
}

// parseIntConfig parses an integer setting as a Go integer literal (base 0): 0x hex, a
// leading 0 or 0o for octal, 0b binary and _ separators, as v0.0.2 did through viper's
// GetInt. Unlike GetInt it rejects what it cannot parse (08, 25.9, 1e3) instead of
// returning 0. An unquoted number in the config file reaches it already decoded by YAML,
// which agrees on 010 and 0x10 but reads 08, 25.0 and 1e3 as floats (8, 25, 1000).
func parseIntConfig(raw, key string) (int, error) {
	raw = strings.TrimSpace(raw)

	n, err := strconv.ParseInt(raw, 0, strconv.IntSize)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q: %w", key, raw, err)
	}

	return int(n), nil
}

// parseBoolConfig parses a boolean setting with strconv.ParseBool. Unlike viper's
// GetBool it rejects what it cannot parse instead of returning false.
func parseBoolConfig(raw, key string) (bool, error) {
	raw = strings.TrimSpace(raw)

	b, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: invalid boolean %q (expected true or false): %w", key, raw, err)
	}

	return b, nil
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
