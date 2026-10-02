// Copyright 2026 The MinURL Authors

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/min0625/minurl/internal/telemetry"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	yaml "go.yaml.in/yaml/v3"
)

const (
	appName          = "minurl"
	defaultSQLiteDSN = "sqlite3://minurl.sqlite3"
	logFormatText    = "text"
	logFormatJSON    = "json"
)

// configKeys lists every setting: the yaml tags of configFile, in order. Each is a flag name, its
// MINURL_* env var and its config file key; the flag's default is the setting's default.
var configKeys = func() []string {
	var keys []string
	for field := range reflect.TypeFor[configFile]().Fields() {
		keys = append(keys, field.Tag.Get("yaml"))
	}

	return keys
}()

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

// loadAppConfig reads every setting and checks it. lookupEnv is os.LookupEnv outside tests.
func loadAppConfig(
	cmd *cobra.Command,
	configPath string,
	lookupEnv func(string) (string, bool),
) (appConfig, error) {
	values, err := settingValues(cmd, configPath, lookupEnv)
	if err != nil {
		return appConfig{}, err
	}

	var cfg appConfig

	cfg.HTTPAddr = strings.TrimSpace(values["http-addr"])
	cfg.IDSeed = strings.TrimSpace(values["id-seed"])
	cfg.StorageDSN = strings.TrimSpace(values["storage-dsn"])
	cfg.LogFormat = strings.ToLower(strings.TrimSpace(values["log-format"]))
	cfg.OTELServiceName = strings.TrimSpace(values["otel-service-name"])
	cfg.OTELExporter = strings.ToLower(strings.TrimSpace(values["otel-exporter"]))
	cfg.OTELEndpoint = strings.TrimSpace(values["otel-endpoint"])

	// A value that does not parse fails startup instead of becoming 0 / false, which mean
	// something here (no connection limit, no idle connections, tracing off).
	if cfg.OTELEnabled, err = parseBoolConfig(values["otel-enabled"], "otel-enabled"); err != nil {
		return appConfig{}, err
	}

	if cfg.OTELInsecure, err = parseBoolConfig(values["otel-insecure"], "otel-insecure"); err != nil {
		return appConfig{}, err
	}

	if cfg.DBMaxOpenConns, err = parseIntConfig(values["db-max-open-conns"], "db-max-open-conns"); err != nil {
		return appConfig{}, err
	}

	if cfg.DBMaxIdleConns, err = parseIntConfig(values["db-max-idle-conns"], "db-max-idle-conns"); err != nil {
		return appConfig{}, err
	}

	dbConnMaxLifetime, err := parseDurationConfig(
		values["db-conn-max-lifetime"],
		"db-conn-max-lifetime",
	)
	if err != nil {
		return appConfig{}, err
	}

	dbConnMaxIdleTime, err := parseDurationConfig(
		values["db-conn-max-idle-time"],
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
		cfg.OTELServiceName = appName
	}

	return cfg, nil
}

// settingValues returns the text of every setting, from the first source that sets it.
func settingValues(
	cmd *cobra.Command,
	configPath string,
	lookupEnv func(string) (string, bool),
) (map[string]string, error) {
	var file map[string]string

	if configPath != "" {
		var err error
		if file, err = readConfigFile(configPath); err != nil {
			return nil, err
		}
	}

	values := make(map[string]string, len(configKeys))

	for _, key := range configKeys {
		f := cmd.Flag(key)
		if f == nil {
			return nil, fmt.Errorf("lookup flag %q: not found", key)
		}

		values[key] = setting(f, lookupEnv, file)
	}

	return values, nil
}

// setting returns a setting's value from the first source that sets it: the flag given on the
// command line, its MINURL_* env var, the config file, then the flag's default. An empty env var
// is unset, as it was in v0.0.2. One holding only whitespace is a value, which loadAppConfig
// trims to "": that fails startup for storage-dsn (rather than quietly starting on SQLite),
// http-addr, the pool sizes, booleans and durations, for otel-exporter when tracing is on and
// for otel-endpoint when the exporter is otlp, and means the default for id-seed, log-format and
// otel-service-name, as a blank value does in any source.
func setting(f *pflag.Flag, lookupEnv func(string) (string, bool), file map[string]string) string {
	if f.Changed {
		return f.Value.String()
	}

	env := "MINURL_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
	if v, ok := lookupEnv(env); ok && v != "" {
		return v
	}

	if v, ok := file[f.Name]; ok {
		return v
	}

	return f.DefValue
}

// readConfigFile reads the settings in a YAML config file.
func readConfigFile(configPath string) (map[string]string, error) {
	if ext := filepath.Ext(configPath); ext != ".yaml" && ext != ".yml" {
		return nil, fmt.Errorf("config file %q: must be YAML (.yaml or .yml)", configPath)
	}

	raw, err := os.ReadFile(configPath) //nolint:gosec // the operator names the file with --config
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", configPath, err)
	}

	values, err := configFileValues(raw)
	if err != nil {
		return nil, fmt.Errorf("config file %q: %w", configPath, err)
	}

	return values, nil
}

