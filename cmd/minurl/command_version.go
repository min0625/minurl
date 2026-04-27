// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = ""
)

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
