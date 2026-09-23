// Copyright 2026 The MinURL Authors

package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func bindConfigFlags(v *viper.Viper, cmd *cobra.Command) error {
	for _, key := range configKeys {
		f := lookupFlag(cmd, key)
		if f == nil {
			return fmt.Errorf("lookup flag %q: not found", key)
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
