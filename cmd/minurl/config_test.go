// Copyright 2026 The MinURL Authors

package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/min0625/minurl/internal/service"
)

// noEnv stands in for os.LookupEnv, so a MINURL_* var in the shell running the tests cannot
// change their result.
func noEnv(string) (string, bool) { return "", false }

func envOf(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := vars[name]

		return v, ok
	}
}

// defaultConfig is every setting's default, as the README's configuration tables list them.
// The flags in main.go are the only place a default is written.
var defaultConfig = appConfig{
	HTTPAddr:          ":8888",
	StorageDSN:        "sqlite3://minurl.sqlite3",
	LogFormat:         "text",
	OTELServiceName:   "minurl",
	OTELExporter:      "stdout",
	OTELInsecure:      true,
	DBMaxOpenConns:    25,
	DBMaxIdleConns:    5,
	DBConnMaxLifetime: 30 * time.Minute,
	DBConnMaxIdleTime: 10 * time.Minute,
}

func TestLoadAppConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := loadAppConfig(newRootCommand(), "", noEnv)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if cfg != defaultConfig {
		t.Fatalf("loadAppConfig() = %+v, want %+v", cfg, defaultConfig)
	}
}

func TestLoadAppConfigTrimsHTTPAddr(t *testing.T) {
	t.Parallel()

	// The trailing newline of a Secret mounted as an env var is ignored, as for every setting.
	cfg, err := loadAppConfig(newRootCommand(), "", envOf(map[string]string{"MINURL_HTTP_ADDR": " :9000\n"}))
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if cfg.HTTPAddr != ":9000" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":9000")
	}

	_, err = loadAppConfig(newRootCommand(), "", envOf(map[string]string{"MINURL_HTTP_ADDR": " "}))
	if err == nil || !strings.Contains(err.Error(), "http-addr must not be empty") {
		t.Fatalf("loadAppConfig() error = %v, want http-addr must not be empty", err)
	}
}

func TestLoadAppConfigPrecedenceFlagOverEnvOverFile(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
	content := []byte(
		"http-addr: ':7000'\nid-seed: '11'\nstorage-dsn: 'sqlite3://from-file.sqlite3'\n",
	)

	if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	env := envOf(map[string]string{
		"MINURL_HTTP_ADDR":   ":8000",
		"MINURL_ID_SEED":     "22",
		"MINURL_STORAGE_DSN": "sqlite3://from-env.sqlite3",
	})

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("http-addr", ":9000"); err != nil {
		t.Fatalf("set http-addr flag: %v", err)
	}

	if err := cmd.PersistentFlags().Set("id-seed", "33"); err != nil {
		t.Fatalf("set id-seed flag: %v", err)
	}

	if err := cmd.PersistentFlags().Set("storage-dsn", "sqlite3://from-flag.sqlite3"); err != nil {
		t.Fatalf("set storage-dsn flag: %v", err)
	}

	if err := cmd.PersistentFlags().Set("log-format", "json"); err != nil {
		t.Fatalf("set log-format flag: %v", err)
	}

	cfg, err := loadAppConfig(cmd, cfgPath, env)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if cfg.HTTPAddr != ":9000" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":9000")
	}

	if cfg.IDSeed != "33" {
		t.Fatalf("IDSeed = %q, want %q", cfg.IDSeed, "33")
	}

	if cfg.StorageDSN != "sqlite3://from-flag.sqlite3" {
		t.Fatalf("StorageDSN = %q, want %q", cfg.StorageDSN, "sqlite3://from-flag.sqlite3")
	}

	if cfg.LogFormat != "json" {
		t.Fatalf("LogFormat = %q, want %q", cfg.LogFormat, "json")
	}

	// Without the flags, the env vars win over the file.
	cfg, err = loadAppConfig(newRootCommand(), cfgPath, env)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if cfg.HTTPAddr != ":8000" || cfg.IDSeed != "22" {
		t.Fatalf("HTTPAddr, IDSeed = %q, %q, want the env values", cfg.HTTPAddr, cfg.IDSeed)
	}
}

func TestLoadAppConfigRejectsInvalidLogFormat(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("log-format", "xml"); err != nil {
		t.Fatalf("set log-format flag: %v", err)
	}

	if _, err := loadAppConfig(cmd, "", noEnv); err == nil {
		t.Fatal("loadAppConfig() error = nil, want non-nil")
	}
}

