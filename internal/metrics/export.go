package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Exporter handles exporting metrics to various formats
type Exporter struct {
	collector *Collector
}

// NewExporter creates a new metrics exporter
func NewExporter(collector *Collector) *Exporter {
	return &Exporter{collector: collector}
}

// ExportJSON writes metrics to a JSON file
func (e *Exporter) ExportJSON(path string) error {
	stats := e.collector.GetStats()
	events := e.collector.GetRecentEvents(1000)

	report := struct {
		GeneratedAt time.Time        `json:"generated_at"`
		Stats       AggregateStats   `json:"stats"`
		Events      []AnalysisEvent  `json:"events"`
	}{
		GeneratedAt: time.Now(),
		Stats:       stats,
		Events:      events,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metrics: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// ExportStatsJSON writes only aggregate stats to a JSON file
func (e *Exporter) ExportStatsJSON(path string) error {
	stats := e.collector.GetStats()

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling stats: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// WriteReport writes a human-readable report to the given writer
func (e *Exporter) WriteReport(w io.Writer) error {
	stats := e.collector.GetStats()

	fmt.Fprintf(w, "Gavel Analysis Metrics Report\n")
	fmt.Fprintf(w, "Generated: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "Window: %s to %s\n\n", 
		stats.WindowStart.Format(time.RFC3339),
		stats.WindowEnd.Format(time.RFC3339))

	fmt.Fprintf(w, "=== Summary ===\n")
	fmt.Fprintf(w, "Total Analyses:     %d\n", stats.TotalAnalyses)
	fmt.Fprintf(w, "Total Errors:       %d (%.1f%%)\n", 
		stats.TotalErrors,
		safePercent(float64(stats.TotalErrors), float64(stats.TotalAnalyses)))
	fmt.Fprintf(w, "Total Findings:     %d\n", stats.TotalFindings)
	fmt.Fprintf(w, "Findings/Analysis:  %.2f\n\n", stats.FindingsPerAnalysis)

	fmt.Fprintf(w, "=== Latency ===\n")
	fmt.Fprintf(w, "Average:  %.0fms\n", stats.AvgAnalysisDurationMs)
	fmt.Fprintf(w, "P50:      %.0fms\n", stats.P50AnalysisDurationMs)
	fmt.Fprintf(w, "P95:      %.0fms\n", stats.P95AnalysisDurationMs)
	fmt.Fprintf(w, "P99:      %.0fms\n", stats.P99AnalysisDurationMs)
	fmt.Fprintf(w, "Max:      %.0fms\n", stats.MaxAnalysisDurationMs)
	fmt.Fprintf(w, "Avg Queue: %.0fms\n", stats.AvgQueueDurationMs)
	fmt.Fprintf(w, "Avg Total: %.0fms\n\n", stats.AvgTotalDurationMs)

	fmt.Fprintf(w, "=== Cache ===\n")
	fmt.Fprintf(w, "Hits:     %d\n", stats.CacheHits)
	fmt.Fprintf(w, "Misses:   %d\n", stats.CacheMisses)
	fmt.Fprintf(w, "Stale:    %d\n", stats.CacheStale)
	fmt.Fprintf(w, "Hit Rate: %.1f%%\n\n", stats.CacheHitRate*100)

	fmt.Fprintf(w, "=== Throughput ===\n")
	fmt.Fprintf(w, "Analyses/min: %.2f\n\n", stats.AnalysesPerMinute)

	fmt.Fprintf(w, "=== Tokens ===\n")
	fmt.Fprintf(w, "Total In:    %d\n", stats.TotalTokensIn)
	fmt.Fprintf(w, "Total Out:   %d\n", stats.TotalTokensOut)
	fmt.Fprintf(w, "Avg In:      %.0f\n", stats.AvgTokensIn)
	fmt.Fprintf(w, "Avg Out:     %.0f\n\n", stats.AvgTokensOut)

	if len(stats.ByTier) > 0 {
		fmt.Fprintf(w, "=== By Tier ===\n")
		for tier, tierStats := range stats.ByTier {
			fmt.Fprintf(w, "%s:\n", tier)
			fmt.Fprintf(w, "  Count:      %d\n", tierStats.Count)
			fmt.Fprintf(w, "  Avg Latency: %.0fms\n", tierStats.AvgAnalysisDurationMs)
			fmt.Fprintf(w, "  Error Rate: %.1f%%\n", tierStats.ErrorRate*100)
		}
	}

	return nil
}

// WriteCSV writes events in CSV format for external analysis
func (e *Exporter) WriteCSV(w io.Writer) error {
	events := e.collector.GetRecentEvents(e.collector.maxEvents)

	// Header
	fmt.Fprintf(w, "id,timestamp,type,tier,file_path,file_size,line_count,chunk_count,policy_count,")
	fmt.Fprintf(w, "queue_duration_ms,analysis_duration_ms,total_duration_ms,")
	fmt.Fprintf(w, "finding_count,error_count,tokens_in,tokens_out,")
	fmt.Fprintf(w, "cache_result,provider,model,error\n")

	for _, e := range events {
		fmt.Fprintf(w, "%s,%s,%s,%s,%s,%d,%d,%d,%d,",
			e.ID,
			e.Timestamp.Format(time.RFC3339),
			e.Type,
			e.Tier.String(),
			escapeCSV(e.FilePath),
			e.FileSize,
			e.LineCount,
			e.ChunkCount,
			e.PolicyCount,
		)
		fmt.Fprintf(w, "%d,%d,%d,",
			e.QueueDuration.Milliseconds(),
			e.AnalysisDuration.Milliseconds(),
			e.TotalDuration.Milliseconds(),
		)
		fmt.Fprintf(w, "%d,%d,%d,%d,",
			e.FindingCount,
			e.ErrorCount,
			e.TokensIn,
			e.TokensOut,
		)
		fmt.Fprintf(w, "%s,%s,%s,%s\n",
			e.CacheResult,
			e.Provider,
			e.Model,
			escapeCSV(e.Error),
		)
	}

	return nil
}

func safePercent(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return (numerator / denominator) * 100
}

func escapeCSV(s string) string {
	if s == "" {
		return ""
	}
	// Simple CSV escaping - wrap in quotes if contains comma, quote, or newline
	needsQuotes := false
	for _, r := range s {
		if r == ',' || r == '"' || r == '\n' || r == '\r' {
			needsQuotes = true
			break
		}
	}
	if !needsQuotes {
		return s
	}
	// Escape quotes by doubling them
	escaped := ""
	for _, r := range s {
		if r == '"' {
			escaped += "\"\""
		} else {
			escaped += string(r)
		}
	}
	return "\"" + escaped + "\""
}
