package bench

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// WriteJSON writes the comparison report as indented JSON.
func WriteJSON(w io.Writer, report *ComparisonReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// WriteMarkdown writes a top-line summary table in Markdown format.
func WriteMarkdown(w io.Writer, report *ComparisonReport) error {
	var sb strings.Builder
	sb.WriteString("# Model Benchmark Results\n\n")
	sb.WriteString(fmt.Sprintf("**Date:** %s | **Runs per model:** %d | **Corpus size:** %d\n\n",
		report.Metadata.Timestamp.Format("2006-01-02"), report.Metadata.RunsPerModel, report.Metadata.CorpusSize))
	sb.WriteString("## Comparison\n\n")
	sb.WriteString("| Model | F1 | Precision | Recall | Latency (p50) | Cost/File | Use Case |\n")
	sb.WriteString("|-------|-----|-----------|--------|---------------|-----------|----------|\n")
	for _, m := range report.Models {
		useCase := recommendUseCase(m)
		sb.WriteString(fmt.Sprintf("| %s | %.2f | %.2f | %.2f | %dms | $%.4f | %s |\n",
			m.ModelID, m.Quality.F1, m.Quality.Precision, m.Quality.Recall,
			m.Latency.P50Ms, m.Cost.PerFileAvgUSD, useCase))
	}
	sb.WriteString("\n## How to reproduce\n\n```bash\n")
	sb.WriteString(fmt.Sprintf("gavel bench --runs %d\n", report.Metadata.RunsPerModel))
	sb.WriteString("```\n\nFor detailed results, see the structured JSON in `.gavel/bench/`.\n\n")
	sb.WriteString("> **Note:** Token counts and costs are estimates (character-count/4 heuristic). Actual costs may vary by ~30%.\n")
	_, err := io.WriteString(w, sb.String())
	return err
}

func recommendUseCase(m ModelResult) string {
	if m.Quality.F1 >= 0.85 && m.Cost.PerFileAvgUSD > 0.01 {
		return "High-stakes review"
	}
	if m.Quality.F1 >= 0.80 && m.Latency.P50Ms < 3000 {
		return "CI default"
	}
	if m.Cost.PerFileAvgUSD < 0.001 {
		return "Budget / bulk"
	}
	if m.Latency.P50Ms < 2000 {
		return "Fast iteration"
	}
	return "General purpose"
}