// configFile holds one *string field per setting, its yaml tag the setting's name (configKeys is
// built from these tags): the value as written in the config file, so it is read with the same
// rules as a flag or env var (08 and 1e3 are not integers, a string setting written 010 stays
// 010), or nil when the key is absent or null.
type configFile struct {
	HTTPAddr          *string `yaml:"http-addr"`
	IDSeed            *string `yaml:"id-seed"`
	StorageDSN        *string `yaml:"storage-dsn"`
	LogFormat         *string `yaml:"log-format"`
	OTELEnabled       *string `yaml:"otel-enabled"`
	OTELServiceName   *string `yaml:"otel-service-name"`
	OTELExporter      *string `yaml:"otel-exporter"`
	OTELEndpoint      *string `yaml:"otel-endpoint"`
	OTELInsecure      *string `yaml:"otel-insecure"`
	DBMaxOpenConns    *string `yaml:"db-max-open-conns"`
	DBMaxIdleConns    *string `yaml:"db-max-idle-conns"`
	DBConnMaxLifetime *string `yaml:"db-conn-max-lifetime"`
	DBConnMaxIdleTime *string `yaml:"db-conn-max-idle-time"`
}

// configFileValues returns the settings in a config file. settingProblems checks the top-level
// keys as written; yaml.v3's strict decoding then rejects a key given twice and checks what an
// alias or a merge key (<<) brings in, in its own words. minurl also requires one YAML document,
// as the decoder would otherwise drop a second one unread, typos and syntax errors included.
func configFileValues(raw []byte) (map[string]string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	if len(doc.Content) > 0 {
		if problems := settingProblems(doc.Content[0]); len(problems) > 0 {
			return nil, errors.New(strings.Join(problems, "; "))
		}
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var file configFile
	if err := dec.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		if typeErr, ok := errors.AsType[*yaml.TypeError](err); ok {
			return nil, errors.New(strings.Join(typeErr.Errors, "; "))
		}

		return nil, err
	}

	// A trailing --- with nothing after it is a null document and sets nothing.
	for {
		var next yaml.Node

		err := dec.Decode(&next)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, err
		}

		if len(next.Content) > 0 && next.Content[0].Tag != "!!null" {
			return nil, fmt.Errorf("line %d: only one YAML document is allowed", next.Line)
		}
	}

	values := make(map[string]string, len(configKeys))

	for field, value := range reflect.ValueOf(file).Fields() {
		if !value.IsNil() {
			values[field.Tag.Get("yaml")] = value.Elem().String()
		}
	}

	return values, nil
}

// settingProblems checks a config file's top-level keys as written, in minurl's words: each is
// a setting (yaml.v3 says "field idseed not found in type main.configFile", and skips a null key
// such as ~ instead of rejecting it), takes a single value, and has no tag of an application's
// own (!secret), which yaml.v3 would drop, leaving the text after it as the value. An alias key,
// a merge key (<<) and a list or mapping as a key are left to yaml.v3, which resolves them.
func settingProblems(root *yaml.Node) []string {
	switch {
	case root.Kind == yaml.ScalarNode && root.Tag == "!!null":
		return nil
	case root.Kind != yaml.MappingNode:
		return []string{fmt.Sprintf("line %d: the config file must hold key: value settings", root.Line)}
	}

	var problems []string

	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]

		switch {
		case key.Kind != yaml.ScalarNode, key.Tag == "!!merge":
		case !slices.Contains(configKeys, key.Value):
			problems = append(problems, fmt.Sprintf("line %d: %s", key.Line, unknownKey(key.Value)))
		case value.Kind == yaml.MappingNode || value.Kind == yaml.SequenceNode:
			problems = append(problems, fmt.Sprintf("line %d: %s takes a single value", key.Line, key.Value))
		case strings.HasPrefix(value.Tag, "!") && !strings.HasPrefix(value.Tag, "!!"):
			problems = append(
				problems,
				fmt.Sprintf("line %d: %s: tag %s is not supported", key.Line, key.Value, value.Tag),
			)
		}
	}

	return problems
}

// unknownKey describes a config file key that is not a setting, naming the setting meant when
// the key is one in another casing (ID-Seed) or in the nested or dotted form v0.0.2 also read
// (otel: {enabled: true}, db.max-open-conns).
func unknownKey(name string) string {
	lower := strings.ToLower(name)
	if slices.Contains(configKeys, lower) {
		return fmt.Sprintf("unknown key %q (keys are lowercase: %s)", name, lower)
	}

	flat := strings.ReplaceAll(lower, ".", "-")
	for _, setting := range configKeys {
		if setting == flat || strings.HasPrefix(setting, flat+"-") {
			return fmt.Sprintf("unknown key %q (settings are flat keys, such as %s)", name, setting)
		}
	}

	return fmt.Sprintf("unknown key %q", name)
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
// returning 0.
func parseIntConfig(raw, key string) (int, error) {
	raw = strings.TrimSpace(raw)

	n, err := strconv.ParseInt(raw, 0, strconv.IntSize)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q: %w", key, raw, err)
	}

	return int(n), nil
}

// parseBoolConfig parses a boolean setting with strconv.ParseBool. Unlike v0.0.2, which
// read it through viper's GetBool, it rejects what it cannot parse instead of returning false.
func parseBoolConfig(raw, key string) (bool, error) {
	raw = strings.TrimSpace(raw)

	b, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: invalid boolean %q (expected true or false): %w", key, raw, err)
	}

	return b, nil
}

// parseDurationConfig parses a duration setting as a Go duration string. "0" is a zero
// duration (no limit); a blank value is an error, as it is for pool sizes and booleans, rather
// than a silent 0.
func parseDurationConfig(raw, key string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)

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
