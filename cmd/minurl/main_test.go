package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunOpenAPICommandAllFormats(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	msg, err := runOpenAPICommand(outDir)
	if err != nil {
		t.Fatalf("runOpenAPICommand returned error: %v", err)
	}

	if !strings.Contains(msg, "OpenAPI files generated in") {
		t.Fatalf("unexpected openapi message: %q", msg)
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.json")); err != nil {
		t.Fatalf("openapi.json should be generated: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.yaml")); err != nil {
		t.Fatalf("openapi.yaml should be generated: %v", err)
	}
}

func TestExecuteOpenAPICommand(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	cmd := newRootCommand()

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetArgs([]string{"openapi", "--out", outDir})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if !strings.Contains(out.String(), "OpenAPI files generated in") {
		t.Fatalf("expected openapi command output, got: %q", out.String())
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.yaml")); err != nil {
		t.Fatalf("openapi.yaml should be generated: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.json")); err != nil {
		t.Fatalf("openapi.json should be generated: %v", err)
	}
}

func TestExecuteVersionCommand(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "minurl version ") {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestExecuteWithConfigDirectoryReturnsFriendlyError(t *testing.T) {
	t.Parallel()

	err := execute([]string{"--config", t.TempDir()})
	if err == nil {
		t.Fatal("expected error for directory config path")
	}

	if !strings.Contains(err.Error(), "expected a file, got directory") {
		t.Fatalf("unexpected config error: %v", err)
	}
}

func TestExecuteVersionCommandSkipsConfigLoading(t *testing.T) {
	t.Parallel()

	cmd := newRootCommand()

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--config", t.TempDir(), "version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command should skip config loading: %v", err)
	}

	if !strings.Contains(out.String(), "minurl version ") {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}

func TestExecuteOpenAPICommandSkipsConfigLoading(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	cmd := newRootCommand()

	cmd.SetArgs([]string{"--config", t.TempDir(), "openapi", "--out", outDir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("openapi command should skip config loading: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.yaml")); err != nil {
		t.Fatalf("openapi.yaml should be generated: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "openapi.json")); err != nil {
		t.Fatalf("openapi.json should be generated: %v", err)
	}
}

func TestBuildVersion(t *testing.T) {
	origVersion := version
	origCommit := commit

	t.Cleanup(func() {
		version = origVersion
		commit = origCommit
	})

	version = "v1.2.3"
	commit = "abc1234"

	got := buildVersion()
	if got != "v1.2.3 (abc1234)" {
		t.Fatalf("unexpected buildVersion with commit: %q", got)
	}

	commit = ""

	got = buildVersion()
	if got != "v1.2.3" {
		t.Fatalf("unexpected buildVersion without commit: %q", got)
	}
}
