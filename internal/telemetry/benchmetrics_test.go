package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestBenchMetrics_Record(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))

	bm := NewBenchMetrics(provider.Meter("test"))

	bm.RecordQuality(context.Background(), QualityMetrics{
		Precision:       0.82,
		Recall:          0.91,
		F1:              0.86,
		HallucinRate:    0.04,
		NoiseRate:       0.12,
		ConfCalibration: 0.78,
	}, "claude-sonnet-4", "anthropic", "code-reviewer")

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(rm.ScopeMetrics) == 0 {
		t.Fatal("no metrics recorded")
	}
}
