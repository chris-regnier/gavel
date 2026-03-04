package harness

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// Summarize reads a JSONL file and produces aggregate metrics
func Summarize(resultsPath string) (*Summary, error) {
	records, err := loadResults(resultsPath)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("no records found in %s", resultsPath)
	}

	// Group by variant
	variantRecords := make(map[string][]RunMetrics)
	for _, r := range records {
		variantRecords[r.Variant] = append(variantRecords[r.Variant], r)
	}

	summary := &Summary{
		Variants: make([]VariantSummary, 0, len(variantRecords)),
	}

	for name, recs := range variantRecords {
		vs := summarizeVariant(name, recs)
		summary.Variants = append(summary.Variants, vs)
	}

	// Sort variants by name
	sort.Slice(summary.Variants, func(i, j int) bool {
		return summary.Variants[i].Name < summary.Variants[j].Name
	})

	return summary, nil
}

// SummarizeWithBaseline produces a summary with delta calculations from a baseline
func SummarizeWithBaseline(resultsPath, baseline string) (*Summary, error) {
	summary, err := Summarize(resultsPath)
	if err != nil {
		return nil, err
	}

	summary.Baseline = baseline

	// Find baseline variant
	var baselineVar *VariantSummary
	for i, v := range summary.Variants {
		if v.Name == baseline {
			baselineVar = &summary.Variants[i]
			break
		}
	}

	if baselineVar == nil {
		return summary, nil // No baseline found, skip delta calculation
	}

	// Calculate deltas
	for i, v := range summary.Variants {
		if v.Name == baseline {
			continue
		}

		summary.Variants[i].Delta = &Delta{
			TotalPct:    pctDelta(v.Total.Mean, baselineVar.Total.Mean),
			LLMPct:      pctDelta(v.LLM.Mean, baselineVar.LLM.Mean),
			ErrorsPct:   pctDelta(v.Errors.Mean, baselineVar.Errors.Mean),
			HighConfPct: pctDelta(v.HighConf.Mean, baselineVar.HighConf.Mean),
			AvgConfDiff: v.AvgConf.Mean - baselineVar.AvgConf.Mean,
		}
	}

	return summary, nil
}

// summarizeVariant aggregates metrics for a single variant
func summarizeVariant(name string, recs []RunMetrics) VariantSummary {
	vs := VariantSummary{
		Name: name,
		Runs: len(recs),
	}

	// Collect values for each metric
	var totals, llms, instants, errors, warnings, notes, nones, highConf []float64
	var confs []float64

	for _, r := range recs {
		totals = append(totals, float64(r.Total))
		llms = append(llms, float64(r.LLM))
		instants = append(instants, float64(r.Instant))
		errors = append(errors, float64(r.Errors))
		warnings = append(warnings, float64(r.Warnings))
		notes = append(notes, float64(r.Notes))
		nones = append(nones, float64(r.Nones))
		highConf = append(highConf, float64(r.HighConfErrors))
		confs = append(confs, r.AvgConfidence)
	}

	vs.Total = calcStat(totals)
	vs.LLM = calcStat(llms)
	vs.Instant = calcStat(instants)
	vs.Errors = calcStat(errors)
	vs.Warnings = calcStat(warnings)
	vs.Notes = calcStat(notes)
	vs.HighConf = calcStat(highConf)
	vs.AvgConf = calcStat(confs)

	return vs
}

// calcStat calculates mean and standard deviation
func calcStat(values []float64) Stat {
	if len(values) == 0 {
		return Stat{}
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	if len(values) < 2 {
		return Stat{Mean: mean, Std: 0}
	}

	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	std := math.Sqrt(variance / float64(len(values)-1))

	return Stat{Mean: mean, Std: std}
}

// pctDelta calculates percentage change from baseline
func pctDelta(value, baseline float64) float64 {
	if baseline == 0 {
		return 0
	}
	return (value - baseline) / baseline * 100
}

// loadResults reads JSONL records from a file
func loadResults(path string) ([]RunMetrics, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []RunMetrics
	dec := json.NewDecoder(f)

	for dec.More() {
		var r RunMetrics
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("parsing JSONL: %w", err)
		}
		records = append(records, r)
	}

	return records, nil
}

// PrintSummary prints a human-readable summary
func PrintSummary(summary *Summary) {
	fmt.Printf("AGGREGATE METRICS (averaged across %d runs)\n", summary.Variants[0].Runs)
	fmt.Println("=" + repeat("=", 70))

	for _, v := range summary.Variants {
		fmt.Printf("\n[%s]\n", v.Name)
		fmt.Printf("  %-22s %8.1f ±%-5.1f\n", "Total findings", v.Total.Mean, v.Total.Std)
		fmt.Printf("  %-22s %8.1f ±%-5.1f\n", "LLM findings", v.LLM.Mean, v.LLM.Std)
		fmt.Printf("  %-22s %8.1f ±%-5.1f\n", "Instant findings", v.Instant.Mean, v.Instant.Std)
		fmt.Printf("  %-22s %8.1f ±%-5.1f\n", "Error-level", v.Errors.Mean, v.Errors.Std)
		fmt.Printf("  %-22s %8.1f ±%-5.1f\n", "Warning-level", v.Warnings.Mean, v.Warnings.Std)
		fmt.Printf("  %-22s %8.1f ±%-5.1f\n", "Note-level", v.Notes.Mean, v.Notes.Std)
		fmt.Printf("  %-22s %8.1f ±%-5.1f\n", "Errors w/ conf>0.8", v.HighConf.Mean, v.HighConf.Std)
		fmt.Printf("  %-22s %8.3f ±%-5.3f\n", "Avg confidence", v.AvgConf.Mean, v.AvgConf.Std)

		if v.Delta != nil {
			fmt.Printf("\n  Delta from %s:\n", summary.Baseline)
			fmt.Printf("  %-22s %+.1f%%\n", "Total findings", v.Delta.TotalPct)
			fmt.Printf("  %-22s %+.1f%%\n", "LLM findings", v.Delta.LLMPct)
			fmt.Printf("  %-22s %+.1f%%\n", "Error-level", v.Delta.ErrorsPct)
			fmt.Printf("  %-22s %+.1f%%\n", "Errors w/ conf>0.8", v.Delta.HighConfPct)
			fmt.Printf("  %-22s %+.3f\n", "Avg confidence", v.Delta.AvgConfDiff)
		}
	}
}

// repeat creates a string of n repetitions of s
func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

// WriteSummaryYAML writes the summary to a YAML file (or returns data if path is empty)
func WriteSummaryYAML(summary *Summary, path string) ([]byte, error) {
	data, err := yaml.Marshal(summary)
	if err != nil {
		return nil, err
	}
	if path != "" {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// WriteSummaryJSON writes the summary to a JSON file
func WriteSummaryJSON(summary *Summary, path string) ([]byte, error) {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil, err
	}
	if path != "" {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nil, err
		}
	}
	return data, nil
}
