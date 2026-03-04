package harness

import (
	"encoding/json"
	"fmt"
	"os"
)

// RunMetrics captures metrics from a single analysis run
type RunMetrics struct {
	// Run is the iteration number (1-indexed)
	Run int `json:"run"`

	// Variant is the name of the variant
	Variant string `json:"variant"`

	// Package is the analyzed package/directory
	Package string `json:"package"`

	// Total findings count
	Total int `json:"total"`

	// LLM findings count (tier=comprehensive)
	LLM int `json:"llm"`

	// Instant findings count (tier=instant, from patterns/AST)
	Instant int `json:"instant"`

	// Error-level findings count
	Errors int `json:"errors"`

	// Warning-level findings count
	Warnings int `json:"warnings"`

	// Note-level findings count
	Notes int `json:"notes"`

	// None-level findings count
	Nones int `json:"nones"`

	// HighConfErrors is the count of error-level findings with confidence > 0.8
	HighConfErrors int `json:"errs_hi_conf"`

	// AvgConfidence is the average confidence across all findings
	AvgConfidence float64 `json:"avg_conf"`

	// Decision is the Rego verdict (merge, review, reject)
	Decision string `json:"decision"`

	// ResultID is the SARIF result identifier
	ResultID string `json:"id"`

	// Duration is the analysis duration in milliseconds
	Duration int64 `json:"duration_ms,omitempty"`
}

// WriteJSONL appends a RunMetrics record to a JSONL file
func (m *RunMetrics) WriteJSONL(path string) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshaling metrics: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening results file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		return fmt.Errorf("writing metrics: %w", err)
	}

	return nil
}

// AggregateMetrics aggregates metrics across multiple runs
type AggregateMetrics struct {
	Variant string
	Runs    int

	Total         Stat
	LLM           Stat
	Instant       Stat
	Errors        Stat
	Warnings      Stat
	Notes         Stat
	Nones         Stat
	HighConfErrors Stat
	AvgConfidence  Stat
}

// Stat holds mean and standard deviation
type Stat struct {
	Mean float64
	Std  float64
}

// Summary represents the summarized results of a harness run
type Summary struct {
	Variants []VariantSummary `json:"variants"`
	Baseline string           `json:"baseline,omitempty"`
}

// VariantSummary represents summarized metrics for a single variant
type VariantSummary struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Runs        int     `json:"runs"`
	Total       Stat    `json:"total"`
	LLM         Stat    `json:"llm"`
	Instant     Stat    `json:"instant"`
	Errors      Stat    `json:"errors"`
	Warnings    Stat    `json:"warnings"`
	Notes       Stat    `json:"notes"`
	HighConf    Stat    `json:"high_conf_errors"`
	AvgConf     Stat    `json:"avg_confidence"`
	Delta       *Delta  `json:"delta,omitempty"`
}

// Delta represents the difference from baseline
type Delta struct {
	TotalPct      float64 `json:"total_pct,omitempty"`
	LLMPct        float64 `json:"llm_pct,omitempty"`
	ErrorsPct     float64 `json:"errors_pct,omitempty"`
	HighConfPct   float64 `json:"high_conf_pct,omitempty"`
	AvgConfDiff   float64 `json:"avg_conf_diff,omitempty"`
}
