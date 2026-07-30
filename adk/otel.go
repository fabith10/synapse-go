package adk

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	syslog "github.com/fabith10/agent-framework/pkg/logger"
)

const TracerName = "github.com/fabith10/agent-framework/adk"

// SetupOTLPTracer configures a global OpenTelemetry TracerProvider using an OTLP stdout/http exporter.
// Returns a shutdown function that flushes pending spans on application exit.
func SetupOTLPTracer(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	if serviceName == "" {
		serviceName = "agent-framework"
	}

	var opts []stdouttrace.Option
	if os.Getenv("OTEL_TRACER_DEBUG") == "true" {
		opts = append(opts, stdouttrace.WithPrettyPrint())
	} else {
		opts = append(opts, stdouttrace.WithoutTimestamps())
	}

	exporter, err := stdouttrace.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("adk: create OTLP trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("adk: create OTLP resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	syslog.Info("OpenTelemetry Tracer initialized", "service_name", serviceName, "endpoint", endpoint)

	return tp.Shutdown, nil
}

// GetTracer returns the package tracer instance.
func GetTracer() trace.Tracer {
	return otel.Tracer(TracerName)
}
