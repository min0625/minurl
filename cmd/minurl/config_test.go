// Copyright 2026 The MinURL Authors

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/min0625/minurl/internal/service"
)

func TestLoadAppConfigPrecedenceFlagOverEnvOverFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
	content := []byte(
		"http-addr: ':7000'\nid-seed: '11'\nstorage-dsn: 'sqlite3://from-file.sqlite3'\n",
	)

	if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Setenv("MINURL_HTTP_ADDR", ":8000")
	t.Setenv("MINURL_ID_SEED", "22")
	t.Setenv("MINURL_STORAGE_DSN", "sqlite3://from-env.sqlite3")

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

	cfg, err := loadAppConfig(cmd, cfgPath)
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
}

func TestLoadAppConfigRejectsInvalidLogFormat(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("log-format", "xml"); err != nil {
		t.Fatalf("set log-format flag: %v", err)
	}

	if _, err := loadAppConfig(cmd, ""); err == nil {
		t.Fatal("loadAppConfig() error = nil, want non-nil")
	}
}

func TestLoadAppConfigOTelEnvOverridesFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "otel-env.yaml")
	content := []byte(
		"http-addr: ':7000'\notel-enabled: false\notel-exporter: stdout\notel-endpoint: http://file:4318\n",
	)

	if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Setenv("MINURL_OTEL_ENABLED", "true")
	t.Setenv("MINURL_OTEL_EXPORTER", "otlp")
	t.Setenv("MINURL_OTEL_ENDPOINT", "http://env:4318")

	cmd := newRootCommand()

	cfg, err := loadAppConfig(cmd, cfgPath)
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

	if _, err := loadAppConfig(cmd, ""); err != nil {
		t.Fatalf("loadAppConfig() error = %v, want nil", err)
	}
}

func TestLoadAppConfigRejectsInvalidSeed(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("id-seed", "not-a-number"); err != nil {
		t.Fatalf("set id-seed flag: %v", err)
	}

	if _, err := loadAppConfig(cmd, ""); err == nil {
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

	if _, err := loadAppConfig(cmd, ""); err == nil {
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

	if _, err := loadAppConfig(cmd, ""); err == nil {
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

	if _, err := loadAppConfig(cmd, ""); err == nil {
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

	cfg, err := loadAppConfig(cmd, "")
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

	cfg, err := loadAppConfig(cmd, "")
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

	cfg, err := loadAppConfig(cmd, "")
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
	}

	for _, tt := range tests {
		t.Run(tt.env+"="+tt.value, func(t *testing.T) {
			t.Setenv(tt.env, tt.value)

			if _, err := loadAppConfig(newRootCommand(), ""); err == nil {
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
	} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()

			cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
			if err := os.WriteFile(cfgPath, []byte(line+"\n"), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			if _, err := loadAppConfig(newRootCommand(), cfgPath); err == nil {
				t.Fatal("loadAppConfig() error = nil, want non-nil")
			}
		})
	}
}

func TestLoadAppConfigParsesIntAndBoolStrictly(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
	if err := os.WriteFile(cfgPath, []byte("db-max-idle-conns: 010\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	// A leading 0 is octal in an env var, as the YAML decoder reads it in the config file,
	// and surrounding space is trimmed.
	t.Setenv("MINURL_DB_MAX_OPEN_CONNS", " 010 ")
	t.Setenv("MINURL_OTEL_INSECURE", "TRUE")

	cfg, err := loadAppConfig(newRootCommand(), cfgPath)
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

	for _, tc := range []struct{ name, file, content, want string }{
		{"unknown key", "minurl.yaml", "idseed: 5\n", `line 1: unknown key "idseed"`},
		{"unknown null key", "minurl.yaml", "idseed:\n", `line 1: unknown key "idseed"`},
		{"unknown flat key", "minurl.yaml", "db-max-open-con: 5\n", `line 1: unknown key "db-max-open-con"`},
		{
			"nested key", "minurl.yaml", "http-addr: ':80'\ndb:\n  max-open-conns: 5\n",
			`line 2: unknown key "db" (settings are flat keys, such as db-max-open-conns)`,
		},
		{"null section", "minurl.yaml", "otel:\n", `line 1: unknown key "otel" (settings are flat keys, such as otel-enabled)`},
		{"section as a value", "minurl.yaml", "otel: true\n", `line 1: unknown key "otel" (settings are flat keys, such as otel-enabled)`},
		{
			"dotted key", "minurl.yaml", "db.max-open-conns: 5\n",
			`unknown key "db.max-open-conns" (settings are flat keys, such as db-max-open-conns)`,
		},
		{"flag that is not a setting", "minurl.yaml", "config: other.yaml\n", `line 1: unknown key "config"`},
		{"every problem", "minurl.yaml", "idseed: 5\nfoo: 1\n", `line 2: unknown key "foo"`},
		{"merge key", "minurl.yaml", "<<: {log-format: json}\n", "line 1: merge keys (<<) are not supported"},
		{"quoted <<", "minurl.yaml", "\"<<\": 1\n", `line 1: unknown key "<<"`},
		{"mapping value", "minurl.yaml", "db-max-open-conns:\n  foo: 1\n", "line 1: db-max-open-conns takes a single value"},
		{"list value", "minurl.yaml", "http-addr: [':80']\n", "line 1: http-addr takes a single value"},
		{"given twice", "minurl.yaml", "id-seed: 1\nid-seed: 2\n", "line 2: id-seed is already set on line 1"},
		{"uppercase", "minurl.yaml", "HTTP-Addr: ':80'\n", `line 1: unknown key "HTTP-Addr" (keys are lowercase: http-addr)`},
		// viper folds casings into one key, so a null ID-SEED: would replace the value.
		{
			"uppercase null after the key", "minurl.yaml", "id-seed: 1\nID-SEED:\n",
			`line 2: unknown key "ID-SEED" (keys are lowercase: id-seed)`,
		},
		{"uppercase section", "minurl.yaml", "OTEL:\n", `line 1: unknown key "OTEL" (settings are flat keys, such as otel-enabled)`},
		{"JSON", "minurl.json", `{"id-seed": 5}`, "must be YAML (.yaml or .yml)"},
		{"TOML", "minurl.toml", "id-seed = 5\n", "must be YAML (.yaml or .yml)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfgPath := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(cfgPath, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			_, err := loadAppConfig(newRootCommand(), cfgPath)
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

			cfg, err := loadAppConfig(newRootCommand(), cfgPath)
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
	} {
		t.Run(content, func(t *testing.T) {
			t.Parallel()

			cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
			if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			cfg, err := loadAppConfig(newRootCommand(), cfgPath)
			if err != nil {
				t.Fatalf("loadAppConfig() error = %v", err)
			}

			if want := defaultAppConfig(); cfg != want {
				t.Fatalf("loadAppConfig() = %+v, want the defaults %+v", cfg, want)
			}
		})
	}
}

func TestLoadAppConfigReadsConfigExample(t *testing.T) {
	t.Parallel()

	if _, err := loadAppConfig(newRootCommand(), "../../config.example.yaml"); err != nil {
		t.Fatalf("loadAppConfig(config.example.yaml) error = %v", err)
	}
}
