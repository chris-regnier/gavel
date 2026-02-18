// Package output provides formatters for rendering Gavel analysis results
// in different output formats (JSON, SARIF, Markdown, pretty terminal).
package output

import (
	"fmt"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

// Formatter renders an AnalysisOutput into a byte slice in a specific format.
type Formatter interface {
	Format(result *AnalysisOutput) ([]byte, error)
}

// AnalysisOutput holds the complete results of a Gavel analysis run,
// combining the verdict, SARIF log, and optional tiered analyzer statistics.
type AnalysisOutput struct {
	Verdict  *store.Verdict
	SARIFLog *sarif.Log
	Stats    *analyzer.TieredAnalyzerStats // optional, nil if not collected
}

// ResolveFormat determines the output format to use. If flagValue is non-empty,
// it is returned directly. Otherwise, "pretty" is returned for TTY output and
// "json" for non-TTY (piped) output.
func ResolveFormat(flagValue string, stdoutIsTTY bool) string {
	if flagValue != "" {
		return flagValue
	}
	if stdoutIsTTY {
		return "pretty"
	}
	return "json"
}

// NewFormatter returns a Formatter for the given format name.
// Supported formats: "json", "sarif", "markdown", "pretty".
// Returns an error for unknown format names.
func NewFormatter(format string) (Formatter, error) {
	switch format {
	case "json":
		return &JSONFormatter{}, nil
	case "sarif":
		return &SARIFFormatter{}, nil
	case "markdown":
		return &MarkdownFormatter{}, nil
	case "pretty":
		return &PrettyFormatter{}, nil
	default:
		return nil, fmt.Errorf("unknown output format: %q (supported: json, sarif, markdown, pretty)", format)
	}
}
