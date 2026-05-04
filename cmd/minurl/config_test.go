package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/min0625/minurl/internal/model"
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

func TestLoadAppConfigReadsHyphenatedOTelConfigKeys(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "otel.yaml")
	content := []byte("http-addr: ':7000'\notel-enabled: true\notel-exporter: stdout\n")

	if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cmd := newRootCommand()

	cfg, err := loadAppConfig(cmd, cfgPath)
	if err != nil {
		t.Fatalf("loadAppConfig() error = %v", err)
	}

	if !cfg.OTELEnabled {
		t.Fatalf("OTELEnabled = %v, want true", cfg.OTELEnabled)
	}

	if cfg.OTELExporter != "stdout" {
		t.Fatalf("OTELExporter = %q, want %q", cfg.OTELExporter, "stdout")
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
		model.ShortURL{OriginalURL: "https://example.org/a"},
	)
	if err != nil {
		t.Fatalf("svcA.Create() error = %v", err)
	}

	b, err := svcB.Create(
		context.Background(),
		model.ShortURL{OriginalURL: "https://example.org/b"},
	)
	if err != nil {
		t.Fatalf("svcB.Create() error = %v", err)
	}

	c, err := svcC.Create(
		context.Background(),
		model.ShortURL{OriginalURL: "https://example.org/c"},
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
		model.ShortURL{OriginalURL: "https://example.com"},
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

	got, found, err := svc2.Get(t.Context(), entry.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if !found {
		t.Fatalf("Get() found = false, want true")
	}

	if got.OriginalURL != "https://example.com" {
		t.Fatalf("OriginalURL = %q, want %q", got.OriginalURL, "https://example.com")
	}

	entry2, err := svc2.Create(
		t.Context(),
		model.ShortURL{OriginalURL: "https://example.org/another"},
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
	if err := cmd.PersistentFlags().Set("storage-dsn", "mysql://localhost/db"); err != nil {
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