func TestLoadAppConfigOTelEnvOverridesFile(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "otel-env.yaml")
	content := []byte(
		"http-addr: ':7000'\notel-enabled: false\notel-exporter: stdout\notel-endpoint: http://file:4318\n",
	)

	if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	env := envOf(map[string]string{
		"MINURL_OTEL_ENABLED":  "true",
		"MINURL_OTEL_EXPORTER": "otlp",
		"MINURL_OTEL_ENDPOINT": "http://env:4318",
	})

	cfg, err := loadAppConfig(newRootCommand(), cfgPath, env)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if !cfg.OTELEnabled {
		t.Fatalf("OTELEnabled = %v, want true", cfg.OTELEnabled)
	}

	if cfg.OTELExporter != "otlp" {
		t.Fatalf("OTELExporter = %q, want %q", cfg.OTELExporter, "otlp")
	}

	if cfg.OTELEndpoint != "http://env:4318" {
		t.Fatalf("OTELEndpoint = %q, want %q", cfg.OTELEndpoint, "http://env:4318")
	}
}

func TestLoadAppConfigSkipsOTelValidationWhenDisabled(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("otel-enabled", "false"); err != nil {
		t.Fatalf("set otel-enabled flag: %v", err)
	}

	if err := cmd.PersistentFlags().Set("otel-exporter", "invalid"); err != nil {
		t.Fatalf("set otel-exporter flag: %v", err)
	}

	if _, err := loadAppConfig(cmd, "", noEnv); err != nil {
		t.Fatalf("loadAppConfig() error = %v, want nil", err)
	}
}

func TestLoadAppConfigRejectsInvalidSeed(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("id-seed", "not-a-number"); err != nil {
		t.Fatalf("set id-seed flag: %v", err)
	}

	if _, err := loadAppConfig(cmd, "", noEnv); err == nil {
		t.Fatal("loadAppConfig() error = nil, want non-nil")
	}
}

func TestNewShortURLServiceFromConfigUsesConfiguredSeed(t *testing.T) {
	t.Parallel()

	svcA, closerA, err := newShortURLServiceFromConfig(appConfig{
		IDSeed:     "1234",
		StorageDSN: "sqlite3:///" + filepath.Join(t.TempDir(), "a.sqlite3"),
	})
	if err != nil {
		t.Fatalf("newShortURLServiceFromConfig(seed 1234) error = %v", err)
	}

	defer func() {
		if closeErr := closerA.Close(); closeErr != nil {
			t.Fatalf("close closerA: %v", closeErr)
		}
	}()

	svcB, closerB, err := newShortURLServiceFromConfig(appConfig{
		IDSeed:     "1234",
		StorageDSN: "sqlite3:///" + filepath.Join(t.TempDir(), "b.sqlite3"),
	})
	if err != nil {
		t.Fatalf("newShortURLServiceFromConfig(seed 1234 second) error = %v", err)
	}

	defer func() {
		if closeErr := closerB.Close(); closeErr != nil {
			t.Fatalf("close closerB: %v", closeErr)
		}
	}()

	svcC, closerC, err := newShortURLServiceFromConfig(appConfig{
		IDSeed:     "9999",
		StorageDSN: "sqlite3:///" + filepath.Join(t.TempDir(), "c.sqlite3"),
	})
	if err != nil {
		t.Fatalf("newShortURLServiceFromConfig(seed 9999) error = %v", err)
	}

	defer func() {
		if closeErr := closerC.Close(); closeErr != nil {
			t.Fatalf("close closerC: %v", closeErr)
		}
	}()

	a, err := svcA.Create(
		context.Background(),
		service.ShortURL{OriginalURL: "https://example.org/a"},
	)
	if err != nil {
		t.Fatalf("svcA.Create() error = %v", err)
	}

	b, err := svcB.Create(
		context.Background(),
		service.ShortURL{OriginalURL: "https://example.org/b"},
	)
	if err != nil {
		t.Fatalf("svcB.Create() error = %v", err)
	}

	c, err := svcC.Create(
		context.Background(),
		service.ShortURL{OriginalURL: "https://example.org/c"},
	)
	if err != nil {
		t.Fatalf("svcC.Create() error = %v", err)
	}

	if a.ID != b.ID {
		t.Fatalf("same seed first id differs: %q != %q", a.ID, b.ID)
	}

	if a.ID == c.ID {
		t.Fatalf("different seed first id should differ: %q == %q", a.ID, c.ID)
	}
}

func TestLoadAppConfigRejectsEmptyStorageDSN(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("storage-dsn", ""); err != nil {
		t.Fatalf("set storage-dsn flag: %v", err)
	}

	if _, err := loadAppConfig(cmd, "", noEnv); err == nil {
		t.Fatal("loadAppConfig() error = nil, want non-nil for empty storage-dsn")
	}
}

