package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunOpenAPICommandAllFormats(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	if err := runOpenAPICommand([]string{"--out", outDir}); err != nil {
		t.Fatalf("runOpenAPICommand returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.json")); err != nil {
		t.Fatalf("openapi.json should be generated: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.yaml")); err != nil {
		t.Fatalf("openapi.yaml should be generated: %v", err)
	}
}

func TestRunOpenAPICommandJSONOnly(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	if err := runOpenAPICommand([]string{"--out", outDir, "--format", "json"}); err != nil {
		t.Fatalf("runOpenAPICommand returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.json")); err != nil {
		t.Fatalf("openapi.json should be generated: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.yaml")); !os.IsNotExist(err) {
		t.Fatalf("openapi.yaml should not be generated for json format")
	}
}

func TestRunOpenAPICommandInvalidFormat(t *testing.T) {
	t.Parallel()

	err := runOpenAPICommand([]string{"--format", "xml"})
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}
