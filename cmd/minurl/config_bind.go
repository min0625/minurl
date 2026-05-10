// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func bindConfigFlags(v *viper.Viper, cmd *cobra.Command) error {
	for _, key := range configKeys {
		flagName := strings.ReplaceAll(key, ".", "-")

		f := lookupFlag(cmd, flagName)
		if f == nil {
			return fmt.Errorf("lookup flag %q: not found", flagName)
		}

		if err := v.BindPFlag(key, f); err != nil {
			return fmt.Errorf("bind flag %q: %w", key, err)
		}
	}

	for _, key := range configKeys {
		if err := v.BindEnv(key); err != nil {
			return fmt.Errorf("bind env %q: %w", key, err)
		}
	}

	return nil
}

func applyHyphenatedOTelConfigKeys(v *viper.Viper, cmd *cobra.Command) {
	if flag := lookupFlag(cmd, "otel-enabled"); flag != nil &&
		!flag.Changed && v.IsSet("otel-enabled") {
		v.Set("otel.enabled", v.Get("otel-enabled"))
	}

	if flag := lookupFlag(cmd, "otel-service-name"); flag != nil &&
		!flag.Changed && v.IsSet("otel-service-name") {
		v.Set("otel.service-name", v.Get("otel-service-name"))
	}

	if flag := lookupFlag(cmd, "otel-exporter"); flag != nil &&
		!flag.Changed && v.IsSet("otel-exporter") {
		v.Set("otel.exporter", v.Get("otel-exporter"))
	}

	if flag := lookupFlag(cmd, "otel-endpoint"); flag != nil &&
		!flag.Changed && v.IsSet("otel-endpoint") {
		v.Set("otel.endpoint", v.Get("otel-endpoint"))
	}

	if flag := lookupFlag(cmd, "otel-insecure"); flag != nil &&
		!flag.Changed && v.IsSet("otel-insecure") {
		v.Set("otel.insecure", v.Get("otel-insecure"))
	}
}

func applyHyphenatedDBConfigKeys(v *viper.Viper, cmd *cobra.Command) {
	if flag := lookupFlag(cmd, "db-max-open-conns"); flag != nil &&
		!flag.Changed && v.IsSet("db-max-open-conns") {
		v.Set("db.max-open-conns", v.Get("db-max-open-conns"))
	}

	if flag := lookupFlag(cmd, "db-max-idle-conns"); flag != nil &&
		!flag.Changed && v.IsSet("db-max-idle-conns") {
		v.Set("db.max-idle-conns", v.Get("db-max-idle-conns"))
	}

	if flag := lookupFlag(cmd, "db-conn-max-lifetime"); flag != nil &&
		!flag.Changed && v.IsSet("db-conn-max-lifetime") {
		v.Set("db.conn-max-lifetime", v.Get("db-conn-max-lifetime"))
	}

	if flag := lookupFlag(cmd, "db-conn-max-idle-time"); flag != nil &&
		!flag.Changed && v.IsSet("db-conn-max-idle-time") {
		v.Set("db.conn-max-idle-time", v.Get("db-conn-max-idle-time"))
	}
}

func lookupFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if f := cmd.Flags().Lookup(name); f != nil {
		return f
	}

	if f := cmd.PersistentFlags().Lookup(name); f != nil {
		return f
	}

	if f := cmd.InheritedFlags().Lookup(name); f != nil {
		return f
	}

	return nil
}
