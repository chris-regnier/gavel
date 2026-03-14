package calibration

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestApplyThresholds_SuppressLowConfidence(t *testing.T) {
	results := []sarif.Result{
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.3}},
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.8}},
		{RuleID: "SEC002", Properties: map[string]interface{}{"gavel/confidence": 0.5}},
	}
	thresholds := map[string]ThresholdOverride{"SEC001": {SuppressBelow: 0.5}}
	filtered := ApplyThresholds(results, thresholds)
	if len(filtered) != 2 {
		t.Errorf("filtered = %d, want 2", len(filtered))
	}
}

func TestApplyThresholds_NoThresholds(t *testing.T) {
	results := []sarif.Result{
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.3}},
	}
	filtered := ApplyThresholds(results, nil)
	if len(filtered) != 1 {
		t.Errorf("filtered = %d, want 1", len(filtered))
	}
}

func TestSuppressedResults(t *testing.T) {
	results := []sarif.Result{
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.3}},
		{RuleID: "SEC001", Properties: map[string]interface{}{"gavel/confidence": 0.8}},
	}
	thresholds := map[string]ThresholdOverride{"SEC001": {SuppressBelow: 0.5}}
	suppressed := SuppressedResults(results, thresholds)
	if len(suppressed) != 1 {
		t.Errorf("suppressed = %d, want 1", len(suppressed))
	}
}
