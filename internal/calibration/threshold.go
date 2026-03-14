package calibration

import "github.com/chris-regnier/gavel/internal/sarif"

// ApplyThresholds filters out findings below their rule's suppress threshold.
// Results with no matching threshold entry, or a zero SuppressBelow value, are
// always kept. When thresholds is empty the original slice is returned unchanged.
func ApplyThresholds(results []sarif.Result, thresholds map[string]ThresholdOverride) []sarif.Result {
	if len(thresholds) == 0 {
		return results
	}
	var filtered []sarif.Result
	for _, r := range results {
		th, ok := thresholds[r.RuleID]
		if !ok || th.SuppressBelow == 0 {
			filtered = append(filtered, r)
			continue
		}
		conf, _ := r.Properties["gavel/confidence"].(float64)
		if conf >= th.SuppressBelow {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// SuppressedResults returns the subset of findings that would be suppressed by
// the given thresholds — i.e. findings whose confidence is strictly below their
// rule's SuppressBelow value. Returns nil when thresholds is empty.
func SuppressedResults(results []sarif.Result, thresholds map[string]ThresholdOverride) []sarif.Result {
	if len(thresholds) == 0 {
		return nil
	}
	var suppressed []sarif.Result
	for _, r := range results {
		th, ok := thresholds[r.RuleID]
		if !ok || th.SuppressBelow == 0 {
			continue
		}
		conf, _ := r.Properties["gavel/confidence"].(float64)
		if conf < th.SuppressBelow {
			suppressed = append(suppressed, r)
		}
	}
	return suppressed
}