func TestNewShortURLServiceFromConfigSQLitePersists(t *testing.T) {
	t.Parallel()

	dbPath := "sqlite3:///" + t.TempDir() + "/test.sqlite3"

	cfg := appConfig{StorageDSN: dbPath, IDSeed: "7"}

	svc, closer, err := newShortURLServiceFromConfig(cfg)
	if err != nil {
		t.Fatalf("newShortURLServiceFromConfig() error = %v", err)
	}

	entry, err := svc.Create(
		t.Context(),
		service.ShortURL{OriginalURL: "https://example.com"},
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if closeErr := closer.Close(); closeErr != nil {
		t.Fatalf("close closer: %v", closeErr)
	}

	// Reopen the same database and verify the entry is still there.
	svc2, closer2, err := newShortURLServiceFromConfig(cfg)
	if err != nil {
		t.Fatalf("newShortURLServiceFromConfig() second open error = %v", err)
	}

	defer func() {
		if closeErr := closer2.Close(); closeErr != nil {
			t.Fatalf("close closer2: %v", closeErr)
		}
	}()

	got, err := svc2.Get(t.Context(), entry.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.OriginalURL != "https://example.com" {
		t.Fatalf("OriginalURL = %q, want %q", got.OriginalURL, "https://example.com")
	}

	entry2, err := svc2.Create(
		t.Context(),
		service.ShortURL{OriginalURL: "https://example.org/another"},
	)
	if err != nil {
		t.Fatalf("Create() second error = %v", err)
	}

	if entry2.ID == entry.ID {
		t.Fatalf(
			"counter did not persist across restart: second ID %q equals first ID %q",
			entry2.ID,
			entry.ID,
		)
	}
}

func TestLoadAppConfigRejectsInvalidStorageBackend(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("storage-dsn", "mongodb://localhost/db"); err != nil {
		t.Fatalf("set storage-dsn flag: %v", err)
	}

	if _, err := loadAppConfig(cmd, "", noEnv); err == nil {
		t.Fatal("loadAppConfig() error = nil, want non-nil for unknown DSN scheme")
	}
}

func TestLoadAppConfigRejectsPostgresWithoutDSN(t *testing.T) {
	t.Parallel()

	// An empty storage-dsn is rejected regardless of the intended backend.
	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("storage-dsn", ""); err != nil {
		t.Fatalf("set storage-dsn flag: %v", err)
	}

	if _, err := loadAppConfig(cmd, "", noEnv); err == nil {
		t.Fatal("loadAppConfig() error = nil, want non-nil for empty storage-dsn")
	}
}

func TestLoadAppConfigAcceptsPostgresWithDSN(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().
		Set("storage-dsn", "postgres://localhost:5432/minurl?sslmode=disable"); err != nil {
		t.Fatalf("set storage-dsn flag: %v", err)
	}

	cfg, err := loadAppConfig(cmd, "", noEnv)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v, want nil", err)
	}

	if cfg.StorageDSN != "postgres://localhost:5432/minurl?sslmode=disable" {
		t.Fatalf(
			"StorageDSN = %q, want %q",
			cfg.StorageDSN,
			"postgres://localhost:5432/minurl?sslmode=disable",
		)
	}
}

func TestLoadAppConfigAcceptsMySQLWithDSN(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().
		Set("storage-dsn", "mysql://user:pass@localhost:3306/minurl"); err != nil {
		t.Fatalf("set storage-dsn flag: %v", err)
	}

	cfg, err := loadAppConfig(cmd, "", noEnv)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v, want nil", err)
	}

	if cfg.StorageDSN != "mysql://user:pass@localhost:3306/minurl" { //nolint:gosec // test credentials
		t.Fatalf(
			"StorageDSN = %q, want %q",
			cfg.StorageDSN,
			"mysql://user:pass@localhost:3306/minurl",
		)
	}
}

func TestLoadAppConfigStorageBackendDefaultsSQLite(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()

	cfg, err := loadAppConfig(cmd, "", noEnv)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if cfg.StorageDSN != "sqlite3://minurl.sqlite3" {
		t.Fatalf("StorageDSN = %q, want %q", cfg.StorageDSN, "sqlite3://minurl.sqlite3")
	}

	backend, err := detectStorageBackend(cfg.StorageDSN)
	if err != nil {
		t.Fatalf("detectStorageBackend(%q) error = %v", cfg.StorageDSN, err)
	}

	if backend != "sqlite" {
		t.Fatalf("detectStorageBackend(%q) = %q, want sqlite", cfg.StorageDSN, backend)
	}
}

