package calibration

import (
	"strings"
	"testing"
)

func TestFormatCalibrationExamples(t *testing.T) {
	examples := []FewShotExample{
		{RuleID: "SEC001", Message: "SQL injection via string concatenation", Verdict: "useful", Reason: "Confirmed vulnerability, was fixed"},
		{RuleID: "SEC001", Message: "Potential SQL injection in user lookup", Verdict: "noise", Reason: "Uses parameterized queries"},
	}
	result := FormatCalibrationExamples(examples)
	if !strings.Contains(result, "USEFUL PATTERN") {
		t.Error("missing USEFUL PATTERN label")
	}
	if !strings.Contains(result, "NOISE PATTERN") {
		t.Error("missing NOISE PATTERN label")
	}
	if !strings.Contains(result, "SQL injection via string concatenation") {
		t.Error("missing useful finding message")
	}
	if !strings.Contains(result, "parameterized queries") {
		t.Error("missing noise reason")
	}
}

func TestFormatCalibrationExamples_Empty(t *testing.T) {
	result := FormatCalibrationExamples(nil)
	if result != "" {
		t.Errorf("expected empty string for nil examples, got %q", result)
	}
}
