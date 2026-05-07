// Copyright 2024 The MinURL Authors

// Package telemetry provides OpenTelemetry initialization and HTTP handler wrapping.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	// ExporterStdout writes traces to stdout in a human-readable format.
	ExporterStdout = "stdout"
	// ExporterOTLP ships traces to an OTLP collector over gRPC.
	ExporterOTLP = "otlp"
)

// Config holds the OpenTelemetry configuration for the service.
type Config struct {
	Enabled     bool
	ServiceName string
	Exporter    string // ExporterStdout or ExporterOTLP
	Endpoint    string
	Insecure    bool
	Version     string
}

// Init sets up a global TracerProvider based on cfg.
// It returns a shutdown function that must be called on process exit.
// When cfg.Enabled is false a no-op shutdown function is returned immediately.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	var (
		exp sdktrace.SpanExporter
		err error
	)

	switch cfg.Exporter {
	case ExporterStdout:
		exp, err = stdouttrace.New(
			stdouttrace.WithWriter(os.Stdout),
			stdouttrace.WithPrettyPrint(),
		)
	case ExporterOTLP:
		clientOpts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			clientOpts = append(clientOpts, otlptracegrpc.WithInsecure())
		}

		exp, err = otlptracegrpc.New(ctx, clientOpts...)
	default:
		return nil, fmt.Errorf("unsupported otel exporter %q", cfg.Exporter)
	}

	if err != nil {
		return nil, fmt.Errorf("create exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.version", cfg.Version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tracerProviderOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}

	if cfg.Exporter == ExporterStdout {
		tracerProviderOpts = append(tracerProviderOpts, sdktrace.WithSyncer(exp))
	} else {
		tracerProviderOpts = append(tracerProviderOpts, sdktrace.WithBatcher(exp))
	}

	tp := sdktrace.NewTracerProvider(tracerProviderOpts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

// WrapHTTPHandler wraps h with OpenTelemetry instrumentation when cfg.Enabled is true.
// When disabled the original handler is returned unchanged.
func WrapHTTPHandler(h http.Handler, cfg Config) http.Handler {
	if cfg.Enabled {
		return otelhttp.NewHandler(
			h,
			"http.server",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				path := r.URL.Path
				if path == "" {
					path = "/"
				}

				if r.Method == "" {
					return path
				}

				return fmt.Sprintf("%s %s", r.Method, path)
			}),
		)
	}

	return h
}
