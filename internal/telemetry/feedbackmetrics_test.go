package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestFeedbackMetrics_Record(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))

	fm := NewFeedbackMetrics(provider.Meter("test"))

	fm.RecordFeedback(context.Background(), "useful", "SEC001")
	fm.RecordFeedback(context.Background(), "noise", "QA003")
	fm.RecordRates(context.Background(), 0.25, 0.5)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(rm.ScopeMetrics) == 0 {
		t.Fatal("no metrics recorded")
	}
}
