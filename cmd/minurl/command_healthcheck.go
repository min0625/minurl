// Copyright 2024 The MinURL Authors
package main

import (
	"fmt"
	"net/http"
	"net/url"
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
			targetURL, err := url.JoinPath(addr, "livez")
			if err != nil {
				return fmt.Errorf("invalid addr: %w", err)
			}

			client := &http.Client{Timeout: 5 * time.Second}

			resp, err := client.Get(targetURL) //nolint:noctx // healthcheck is intentionally simple
			if err != nil {
				return fmt.Errorf("healthcheck failed: %w", err)
			}

			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("healthcheck failed: HTTP %d", resp.StatusCode)
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