func TestLoadAppConfigRejectsUnparsableIntAndBoolEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		env   string
		value string
	}{
		{"MINURL_DB_MAX_OPEN_CONNS", "abc"},
		{"MINURL_DB_MAX_OPEN_CONNS", "25abc"},
		{"MINURL_DB_MAX_OPEN_CONNS", "25.9"},
		{"MINURL_DB_MAX_IDLE_CONNS", "08"},
		{"MINURL_DB_MAX_IDLE_CONNS", "0x"},
		{"MINURL_OTEL_ENABLED", "yes"},
		{"MINURL_OTEL_INSECURE", "abc"},
		// Only whitespace is a value, unlike an empty env var.
		{"MINURL_DB_MAX_OPEN_CONNS", " "},
		{"MINURL_OTEL_INSECURE", "\t"},
	}

	for _, tt := range tests {
		t.Run(tt.env+"="+tt.value, func(t *testing.T) {
			t.Parallel()

			env := envOf(map[string]string{tt.env: tt.value})
			if _, err := loadAppConfig(newRootCommand(), "", env); err == nil {
				t.Fatal("loadAppConfig() error = nil, want non-nil")
			}
		})
	}
}

func TestLoadAppConfigRejectsUnparsableIntAndBoolFile(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		"db-max-open-conns: abc",
		"db-max-idle-conns: ''",
		"otel-enabled: yes",
		"otel-insecure: 'on'",
		// The file is read as written, not decoded by YAML first, which would read these
		// as the floats 8, 25 and 1000.
		"db-max-open-conns: 08",
		"db-max-open-conns: 25.0",
		"db-max-open-conns: 1e3",
		"id-seed: 1e3",
	} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()

			cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
			if err := os.WriteFile(cfgPath, []byte(line+"\n"), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			if _, err := loadAppConfig(newRootCommand(), cfgPath, noEnv); err == nil {
				t.Fatal("loadAppConfig() error = nil, want non-nil")
			}
		})
	}
}

