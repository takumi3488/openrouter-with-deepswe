// Package telemetry wires up OpenTelemetry tracing for the binaries in this
// repository. Every cmd exports traces via OTLP/gRPC to the collector
// endpoint configured in internal/config.
package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

// Shutdown flushes and stops the tracer provider. Callers should defer it
// right after a successful Setup call.
type Shutdown func(context.Context) error

// Setup configures the global OpenTelemetry tracer provider to export spans
// to endpoint via OTLP/gRPC (insecure, as is standard for a local/sidecar
// collector such as Jaeger). It never fails processing because the
// collector is unreachable: exporter errors surface only when spans are
// flushed, and are logged by the caller via the returned Shutdown, not
// treated as fatal here.
func Setup(ctx context.Context, serviceName, endpoint string) (Shutdown, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: create OTLP exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// Tracer returns a tracer scoped to name, typically the calling package's
// import path.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
