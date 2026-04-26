package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppConfigPrecedenceFlagOverEnvOverFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "minurl.yaml")
	content := []byte("http-addr: ':7000'\nid-seed: '11'\n")

	if err := os.WriteFile(cfgPath, content, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	t.Setenv("MINURL_HTTP_ADDR", ":8000")
	t.Setenv("MINURL_ID_SEED", "22")

	cmd := newRootCommand()
	if err := cmd.PersistentFlags().Set("http-addr", ":9000"); err != nil {
		t.Fatalf("set http-addr flag: %v", err)
	}

	if err := cmd.PersistentFlags().Set("id-seed", "33"); err != nil {
		t.Fatalf("set id-seed flag: %v", err)
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

	svcA, err := newShortURLServiceFromConfig(appConfig{IDSeed: "1234"})
	if err != nil {
		t.Fatalf("newShortURLServiceFromConfig(seed 1234) error = %v", err)
	}

	svcB, err := newShortURLServiceFromConfig(appConfig{IDSeed: "1234"})
	if err != nil {
		t.Fatalf("newShortURLServiceFromConfig(seed 1234 second) error = %v", err)
	}

	svcC, err := newShortURLServiceFromConfig(appConfig{IDSeed: "9999"})
	if err != nil {
		t.Fatalf("newShortURLServiceFromConfig(seed 9999) error = %v", err)
	}

	a, err := svcA.Create(context.Background(), "https://example.org/a")
	if err != nil {
		t.Fatalf("svcA.Create() error = %v", err)
	}

	b, err := svcB.Create(context.Background(), "https://example.org/b")
	if err != nil {
		t.Fatalf("svcB.Create() error = %v", err)
	}

	c, err := svcC.Create(context.Background(), "https://example.org/c")
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
