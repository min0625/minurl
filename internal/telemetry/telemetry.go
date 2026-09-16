// Copyright 2026 The MinURL Authors

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
		otelHandler := otelhttp.NewHandler(
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

		// otelhttp replaces r.Body with its own wrapper on the request it is given. On the
		// server's own request, net/http then no longer recognizes the body it created, so
		// after a failed body read (a timeout, broken chunked encoding) it keeps the connection
		// open and parses the unread rest of the body as the next request. A shallow copy keeps
		// the replacement off the server's request. otelhttp v0.69.0 restores the body itself
		// once the handler returns.
		//
		//   - otelhttp v0.68.0: https://github.com/open-telemetry/opentelemetry-go-contrib/blob/instrumentation/net/http/otelhttp/v0.68.0/instrumentation/net/http/otelhttp/handler.go#L139-L142
		//   - net/http: https://github.com/golang/go/blob/go1.26.8/src/net/http/server.go#L1385-L1427
		//   - otelhttp v0.69.0: https://github.com/open-telemetry/opentelemetry-go-contrib/blob/instrumentation/net/http/otelhttp/v0.69.0/instrumentation/net/http/otelhttp/handler.go#L141-L149
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			otelHandler.ServeHTTP(w, r.WithContext(r.Context()))
		})
	}

	return h
}
