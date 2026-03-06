package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// QualityMetrics holds quality scores to emit as OTel gauges.
type QualityMetrics struct {
	Precision       float64
	Recall          float64
	F1              float64
	HallucinRate    float64
	NoiseRate       float64
	ConfCalibration float64
}

// BenchMetrics wraps OTel instruments for benchmark quality metrics.
type BenchMetrics struct {
	precision       metric.Float64Gauge
	recall          metric.Float64Gauge
	f1              metric.Float64Gauge
	hallucinRate    metric.Float64Gauge
	noiseRate       metric.Float64Gauge
	confCalibration metric.Float64Gauge
}

// NewBenchMetrics creates OTel instruments for benchmark quality tracking.
func NewBenchMetrics(meter metric.Meter) *BenchMetrics {
	bm := &BenchMetrics{}
	bm.precision, _ = meter.Float64Gauge("gavel.bench.precision",
		metric.WithDescription("Benchmark precision score"))
	bm.recall, _ = meter.Float64Gauge("gavel.bench.recall",
		metric.WithDescription("Benchmark recall score"))
	bm.f1, _ = meter.Float64Gauge("gavel.bench.f1",
		metric.WithDescription("Benchmark F1 score"))
	bm.hallucinRate, _ = meter.Float64Gauge("gavel.bench.hallucination_rate",
		metric.WithDescription("Benchmark hallucination rate"))
	bm.noiseRate, _ = meter.Float64Gauge("gavel.bench.noise_rate",
		metric.WithDescription("Benchmark noise rate"))
	bm.confCalibration, _ = meter.Float64Gauge("gavel.bench.confidence_calibration",
		metric.WithDescription("Confidence calibration score"))
	return bm
}

// RecordQuality records quality metrics with model/provider/persona tags.
func (bm *BenchMetrics) RecordQuality(ctx context.Context, q QualityMetrics, model, provider, persona string) {
	attrs := metric.WithAttributes(
		attribute.String("gavel.model", model),
		attribute.String("gavel.provider", provider),
		attribute.String("gavel.persona", persona),
	)
	bm.precision.Record(ctx, q.Precision, attrs)
	bm.recall.Record(ctx, q.Recall, attrs)
	bm.f1.Record(ctx, q.F1, attrs)
	bm.hallucinRate.Record(ctx, q.HallucinRate, attrs)
	bm.noiseRate.Record(ctx, q.NoiseRate, attrs)
	bm.confCalibration.Record(ctx, q.ConfCalibration, attrs)
}
