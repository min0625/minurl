// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

type rootOptions struct {
	configPath string
	appConfig  appConfig
}

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})))
}

func main() {
	if err := execute(os.Args[1:]); err != nil {
		slog.Error("execution failed", "error", err)
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
	cmd.PersistentFlags().String(
		"storage-dsn",
		"sqlite3://minurl.sqlite3",
		"storage DSN: sqlite3://path for SQLite (default) or postgres://... for PostgreSQL",
	)
	cmd.PersistentFlags().String("log-format", "text", "log output format: text or json")
	cmd.PersistentFlags().Bool("otel-enabled", false, "enable OpenTelemetry tracing")
	cmd.PersistentFlags().String("otel-service-name", "minurl", "OpenTelemetry service name")
	cmd.PersistentFlags().
		String("otel-exporter", "stdout", "OpenTelemetry exporter: stdout or otlp")
	cmd.PersistentFlags().String("otel-endpoint", "", "OTLP collector endpoint")
	cmd.PersistentFlags().Bool("otel-insecure", true, "allow insecure OTLP connection")
	cmd.PersistentFlags().Int(
		"db-max-open-conns",
		25,
		"max open DB connections, PostgreSQL only (0 = unlimited, not recommended)",
	)
	cmd.PersistentFlags().Int(
		"db-max-idle-conns",
		5,
		"max idle DB connections retained in pool, PostgreSQL only (0 = none retained)",
	)
	cmd.PersistentFlags().String(
		"db-conn-max-lifetime",
		"30m",
		"max connection lifetime, PostgreSQL only (0 = no limit, e.g. 30m, 1h)",
	)
	cmd.PersistentFlags().String(
		"db-conn-max-idle-time",
		"10m",
		"max connection idle time, PostgreSQL only (0 = no limit, e.g. 10m, 30m)",
	)
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
