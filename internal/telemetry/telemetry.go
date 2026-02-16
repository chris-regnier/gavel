package telemetry

import (
	"context"
	"errors"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"

	"github.com/chris-regnier/gavel/internal/config"
)

// Init initializes OTel providers and returns a shutdown function.
// If telemetry is disabled, returns a no-op shutdown.
func Init(ctx context.Context, cfg config.TelemetryConfig) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }

	// Check env var override
	if envEnabled := os.Getenv("GAVEL_TELEMETRY_ENABLED"); envEnabled != "" {
		cfg.Enabled = strings.EqualFold(envEnabled, "true") || envEnabled == "1"
	}

	if !cfg.Enabled {
		return noop, nil
	}

	// Build resource with service info
	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		))
	if err != nil {
		return noop, err
	}

	// Create trace exporter
	var traceExporter trace.SpanExporter
	switch cfg.Protocol {
	case "http":
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
		}
		traceExporter, err = otlptracehttp.New(ctx, opts...)
	default: // "grpc" or empty
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(cfg.Headers))
		}
		traceExporter, err = otlptracegrpc.New(ctx, opts...)
	}
	if err != nil {
		return noop, err
	}

	// Create metric exporter
	var metricExporter metric.Exporter
	switch cfg.Protocol {
	case "http":
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlpmetrichttp.WithHeaders(cfg.Headers))
		}
		metricExporter, err = otlpmetrichttp.New(ctx, opts...)
	default:
		opts := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
		}
		if cfg.Insecure {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlpmetricgrpc.WithHeaders(cfg.Headers))
		}
		metricExporter, err = otlpmetricgrpc.New(ctx, opts...)
	}
	if err != nil {
		return noop, err
	}

	// Create TracerProvider with ParentBased sampler
	sampler := trace.ParentBased(trace.TraceIDRatioBased(cfg.SampleRate))
	tp := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter),
		trace.WithResource(res),
		trace.WithSampler(sampler),
	)

	// Create MeterProvider with periodic reader (60s for CLI)
	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter)),
		metric.WithResource(res),
	)

	// Set global providers
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return combined shutdown
	shutdown = func(ctx context.Context) error {
		return errors.Join(
			tp.Shutdown(ctx),
			mp.Shutdown(ctx),
		)
	}

	return shutdown, nil
}