func TestLoadAppConfigParsesIntAndBoolStrictly(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
	if err := os.WriteFile(cfgPath, []byte("db-max-idle-conns: 010\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	// A leading 0 is octal in an env var as in the config file, and surrounding space is
	// trimmed.
	env := envOf(map[string]string{
		"MINURL_DB_MAX_OPEN_CONNS": " 010 ",
		"MINURL_OTEL_INSECURE":     "TRUE",
	})

	cfg, err := loadAppConfig(newRootCommand(), cfgPath, env)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if cfg.DBMaxOpenConns != 8 {
		t.Fatalf("DBMaxOpenConns = %d, want 8", cfg.DBMaxOpenConns)
	}

	if cfg.DBMaxIdleConns != 8 {
		t.Fatalf("DBMaxIdleConns = %d, want 8", cfg.DBMaxIdleConns)
	}

	if !cfg.OTELInsecure {
		t.Fatalf("OTELInsecure = %v, want true", cfg.OTELInsecure)
	}

	if cfg.OTELEnabled {
		t.Fatalf("OTELEnabled = %v, want the default false", cfg.OTELEnabled)
	}
}

func TestParseIntegerSettingsAsGoLiterals(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		raw  string
		want int
	}{
		{"0", 0},
		{"10", 10},
		{"010", 8},
		{"0o10", 8},
		{"0x10", 16},
		{"0X10", 16},
		{"0b10", 2},
		{"1_000", 1000},
	} {
		if got, err := parseIntConfig(tt.raw, "key"); err != nil || got != tt.want {
			t.Errorf("parseIntConfig(%q) = %d, %v, want %d", tt.raw, got, err, tt.want)
		}

		if got, err := parseUint32(tt.raw); err != nil || int(got) != tt.want {
			t.Errorf("parseUint32(%q) = %d, %v, want %d", tt.raw, got, err, tt.want)
		}
	}

	for _, raw := range []string{"08", "0x", "_1", "1e3", "25.9"} {
		if _, err := parseIntConfig(raw, "key"); err == nil {
			t.Errorf("parseIntConfig(%q) error = nil, want non-nil", raw)
		}

		if _, err := parseUint32(raw); err == nil {
			t.Errorf("parseUint32(%q) error = nil, want non-nil", raw)
		}
	}

	if got, err := parseUint32("0xFFFFFFFF"); err != nil || got != 1<<32-1 {
		t.Errorf("parseUint32(0xFFFFFFFF) = %d, %v, want %d", got, err, uint32(1<<32-1))
	}

	for _, raw := range []string{"-1", "4294967296"} {
		if _, err := parseUint32(raw); err == nil {
			t.Errorf("parseUint32(%q) error = nil, want non-nil", raw)
		}
	}
}

func TestLoadAppConfigRejectsInvalidConfigFileKeys(t *testing.T) {
	t.Parallel()

	// minurl checks the top-level keys as written and names the setting meant where it can;
	// yaml.v3 reports a key given twice and what an alias or a merge key brings in.
	unknown := func(key string) string { return `unknown key "` + key + `"` }
	library := func(key string) string { return "field " + key + " not found in type main.configFile" }
	flat := func(key, setting string) string {
		return unknown(key) + " (settings are flat keys, such as " + setting + ")"
	}

	for _, tc := range []struct{ name, file, content, want string }{
		{"unknown key", "minurl.yaml", "idseed: 5\n", "line 1: " + unknown("idseed")},
		{"unknown null key", "minurl.yaml", "idseed:\n", "line 1: " + unknown("idseed")},
		{"unknown flat key", "minurl.yaml", "db-max-open-con: 5\n", "line 1: " + unknown("db-max-open-con")},
		// v0.0.2 read the nested form, so the error names a flat key to use.
		{"nested key", "minurl.yaml", "http-addr: ':80'\ndb:\n  max-open-conns: 5\n", "line 2: " + flat("db", "db-max-open-conns")},
		{"null section", "minurl.yaml", "otel:\n", "line 1: " + flat("otel", "otel-enabled")},
		{"section as a value", "minurl.yaml", "otel: true\n", "line 1: " + flat("otel", "otel-enabled")},
		{"dotted key", "minurl.yaml", "db.max-open-conns: 5\n", "line 1: " + flat("db.max-open-conns", "db-max-open-conns")},
		{"flag that is not a setting", "minurl.yaml", "config: other.yaml\n", "line 1: " + unknown("config")},
		{"every problem", "minurl.yaml", "idseed: 5\nfoo: 1\n", "line 1: " + unknown("idseed") + "; line 2: " + unknown("foo")},
		{"merge key with an unknown key", "minurl.yaml", "<<: {idseed: 1}\n", "line 1: " + library("idseed")},
		{"merge key list with an unknown key", "minurl.yaml", "<<: [{log-format: json}, {idseed: 1}]\n", "line 1: " + library("idseed")},
		// yaml.v3 skips a null key instead of rejecting it as unknown.
		{"null key", "minurl.yaml", "http-addr: ':80'\n~: [1, 2]\n", "line 2: " + unknown("~")},
		{"null word key", "minurl.yaml", "null: 5\n", "line 1: " + unknown("null")},
		{"uppercase null key", "minurl.yaml", "NULL: {a: 1}\n", "line 1: " + unknown("NULL")},
		{"list as a key", "minurl.yaml", "? [a, b]\n: 1\n", "line 1: cannot unmarshal !!seq into string"},
		// yaml.v3 would drop an application's own tag and read the text after it.
		{"custom tag", "minurl.yaml", "otel-endpoint: !secret collector\n", "line 1: otel-endpoint: tag !secret is not supported"},
		// A quoted << is a plain key, not a merge key.
		{"quoted <<", "minurl.yaml", "\"<<\": 1\n", "line 1: " + unknown("<<")},
		{"mapping value", "minurl.yaml", "db-max-open-conns:\n  foo: 1\n", "line 1: db-max-open-conns takes a single value"},
		{"list value", "minurl.yaml", "http-addr: [':80']\n", "line 1: http-addr takes a single value"},
		{"given twice", "minurl.yaml", "id-seed: 1\nid-seed: 2\n", `line 2: mapping key "id-seed" already defined at line 1`},
		{"uppercase", "minurl.yaml", "HTTP-Addr: ':80'\n", "line 1: " + unknown("HTTP-Addr") + " (keys are lowercase: http-addr)"},
		// v0.0.2 folded casings into one key, so a null ID-SEED: replaced the value.
		{"uppercase null after the key", "minurl.yaml", "id-seed: 1\nID-SEED:\n", "line 2: " + unknown("ID-SEED") + " (keys are lowercase: id-seed)"},
		{"uppercase section", "minurl.yaml", "OTEL:\n", "line 1: " + flat("OTEL", "otel-enabled")},
		{"JSON", "minurl.json", `{"id-seed": 5}`, "must be YAML (.yaml or .yml)"},
		{"TOML", "minurl.toml", "id-seed = 5\n", "must be YAML (.yaml or .yml)"},
		{"not a mapping", "minurl.yaml", "id-seed\n", "line 1: the config file must hold key: value settings"},
		{"list document", "minurl.yaml", "- id-seed: 1\n", "line 1: the config file must hold key: value settings"},
		{"alias to a mapping", "minurl.yaml", "http-addr: &m {a: 1}\nlog-format: *m\n", "line 1: http-addr takes a single value"},
		// An alias key is the key it points at, so it can repeat one, which decoding into a map
		// does not catch; decoding into the struct does.
		{"alias key given twice", "minurl.yaml", "&k id-seed: 1\n*k : 2\n", "line 2: field id-seed already set in type main.configFile"},
		{"alias key to an unknown key", "minurl.yaml", "otel-endpoint: &k idseed\n*k : 2\n", "line 2: " + library("idseed")},
		{"tag that does not match the value", "minurl.yaml", "otel-service-name: !!int abc\n", "cannot decode !!str `abc` as a !!int"},
		// Only the first YAML document used to be read, so anything after --- went unchecked.
		{"second document", "minurl.yaml", "log-format: text\n---\nidseed: 1\n", "line 2: only one YAML document is allowed"},
		{"syntax error in a second document", "minurl.yaml", "log-format: text\n---\nkey: [unclosed\n", "did not find expected"},
		{"document after a null one", "minurl.yaml", "log-format: text\n---\n---\nid-seed: 1\n", "line 3: only one YAML document is allowed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfgPath := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(cfgPath, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			_, err := loadAppConfig(newRootCommand(), cfgPath, noEnv)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadAppConfig() error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestLoadAppConfigReadsEveryConfigFileKey(t *testing.T) {
	t.Parallel()

	// Every value differs from its default, so a key that is accepted but not read fails.
	want := appConfig{
		HTTPAddr:          ":9090",
		IDSeed:            "5",
		StorageDSN:        "sqlite3://file.sqlite3",
		LogFormat:         "json",
		OTELEnabled:       true,
		OTELServiceName:   "svc",
		OTELExporter:      "otlp",
		OTELEndpoint:      "collector:4317",
		OTELInsecure:      false,
		DBMaxOpenConns:    7,
		DBMaxIdleConns:    3,
		DBConnMaxLifetime: time.Minute,
		DBConnMaxIdleTime: 2 * time.Minute,
	}
	top := "http-addr: ':9090'\nid-seed: 5\nstorage-dsn: sqlite3://file.sqlite3\nlog-format: json\n"

	for _, tc := range []struct{ name, file, content string }{
		{".yml", "minurl.yml", top + `otel-enabled: true
otel-service-name: svc
otel-exporter: otlp
otel-endpoint: collector:4317
otel-insecure: false
db-max-open-conns: 7
db-max-idle-conns: 3
db-conn-max-lifetime: 1m
db-conn-max-idle-time: 2m
`},
		// JSON is YAML, so a .json file from v0.0.2 works renamed to .yaml.
		{"JSON renamed", "minurl.yaml", `{
	"http-addr": ":9090", "id-seed": 5, "storage-dsn": "sqlite3://file.sqlite3", "log-format": "json",
	"otel-enabled": true, "otel-service-name": "svc", "otel-exporter": "otlp",
	"otel-endpoint": "collector:4317", "otel-insecure": false,
	"db-max-open-conns": 7, "db-max-idle-conns": 3,
	"db-conn-max-lifetime": "1m", "db-conn-max-idle-time": "2m"
}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfgPath := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(cfgPath, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			cfg, err := loadAppConfig(newRootCommand(), cfgPath, noEnv)
			if err != nil {
				t.Fatalf("loadAppConfig() error = %v", err)
			}

			if cfg != want {
				t.Fatalf("loadAppConfig() = %+v, want %+v", cfg, want)
			}
		})
	}
}

func TestLoadAppConfigIgnoresNullConfigFileKeys(t *testing.T) {
	t.Parallel()

	for _, content := range []string{
		"",
		"http-addr:\ndb-max-open-conns:\notel-enabled:\n",
		"http-addr: &n\ndb-max-open-conns: *n\n",
		"~\n",
		"# comments only\n",
		"http-addr:\n---\n# a trailing document that holds nothing\n",
		"---\n---\n",
		"log-format: !!null ''\n",
	} {
		t.Run(content, func(t *testing.T) {
			t.Parallel()

			cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
			if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			cfg, err := loadAppConfig(newRootCommand(), cfgPath, noEnv)
			if err != nil {
				t.Fatalf("loadAppConfig() error = %v", err)
			}

			if cfg != defaultConfig {
				t.Fatalf("loadAppConfig() = %+v, want the defaults %+v", cfg, defaultConfig)
			}
		})
	}
}

func TestLoadAppConfigReadsConfigExample(t *testing.T) {
	t.Parallel()

	if _, err := loadAppConfig(newRootCommand(), "../../config.example.yaml", noEnv); err != nil {
		t.Fatalf("loadAppConfig(config.example.yaml) error = %v", err)
	}
}

func TestLoadAppConfigParsesDurationsStrictly(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{"zero", "0", 0, false},
		{"surrounding whitespace", " 45s\n", 45 * time.Second, false},
		// An empty env var is unset, as for every setting, and keeps the default.
		{"empty", "", 30 * time.Minute, false},
		// A blank value used to mean 0, no limit; pool sizes and booleans reject it.
		{"blank", " \n", 0, true},
		{"not a duration", "abc", 0, true},
		{"negative", "-1m", 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := envOf(map[string]string{"MINURL_DB_CONN_MAX_LIFETIME": tt.value})

			cfg, err := loadAppConfig(newRootCommand(), "", env)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "db-conn-max-lifetime") {
					t.Fatalf("loadAppConfig() error = %v, want one naming db-conn-max-lifetime", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("loadAppConfig() error = %v", err)
			}

			if cfg.DBConnMaxLifetime != tt.want {
				t.Fatalf("DBConnMaxLifetime = %v, want %v", cfg.DBConnMaxLifetime, tt.want)
			}
		})
	}
}

func TestLoadAppConfigReadsBlankDurationFlagAndFile(t *testing.T) {
	t.Parallel()

	t.Run("blank flag", func(t *testing.T) {
		t.Parallel()

		cmd := newRootCommand()
		if err := cmd.PersistentFlags().Set("db-conn-max-lifetime", ""); err != nil {
			t.Fatalf("set db-conn-max-lifetime flag: %v", err)
		}

		_, err := loadAppConfig(cmd, "", noEnv)
		if err == nil || !strings.Contains(err.Error(), `db-conn-max-lifetime: invalid duration ""`) {
			t.Fatalf("loadAppConfig() error = %v, want an invalid duration error", err)
		}
	})

	t.Run("blank in the config file", func(t *testing.T) {
		t.Parallel()

		cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
		if err := os.WriteFile(cfgPath, []byte("db-conn-max-idle-time: ''\n"), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		_, err := loadAppConfig(newRootCommand(), cfgPath, noEnv)
		if err == nil || !strings.Contains(err.Error(), `db-conn-max-idle-time: invalid duration ""`) {
			t.Fatalf("loadAppConfig() error = %v, want an invalid duration error", err)
		}
	})

	t.Run("null in the config file", func(t *testing.T) {
		t.Parallel()

		cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
		if err := os.WriteFile(cfgPath, []byte("db-conn-max-idle-time:\n"), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		cfg, err := loadAppConfig(newRootCommand(), cfgPath, noEnv)
		if err != nil {
			t.Fatalf("loadAppConfig() error = %v", err)
		}

		if want := 10 * time.Minute; cfg.DBConnMaxIdleTime != want {
			t.Fatalf("DBConnMaxIdleTime = %v, want the default %v", cfg.DBConnMaxIdleTime, want)
		}
	})
}

func TestLoadAppConfigReadsEverySourceAlike(t *testing.T) {
	t.Parallel()

	// The same text means the same thing as a flag, an env var or in the config file.
	for _, tt := range []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{"010", 8, false},
		{"0x19", 25, false},
		{"1_000", 1000, false},
		{"08", 0, true},
		{"25.0", 0, true},
		{"1e3", 0, true},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()

			cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
			if err := os.WriteFile(cfgPath, []byte("db-max-open-conns: "+tt.raw+"\n"), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			flagCmd := newRootCommand()
			flagErr := flagCmd.PersistentFlags().Set("db-max-open-conns", tt.raw)

			sources := map[string]func() (appConfig, error){
				"flag": func() (appConfig, error) {
					if flagErr != nil {
						return appConfig{}, flagErr
					}

					return loadAppConfig(flagCmd, "", noEnv)
				},
				"env": func() (appConfig, error) {
					env := envOf(map[string]string{"MINURL_DB_MAX_OPEN_CONNS": tt.raw})

					return loadAppConfig(newRootCommand(), "", env)
				},
				"file": func() (appConfig, error) { return loadAppConfig(newRootCommand(), cfgPath, noEnv) },
			}

			for name, load := range sources {
				cfg, err := load()
				if (err != nil) != tt.wantErr || cfg.DBMaxOpenConns != tt.want {
					t.Errorf("%s: DBMaxOpenConns = %d, error = %v, want %d, error %v",
						name, cfg.DBMaxOpenConns, err, tt.want, tt.wantErr)
				}
			}
		})
	}
}

func TestLoadAppConfigReadsConfigFileValuesAsWritten(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
	content := "otel-service-name: 010\ndb-max-open-conns: &n 7\ndb-max-idle-conns: *n\n" +
		"otel-exporter: &k otel-endpoint\n*k : collector:4317\n" +
		"http-addr: 2001-12-14\nid-seed: !!str 7\n"

	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := loadAppConfig(newRootCommand(), cfgPath, noEnv)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	// A string setting keeps its text; YAML would read 010 as the integer 8.
	if cfg.OTELServiceName != "010" {
		t.Errorf("OTELServiceName = %q, want %q", cfg.OTELServiceName, "010")
	}

	// An alias reads the node its anchor points at, as a value or as a key.
	if cfg.DBMaxOpenConns != 7 || cfg.DBMaxIdleConns != 7 {
		t.Errorf("DBMaxOpenConns, DBMaxIdleConns = %d, %d, want 7, 7", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)
	}

	if cfg.OTELEndpoint != "collector:4317" {
		t.Errorf("OTELEndpoint = %q, want %q", cfg.OTELEndpoint, "collector:4317")
	}

	// YAML's own reading of a value, here a timestamp, is not used; !!str asks for the text.
	if cfg.HTTPAddr != "2001-12-14" || cfg.IDSeed != "7" {
		t.Errorf("HTTPAddr, IDSeed = %q, %q, want %q, %q", cfg.HTTPAddr, cfg.IDSeed, "2001-12-14", "7")
	}
}

func TestLoadAppConfigTreatsEmptyEnvAsUnset(t *testing.T) {
	t.Parallel()

	// An empty env var keeps the default and does not hide the config file.
	cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
	if err := os.WriteFile(cfgPath, []byte("db-max-idle-conns: 3\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	env := envOf(map[string]string{
		"MINURL_DB_MAX_OPEN_CONNS": "",
		"MINURL_DB_MAX_IDLE_CONNS": "",
		"MINURL_OTEL_INSECURE":     "",
	})

	cfg, err := loadAppConfig(newRootCommand(), cfgPath, env)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if cfg.DBMaxOpenConns != 25 || cfg.DBMaxIdleConns != 3 || !cfg.OTELInsecure {
		t.Fatalf("DBMaxOpenConns, DBMaxIdleConns, OTELInsecure = %d, %d, %v, want 25, 3, true",
			cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, cfg.OTELInsecure)
	}
}

func TestLoadAppConfigBlankStringSettingsUseTheDefault(t *testing.T) {
	t.Parallel()

	// Unlike a pool size or a duration, a blank otel-service-name or log-format means its default.
	cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
	if err := os.WriteFile(cfgPath, []byte("otel-service-name: ''\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := loadAppConfig(newRootCommand(), cfgPath, envOf(map[string]string{"MINURL_LOG_FORMAT": " "}))
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if cfg.OTELServiceName != defaultConfig.OTELServiceName || cfg.LogFormat != defaultConfig.LogFormat {
		t.Fatalf("OTELServiceName, LogFormat = %q, %q, want the defaults %q, %q",
			cfg.OTELServiceName, cfg.LogFormat, defaultConfig.OTELServiceName, defaultConfig.LogFormat)
	}
}

func TestConfigFileFieldsAreStrings(t *testing.T) {
	t.Parallel()

	// configFileValues reads each field with Elem().String(), which does not fail on another type.
	for field := range reflect.TypeFor[configFile]().Fields() {
		if field.Type != reflect.TypeFor[*string]() {
			t.Errorf("configFile.%s is %s, want *string", field.Name, field.Type)
		}
	}
}

func TestLoadAppConfigReadsMergeKeysAndTags(t *testing.T) {
	t.Parallel()

	// yaml.v3 expands a merge key, a key written out winning, and applies a standard tag, as
	// v0.0.2 did.
	cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
	content := "log-format: text\n<<: {log-format: json, otel-exporter: otlp}\n" +
		"otel-service-name: !!binary c3Zj\ndb-max-open-conns: !!int 7\notel-endpoint: !!str collector\n"

	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := loadAppConfig(newRootCommand(), cfgPath, noEnv)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if cfg.LogFormat != "text" || cfg.OTELExporter != "otlp" {
		t.Errorf("LogFormat, OTELExporter = %q, %q, want text, otlp", cfg.LogFormat, cfg.OTELExporter)
	}

	if cfg.OTELServiceName != "svc" || cfg.DBMaxOpenConns != 7 || cfg.OTELEndpoint != "collector" {
		t.Errorf("OTELServiceName, DBMaxOpenConns, OTELEndpoint = %q, %d, %q, want svc, 7, collector",
			cfg.OTELServiceName, cfg.DBMaxOpenConns, cfg.OTELEndpoint)
	}
}
