// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/danielgtaylor/huma/v2"
	"github.com/min0625/minurl/internal/httpserver"
	"github.com/spf13/cobra"
)

const (
	openAPIDirPerm  os.FileMode = 0o750
	openAPIFilePerm os.FileMode = 0o600
)

func newOpenAPICommand() *cobra.Command {
	var outDir string

	cmd := &cobra.Command{
		Use:   cmdOpenAPI,
		Short: "Generate OpenAPI specification files",
		RunE: func(cmd *cobra.Command, _ []string) error {
			msg, err := runOpenAPICommand(outDir)
			if err != nil {
				return fmt.Errorf("openapi command failed: %w", err)
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
			if err != nil {
				return fmt.Errorf("write openapi command output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&outDir, "out", "docs/openapi", "output directory for OpenAPI files")

	return cmd
}

func runOpenAPICommand(outDir string) (string, error) {
	_, api := httpserver.BuildOpenAPIRouter(version)

	spec := api.OpenAPI()

	if err := os.MkdirAll(outDir, openAPIDirPerm); err != nil {
		return "", fmt.Errorf("create output directory %q: %w", outDir, err)
	}

	if err := writeOpenAPIJSON(spec, filepath.Join(outDir, "openapi.json")); err != nil {
		return "", err
	}

	if err := writeOpenAPIYAML(spec, filepath.Join(outDir, "openapi.yaml")); err != nil {
		return "", err
	}

	return "OpenAPI files generated in " + outDir, nil
}

func writeOpenAPIJSON(spec *huma.OpenAPI, path string) error {
	b, err := spec.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal OpenAPI JSON: %w", err)
	}

	if err := os.WriteFile(path, b, openAPIFilePerm); err != nil {
		return fmt.Errorf("write OpenAPI JSON to %q: %w", path, err)
	}

	return nil
}

func writeOpenAPIYAML(spec *huma.OpenAPI, path string) error {
	b, err := spec.YAML()
	if err != nil {
		return fmt.Errorf("marshal OpenAPI YAML: %w", err)
	}

	if err := os.WriteFile(path, b, openAPIFilePerm); err != nil {
		return fmt.Errorf("write OpenAPI YAML to %q: %w", path, err)
	}

	return nil
}
