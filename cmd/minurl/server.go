// Copyright 2024 The MinURL Authors
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/min0625/minurl/internal/handler"
	"github.com/min0625/minurl/internal/httpserver"
	"github.com/min0625/minurl/internal/middleware"
	"github.com/min0625/minurl/internal/telemetry"
)

func runServer(cfg appConfig) error {
	middleware.ConfigureDefaultLogger(cfg.LogFormat)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	otelCfg := telemetry.Config{
		Enabled:     cfg.OTELEnabled,
		ServiceName: cfg.OTELServiceName,
		Exporter:    cfg.OTELExporter,
		Endpoint:    cfg.OTELEndpoint,
		Insecure:    cfg.OTELInsecure,
		Version:     version,
	}

	shutdown, err := telemetry.Init(ctx, otelCfg)
	if err != nil {
		return fmt.Errorf("initialize opentelemetry: %w", err)
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if shutdownErr := shutdown(shutdownCtx); shutdownErr != nil {
			slog.ErrorContext(shutdownCtx, "shutdown opentelemetry", "error", shutdownErr)
		}
	}()

	svc, closer, err := newShortURLServiceFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("build short url service from config: %w", err)
	}

	defer func() {
		_ = closer.Close()
	}()

	r, _ := httpserver.BuildAPI(svc, version)
	handler.RegisterHealthHandlers(r, closer)

	h := telemetry.WrapHTTPHandler(r, otelCfg)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	server.RegisterOnShutdown(func() {
		slog.Info("server shutdown hook triggered")
	})

	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", cfg.HTTPAddr, err)
	}

	boundAddr, docsURL := httpserver.ListenLogValues(listener.Addr())

	listenErrCh := make(chan error, 1)

	go func() {
		logger := slog.With("addr", boundAddr)
		if docsURL != "" {
			logger = logger.With("docs_url", docsURL)
		}

		logger.InfoContext(ctx, "server listening")

		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			listenErrCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-listenErrCh:
		return fmt.Errorf("serve http: %w", err)
	}

	slog.InfoContext(ctx, "shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	return nil
}
