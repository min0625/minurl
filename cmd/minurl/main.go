// Copyright 2024 The MinURL Authors
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/min0625/minurl/internal/handler"
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

type rootOptions struct {
	configPath string
	appConfig  appConfig
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
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if !requiresRuntimeConfig(cmd) {
				return nil
			}

			if err := validateConfigPath(opts.configPath); err != nil {
				return err
			}

			cfg, err := loadAppConfig(cmd, opts.configPath)
			if err != nil {
				return err
			}

			opts.appConfig = cfg

			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := runServer(opts.appConfig); err != nil {
				return fmt.Errorf("server error: %w", err)
			}

			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "path to configuration file")
	cmd.PersistentFlags().String("http-addr", ":8888", "HTTP listen address")
	cmd.PersistentFlags().String(
		"id-seed",
		"",
		"seed for deterministic ID key derivation (uint32, decimal or 0x hex)",
	)
	cmd.PersistentFlags().
		String("storage-path", "minurl.sqlite3", "file path for the SQLite database")

	cmd.AddCommand(newOpenAPICommand())
	cmd.AddCommand(newVersionCommand())

	return cmd
}

func requiresRuntimeConfig(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}

	name := cmd.Name()

	return name != "openapi" && name != "version"
}

func newOpenAPICommand() *cobra.Command {
	var outDir string

	cmd := &cobra.Command{
		Use:   "openapi",
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

func buildAPI(svc handler.ShortURLService) (*chi.Mux, huma.API) {
	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", "0.1.0"))

	handler.Register(api, svc)

	return r, api
}

func runServer(cfg appConfig) error {
	svc, closer, err := newShortURLServiceFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("build short url service from config: %w", err)
	}

	defer func() {
		_ = closer.Close()
	}()

	r, _ := buildAPI(svc)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listenErrCh := make(chan error, 1)

	go func() {
		fmt.Printf("Server listening on %s\n", cfg.HTTPAddr)
		fmt.Printf("API docs: http://localhost%s/docs\n", cfg.HTTPAddr)

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

func runOpenAPICommand(outDir string) (string, error) {
	_, api := buildAPI(nil)

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
