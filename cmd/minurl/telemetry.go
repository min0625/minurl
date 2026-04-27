// Copyright 2024 The MinURL Authors

package main

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

func initOpenTelemetry(ctx context.Context, cfg appConfig) (func(context.Context) error, error) {
	if !cfg.OTELEnabled {
		noopShutdown := func(context.Context) error { return nil }
		return noopShutdown, nil
	}

	var (
		exp sdktrace.SpanExporter
		err error
	)

	switch cfg.OTELExporter {
	case otelExporterStdout:
		exp, err = stdouttrace.New(
			stdouttrace.WithWriter(os.Stdout),
			stdouttrace.WithPrettyPrint(),
		)
	case otelExporterOTLP:
		clientOpts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.OTELEndpoint),
		}
		if cfg.OTELInsecure {
			clientOpts = append(clientOpts, otlptracegrpc.WithInsecure())
		}

		exp, err = otlptracegrpc.New(ctx, clientOpts...)
	default:
		return nil, fmt.Errorf("unsupported otel exporter %q", cfg.OTELExporter)
	}

	if err != nil {
		return nil, fmt.Errorf("create exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.OTELServiceName),
			attribute.String("service.version", version),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	tracerProviderOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}

	if cfg.OTELExporter == otelExporterStdout {
		tracerProviderOpts = append(tracerProviderOpts, sdktrace.WithSyncer(exp))
	} else {
		tracerProviderOpts = append(tracerProviderOpts, sdktrace.WithBatcher(exp))
	}

	tp := sdktrace.NewTracerProvider(tracerProviderOpts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

func wrapHTTPHandlerWithTelemetry(h http.Handler, cfg appConfig) http.Handler {
	if cfg.OTELEnabled {
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
