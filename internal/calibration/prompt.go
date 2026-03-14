package calibration

import (
	"fmt"
	"strings"
)

// FormatCalibrationExamples formats few-shot examples for prompt injection.
//
// It renders a structured calibration context block containing labelled
// historical examples from team feedback. Examples with verdict "useful" are
// labelled "USEFUL PATTERN" to reinforce correct detection; examples with
// verdict "noise" or "wrong" are labelled "NOISE PATTERN" to discourage
// similar false positives. The returned string is intended to be prepended
// to an LLM analysis prompt so the model can calibrate its confidence
// against known outcomes for the same rule and file type.
//
// Returns an empty string when examples is nil or empty.
func FormatCalibrationExamples(examples []FewShotExample) string {
	if len(examples) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n--- Calibration Context ---\n")
	b.WriteString("Based on historical review feedback on similar code:\n\n")
	for _, ex := range examples {
		label := "USEFUL PATTERN"
		if ex.Verdict == "noise" || ex.Verdict == "wrong" {
			label = "NOISE PATTERN"
		}
		b.WriteString(fmt.Sprintf("[%s] Rule %s:\n", label, ex.RuleID))
		b.WriteString(fmt.Sprintf("Finding: %q\n", ex.Message))
		if ex.Reason != "" {
			b.WriteString(fmt.Sprintf("Context: %s\n", ex.Reason))
		}
		b.WriteString("\n")
	}
	b.WriteString("Use these patterns to calibrate your confidence. Avoid raising findings\n")
	b.WriteString("similar to NOISE patterns unless you have strong evidence.\n")
	b.WriteString("---\n")
	return b.String()
}
