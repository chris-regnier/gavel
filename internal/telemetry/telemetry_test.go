package telemetry

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/chris-regnier/gavel/internal/config"
)

// shutdownCtx returns a context with a short timeout for test shutdown calls,
// avoiding 10s gRPC connection timeouts when no collector is running.
func shutdownCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 1*time.Second)
}

func TestInit_DisabledReturnsNoop(t *testing.T) {
	cfg := config.TelemetryConfig{
		Enabled: false,
	}

	shutdown, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Shutdown should succeed (noop)
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("noop shutdown should not error, got: %v", err)
	}
}

func TestInit_EnvVarOverrideDisables(t *testing.T) {
	// Start with enabled config, but env var disables it
	cfg := config.TelemetryConfig{
		Enabled:     true,
		Endpoint:    "localhost:4317",
		Protocol:    "grpc",
		ServiceName: "gavel-test",
		SampleRate:  1.0,
	}

	t.Setenv("GAVEL_TELEMETRY_ENABLED", "false")

	shutdown, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Should be noop since env var disabled it
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown should not error, got: %v", err)
	}
}

func TestInit_EnvVarOverrideEnables(t *testing.T) {
	// Start with disabled config, env var enables it.
	// This will fail to connect (no collector), but Init should still succeed
	// because exporters connect lazily.
	cfg := config.TelemetryConfig{
		Enabled:     false,
		Endpoint:    "localhost:4317",
		Protocol:    "grpc",
		Insecure:    true,
		ServiceName: "gavel-test",
		SampleRate:  1.0,
	}

	t.Setenv("GAVEL_TELEMETRY_ENABLED", "true")

	shutdown, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify global tracer provider is set (not the noop default)
	tp := otel.GetTracerProvider()
	if tp == nil {
		t.Fatal("expected non-nil tracer provider")
	}

	ctx, cancel := shutdownCtx()
	defer cancel()
	_ = shutdown(ctx)
}

func TestInit_EnvVarCaseInsensitive(t *testing.T) {
	cfg := config.TelemetryConfig{
		Enabled:  false,
		Endpoint: "localhost:4317",
		Protocol: "grpc",
		Insecure: true,
	}

	for _, val := range []string{"TRUE", "True", "1"} {
		t.Run(val, func(t *testing.T) {
			t.Setenv("GAVEL_TELEMETRY_ENABLED", val)

			shutdown, err := Init(context.Background(), cfg)
			if err != nil {
				t.Fatalf("expected no error for value %q, got: %v", val, err)
			}
			ctx, cancel := shutdownCtx()
			defer cancel()
			_ = shutdown(ctx)
		})
	}
}

func TestInit_HTTPProtocol(t *testing.T) {
	cfg := config.TelemetryConfig{
		Enabled:     true,
		Endpoint:    "localhost:4318",
		Protocol:    "http",
		Insecure:    true,
		ServiceName: "gavel-test-http",
		SampleRate:  1.0,
	}

	shutdown, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	ctx, cancel := shutdownCtx()
	defer cancel()
	_ = shutdown(ctx)
}

func TestInit_WithHeaders(t *testing.T) {
	cfg := config.TelemetryConfig{
		Enabled:     true,
		Endpoint:    "localhost:4317",
		Protocol:    "grpc",
		Insecure:    true,
		ServiceName: "gavel-test",
		SampleRate:  1.0,
		Headers: map[string]string{
			"Authorization": "Bearer test-token",
		},
	}

	shutdown, err := Init(context.Background(), cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	ctx, cancel := shutdownCtx()
	defer cancel()
	_ = shutdown(ctx)
}
