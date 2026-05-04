// Copyright 2024 The MinURL Authors

package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	logFormatText      = "text"
	logFormatJSON      = "json"
	otelExporterStdout = "stdout"
	otelExporterOTLP   = "otlp"
)

// configKeys lists all configuration keys that should be bound from flags and environment variables.
var configKeys = []string{
	"http-addr",
	"id-seed",
	"storage-dsn",
	"log-format",
	"otel.enabled",
	"otel.service-name",
	"otel.exporter",
	"otel.endpoint",
	"otel.insecure",
}

type appConfig struct {
	HTTPAddr        string
	IDSeed          string
	StorageDSN      string
	LogFormat       string
	OTELEnabled     bool
	OTELServiceName string
	OTELExporter    string
	OTELEndpoint    string
	OTELInsecure    bool
}

func defaultAppConfig() appConfig {
	return appConfig{
		HTTPAddr:        ":8888",
		StorageDSN:      "sqlite3://minurl.sqlite3",
		LogFormat:       logFormatText,
		OTELEnabled:     false,
		OTELServiceName: "minurl",
		OTELExporter:    otelExporterStdout,
		OTELEndpoint:    "",
		OTELInsecure:    true,
	}
}

func loadAppConfig(cmd *cobra.Command, configPath string) (appConfig, error) {
	cfg := defaultAppConfig()

	v := viper.New()
	v.SetEnvPrefix("MINURL")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	v.SetDefault("http-addr", cfg.HTTPAddr)
	v.SetDefault("storage-dsn", cfg.StorageDSN)
	v.SetDefault("log-format", cfg.LogFormat)
	v.SetDefault("otel.enabled", cfg.OTELEnabled)
	v.SetDefault("otel.service-name", cfg.OTELServiceName)
	v.SetDefault("otel.exporter", cfg.OTELExporter)
	v.SetDefault("otel.endpoint", cfg.OTELEndpoint)
	v.SetDefault("otel.insecure", cfg.OTELInsecure)

	if err := bindConfigFlags(v, cmd); err != nil {
		return appConfig{}, err
	}

	if configPath != "" {
		v.SetConfigFile(configPath)

		if err := v.ReadInConfig(); err != nil {
			return appConfig{}, fmt.Errorf("read config file %q: %w", configPath, err)
		}

		applyHyphenatedOTelConfigKeys(v, cmd)
	}

	cfg.HTTPAddr = v.GetString("http-addr")
	cfg.IDSeed = strings.TrimSpace(v.GetString("id-seed"))
	cfg.StorageDSN = strings.TrimSpace(v.GetString("storage-dsn"))
	cfg.LogFormat = strings.ToLower(strings.TrimSpace(v.GetString("log-format")))
	cfg.OTELEnabled = v.GetBool("otel.enabled")
	cfg.OTELServiceName = strings.TrimSpace(v.GetString("otel.service-name"))
	cfg.OTELExporter = strings.ToLower(strings.TrimSpace(v.GetString("otel.exporter")))
	cfg.OTELEndpoint = strings.TrimSpace(v.GetString("otel.endpoint"))
	cfg.OTELInsecure = v.GetBool("otel.insecure")

	if cfg.HTTPAddr == "" {
		return appConfig{}, fmt.Errorf("http-addr must not be empty")
	}

	if cfg.IDSeed != "" {
		if _, err := parseUint32(cfg.IDSeed); err != nil {
			return appConfig{}, fmt.Errorf("parse id-seed: %w", err)
		}
	}

	if cfg.StorageDSN == "" {
		return appConfig{}, fmt.Errorf("storage-dsn must not be empty")
	}

	if _, err := detectStorageBackend(cfg.StorageDSN); err != nil {
		return appConfig{}, fmt.Errorf("storage-dsn: %w", err)
	}

	switch cfg.LogFormat {
	case "", logFormatText, logFormatJSON:
		if cfg.LogFormat == "" {
			cfg.LogFormat = logFormatText
		}
	default:
		return appConfig{}, fmt.Errorf(
			"invalid log-format %q: expected %s or %s",
			cfg.LogFormat,
			logFormatText,
			logFormatJSON,
		)
	}

	if cfg.OTELEnabled {
		switch cfg.OTELExporter {
		case otelExporterStdout, otelExporterOTLP:
			if cfg.OTELExporter == otelExporterOTLP && cfg.OTELEndpoint == "" {
				return appConfig{}, fmt.Errorf("otel.endpoint must be set when otel.exporter=otlp")
			}
		default:
			return appConfig{}, fmt.Errorf(
				"invalid otel.exporter %q: expected %s or %s",
				cfg.OTELExporter,
				otelExporterStdout,
				otelExporterOTLP,
			)
		}
	}

	if cfg.OTELServiceName == "" {
		cfg.OTELServiceName = "minurl"
	}

	return cfg, nil
}

func parseUint32(raw string) (uint32, error) {
	if raw == "" {
		return 0, fmt.Errorf("empty value")
	}

	v, err := strconv.ParseUint(raw, 0, 32)
	if err != nil {
		return 0, err
	}

	return uint32(v), nil
}
