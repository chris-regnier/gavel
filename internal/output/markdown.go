package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// MarkdownFormatter renders analysis output as GitHub-Flavored Markdown
// suitable for PR comments. Uses collapsible <details> sections for findings
// and severity emojis for quick visual scanning.
type MarkdownFormatter struct{}

// severityPriority returns a sort priority for SARIF severity levels.
// Lower values sort first: error (0) > warning (1) > note (2).
func severityPriority(level string) int {
	switch level {
	case "error":
		return 0
	case "warning":
		return 1
	case "note":
		return 2
	default:
		return 3
	}
}

// severityEmoji returns the GitHub emoji shortcode for a SARIF severity level.
func severityEmoji(level string) string {
	switch level {
	case "error":
		return ":red_circle:"
	case "warning":
		return ":warning:"
	case "note":
		return ":information_source:"
	default:
		return ":grey_question:"
	}
}

// decisionBanner returns the emoji + text for a verdict decision.
func decisionBanner(decision string) string {
	switch decision {
	case "merge":
		return ":white_check_mark: Merge"
	case "reject":
		return ":x: Reject"
	case "review":
		return ":warning: Review Required"
	default:
		return decision
	}
}

// resultFilePath extracts the file URI from the first location of a SARIF result.
func resultFilePath(r sarif.Result) string {
	if len(r.Locations) > 0 {
		return r.Locations[0].PhysicalLocation.ArtifactLocation.URI
	}
	return ""
}

// resultLineRange returns a human-readable line range string (e.g. "42-42" or "10-75").
func resultLineRange(r sarif.Result) string {
	if len(r.Locations) == 0 {
		return ""
	}
	region := r.Locations[0].PhysicalLocation.Region
	if region.StartLine == 0 {
		return ""
	}
	if region.EndLine == 0 || region.EndLine == region.StartLine {
		return fmt.Sprintf("%d-%d", region.StartLine, region.StartLine)
	}
	return fmt.Sprintf("%d-%d", region.StartLine, region.EndLine)
}

// resultConfidence extracts the gavel/confidence property from a SARIF result.
func resultConfidence(r sarif.Result) string {
	if r.Properties == nil {
		return ""
	}
	if v, ok := r.Properties["gavel/confidence"]; ok {
		return fmt.Sprintf("%.2f", v)
	}
	return ""
}

// resultRecommendation extracts the gavel/recommendation property from a SARIF result.
func resultRecommendation(r sarif.Result) string {
	if r.Properties == nil {
		return ""
	}
	if v, ok := r.Properties["gavel/recommendation"].(string); ok {
		return v
	}
	return ""
}

// Format produces GFM Markdown output from the analysis results.
func (f *MarkdownFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("markdown formatter: result is required")
	}
	if result.Verdict == nil {
		return nil, fmt.Errorf("markdown formatter: verdict is required")
	}

	var b strings.Builder

	// Extract results from SARIF log.
	var results []sarif.Result
	var persona string
	if result.SARIFLog != nil && len(result.SARIFLog.Runs) > 0 {
		run := result.SARIFLog.Runs[0]
		results = run.Results
		if run.Properties != nil {
			if p, ok := run.Properties["gavel/persona"].(string); ok {
				persona = p
			}
		}
	}

	// Count unique files and severity counts.
	fileSet := make(map[string]struct{})
	severityCounts := make(map[string]int)
	for _, r := range results {
		fp := resultFilePath(r)
		if fp != "" {
			fileSet[fp] = struct{}{}
		}
		severityCounts[r.Level]++
	}

	// Header.
	b.WriteString("## Gavel Analysis Summary\n\n")

	// Decision banner.
	b.WriteString(fmt.Sprintf("**Decision:** %s | **Findings:** %d | **Files:** %d\n",
		decisionBanner(result.Verdict.Decision),
		len(results),
		len(fileSet)))

	if len(results) == 0 {
		// No findings case.
		b.WriteString("\nNo findings detected.\n")
	} else {
		// Severity table.
		b.WriteString("\n### Findings by Severity\n")
		b.WriteString("| Severity | Count |\n")
		b.WriteString("|----------|-------|\n")

		// Display severity rows in a fixed order: error, warning, note.
		for _, level := range []string{"error", "warning", "note"} {
			if count, ok := severityCounts[level]; ok && count > 0 {
				b.WriteString(fmt.Sprintf("| %s    | %d     |\n", level, count))
			}
		}

		// Sort results: by severity priority first, then by file path.
		sorted := make([]sarif.Result, len(results))
		copy(sorted, results)
		sort.SliceStable(sorted, func(i, j int) bool {
			pi, pj := severityPriority(sorted[i].Level), severityPriority(sorted[j].Level)
			if pi != pj {
				return pi < pj
			}
			return resultFilePath(sorted[i]) < resultFilePath(sorted[j])
		})

		// Findings section.
		b.WriteString("\n### Findings\n\n")

		for _, r := range sorted {
			fp := resultFilePath(r)
			lineRange := resultLineRange(r)
			emoji := severityEmoji(r.Level)

			// Summary line for the collapsible section.
			locationStr := ""
			if fp != "" && lineRange != "" {
				locationStr = fmt.Sprintf(" in <code>%s:%s</code>", fp, strings.Split(lineRange, "-")[0])
			} else if fp != "" {
				locationStr = fmt.Sprintf(" in <code>%s</code>", fp)
			}

			b.WriteString("<details>\n")
			b.WriteString(fmt.Sprintf("<summary>%s <strong>%s</strong> — %s: %s%s</summary>\n\n",
				emoji, r.Level, r.RuleID, truncate(r.Message.Text, 80), locationStr))

			b.WriteString(fmt.Sprintf("**Rule:** %s\n", r.RuleID))

			confidence := resultConfidence(r)
			if confidence != "" {
				b.WriteString(fmt.Sprintf("**Confidence:** %s\n", confidence))
			}

			if fp != "" {
				if lineRange != "" {
					b.WriteString(fmt.Sprintf("**File:** `%s` lines %s\n", fp, lineRange))
				} else {
					b.WriteString(fmt.Sprintf("**File:** `%s`\n", fp))
				}
			}

			b.WriteString(fmt.Sprintf("\n> %s\n", r.Message.Text))

			recommendation := resultRecommendation(r)
			if recommendation != "" {
				b.WriteString(fmt.Sprintf("\n**Recommendation:** %s\n", recommendation))
			}

			b.WriteString("\n</details>\n\n")
		}
	}

	// Footer.
	b.WriteString("---\n")
	if persona != "" {
		b.WriteString(fmt.Sprintf("*Generated by [Gavel](https://github.com/chris-regnier/gavel) · %s persona*\n", persona))
	} else {
		b.WriteString("*Generated by [Gavel](https://github.com/chris-regnier/gavel)*\n")
	}

	return []byte(b.String()), nil
}

// truncate shortens a string to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
