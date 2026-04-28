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

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/min0625/minurl/internal/handler"
)

// buildBaseRouter creates the chi router with standard middleware applied.
func buildBaseRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(panicRecoveryMiddleware)
	r.Use(requestLoggerMiddleware)
	r.Use(accessLogMiddleware)

	return r
}

// buildAPI creates a chi router with handlers and returns it along with the Huma API for server runtime.
func buildAPI(svc handler.ShortURLService) (*chi.Mux, huma.API) {
	r := buildBaseRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", version))

	handler.Register(api, svc)

	return r, api
}

// buildOpenAPIRouter creates a chi router suitable for OpenAPI schema generation without runtime services.
func buildOpenAPIRouter() (*chi.Mux, huma.API) {
	r := buildBaseRouter()
	api := humachi.New(r, huma.DefaultConfig("MinURL API", version))
	handler.RegisterOpenAPI(api)

	return r, api
}

func runServer(cfg appConfig) error {
	configureDefaultLogger(cfg.LogFormat)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdown, err := initOpenTelemetry(ctx, cfg)
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

	r, _ := buildAPI(svc)

	h := wrapHTTPHandlerWithTelemetry(r, cfg)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	server.RegisterOnShutdown(func() {
		slog.Info("server shutdown hook triggered")
	})

	listener, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", cfg.HTTPAddr, err)
	}

	boundAddr, docsURL := serverListenLogValues(listener.Addr())

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

func serverListenLogValues(addr net.Addr) (string, string) {
	boundAddr := addr.String()

	_, port, err := net.SplitHostPort(boundAddr)
	if err != nil || port == "" {
		return boundAddr, ""
	}

	return boundAddr, "http://" + net.JoinHostPort("localhost", port) + "/docs"
}
