// Copyright 2024 The MinURL Authors
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/min0625/minurl/internal/handler"
	"github.com/min0625/minurl/internal/service"
	"github.com/spf13/cobra"
)

const (
	openAPIDirPerm  os.FileMode = 0o750
	openAPIFilePerm os.FileMode = 0o600
)

var (
	version = "dev"
	commit  = ""
)

type openAPIFormat string

const (
	openAPIFormatAll  openAPIFormat = "all"
	openAPIFormatJSON openAPIFormat = "json"
	openAPIFormatYAML openAPIFormat = "yaml"
)

func (f *openAPIFormat) String() string {
	if f == nil {
		return string(openAPIFormatAll)
	}

	return string(*f)
}

func (f *openAPIFormat) Set(value string) error {
	normalized := strings.ToLower(strings.TrimSpace(value))

	switch openAPIFormat(normalized) {
	case openAPIFormatAll, openAPIFormatJSON, openAPIFormatYAML:
		*f = openAPIFormat(normalized)

		return nil
	default:
		return fmt.Errorf("invalid value %q for --format: must be one of: all, json, yaml", value)
	}
}

func (*openAPIFormat) Type() string {
	return "openapi-format"
}

type rootOptions struct {
	configPath string
}

func main() {
	if err := execute(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func execute(args []string) error {
	cmd := newRootCommand()
	cmd.SetArgs(args)

	return cmd.Execute()
}

func newRootCommand() *cobra.Command {
	opts := &rootOptions{}

	cmd := &cobra.Command{
		Use:           "minurl",
		Short:         "MinURL service",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if err := validateConfigPath(opts.configPath); err != nil {
				return err
			}

			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := runServer(); err != nil {
				return fmt.Errorf("server error: %w", err)
			}

			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "path to configuration file")

	cmd.AddCommand(newOpenAPICommand())
	cmd.AddCommand(newVersionCommand())

	return cmd
}

func newOpenAPICommand() *cobra.Command {
	var (
		outDir string
		format = openAPIFormatAll
	)

	cmd := &cobra.Command{
		Use:   "openapi",
		Short: "Generate OpenAPI specification files",
		RunE: func(cmd *cobra.Command, _ []string) error {
			msg, err := runOpenAPICommand(outDir, format)
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
	cmd.Flags().Var(&format, "format", "output format: all|json|yaml")

	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the minurl CLI version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "minurl version %s\n", buildVersion())

			return err
		},
	}
}

func buildVersion() string {
	if commit == "" {
		return version
	}

	return fmt.Sprintf("%s (%s)", version, commit)
}

func validateConfigPath(path string) error {
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("invalid --config path %q: %w", path, err)
	}

	if info.IsDir() {
		return fmt.Errorf("invalid --config path %q: expected a file, got directory", path)
	}

	return nil
}

func buildAPI() huma.API {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("MinURL API", "0.1.0"))

	svc := service.NewShortURLService()
	handler.Register(api, svc)

	return api
}

func runServer() error {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("MinURL API", "0.1.0"))

	svc := service.NewShortURLService()
	handler.Register(api, svc)

	addr := ":8888"
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listenErrCh := make(chan error, 1)

	go func() {
		fmt.Printf("Server listening on %s\n", addr)
		fmt.Printf("API docs: http://localhost%s/docs\n", addr)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			listenErrCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-listenErrCh:
		return fmt.Errorf("listen and serve: %w", err)
	}

	fmt.Println("Shutting down...")

	if err := server.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	return nil
}

func runOpenAPICommand(outDir string, format openAPIFormat) (string, error) {
	api := buildAPI()

	spec := api.OpenAPI()

	if err := os.MkdirAll(outDir, openAPIDirPerm); err != nil {
		return "", fmt.Errorf("create output directory %q: %w", outDir, err)
	}

	switch format {
	case openAPIFormatAll:
		if err := writeOpenAPIJSON(spec, filepath.Join(outDir, "openapi.json")); err != nil {
			return "", err
		}

		if err := writeOpenAPIYAML(spec, filepath.Join(outDir, "openapi.yaml")); err != nil {
			return "", err
		}
	case openAPIFormatJSON:
		if err := writeOpenAPIJSON(spec, filepath.Join(outDir, "openapi.json")); err != nil {
			return "", err
		}
	case openAPIFormatYAML:
		if err := writeOpenAPIYAML(spec, filepath.Join(outDir, "openapi.yaml")); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported format %q (expected all|json|yaml)", format)
	}

	return fmt.Sprintf("OpenAPI files generated in %s", outDir), nil
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
