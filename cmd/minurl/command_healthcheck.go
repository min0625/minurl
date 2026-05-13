// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

func newHealthCheckCommand() *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:   "healthcheck",
		Short: "Check if the MinURL server is healthy",
		Long: "Makes an HTTP GET request to /livez on the running server. " +
			"Exits 0 if the server is healthy, 1 otherwise.",
		RunE: func(_ *cobra.Command, _ []string) error {
			client := &http.Client{Timeout: 5 * time.Second}
			url := addr + "/livez"

			resp, err := client.Get(url) //nolint:noctx // healthcheck is intentionally simple
			if err != nil {
				fmt.Fprintf(os.Stderr, "healthcheck failed: %v\n", err)
				os.Exit(1)
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				fmt.Fprintf(os.Stderr, "healthcheck failed: HTTP %d\n", resp.StatusCode)
				os.Exit(1)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(
		&addr,
		"addr",
		"http://localhost:8888",
		"Base URL of the MinURL server to check",
	)

	return cmd
}
