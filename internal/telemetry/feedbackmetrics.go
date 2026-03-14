package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// FeedbackMetrics wraps OTel instruments for feedback tracking.
type FeedbackMetrics struct {
	count      metric.Int64Counter
	noiseRate  metric.Float64Gauge
	usefulRate metric.Float64Gauge
}

// NewFeedbackMetrics creates OTel instruments for feedback tracking.
func NewFeedbackMetrics(meter metric.Meter) *FeedbackMetrics {
	fm := &FeedbackMetrics{}
	fm.count, _ = meter.Int64Counter("gavel.feedback.count",
		metric.WithDescription("Total feedback submissions"))
	fm.noiseRate, _ = meter.Float64Gauge("gavel.feedback.noise_rate",
		metric.WithDescription("Rate of findings marked as noise"))
	fm.usefulRate, _ = meter.Float64Gauge("gavel.feedback.useful_rate",
		metric.WithDescription("Rate of findings marked as useful"))
	return fm
}

// RecordFeedback records a single feedback submission.
func (fm *FeedbackMetrics) RecordFeedback(ctx context.Context, verdict string, ruleID string) {
	fm.count.Add(ctx, 1, metric.WithAttributes(
		attribute.String("gavel.feedback.verdict", verdict),
		attribute.String("gavel.feedback.rule_id", ruleID),
	))
}

// RecordRates records aggregate noise and useful rates.
func (fm *FeedbackMetrics) RecordRates(ctx context.Context, noiseRate, usefulRate float64) {
	fm.noiseRate.Record(ctx, noiseRate)
	fm.usefulRate.Record(ctx, usefulRate)
}
