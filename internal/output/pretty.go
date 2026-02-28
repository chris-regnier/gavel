package output

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// PrettyFormatter renders analysis output as colored, human-readable
// terminal output suitable for interactive use. Output is grouped by file,
// sorted alphabetically, with findings sorted by line number within each file.
// Respects the NO_COLOR environment variable (https://no-color.org/).
type PrettyFormatter struct{}

// Format produces pretty terminal output from the analysis results.
func (f *PrettyFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	if result == nil {
		return nil, fmt.Errorf("pretty formatter: result is required")
	}

	noColor := os.Getenv("NO_COLOR") != ""

	// Extract results and metadata from SARIF log.
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

	decision := "unknown"
	if result.Verdict != nil {
		decision = result.Verdict.Decision
	}

	// Define styles, falling back to plain when NO_COLOR is set.
	headerStyle := lipgloss.NewStyle().Bold(true)
	fileStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	if noColor {
		headerStyle = lipgloss.NewStyle()
		fileStyle = lipgloss.NewStyle()
		errorStyle = lipgloss.NewStyle()
		warningStyle = lipgloss.NewStyle()
		noteStyle = lipgloss.NewStyle()
		dimStyle = lipgloss.NewStyle()
	}

	var b strings.Builder
	separator := "──────────────────────────────────"

	// Header.
	b.WriteString("\n")
	b.WriteString("  " + headerStyle.Render("Gavel Analysis") + "\n")
	b.WriteString("  " + dimStyle.Render(separator) + "\n")

	// Count unique files.
	fileSet := make(map[string]struct{})
	for _, r := range results {
		uri := prettyResultURI(r)
		if uri != "" {
			fileSet[uri] = struct{}{}
		}
	}

	fmt.Fprintf(&b, "  Decision: %s  |  %d findings  |  %d files\n", decision, len(results), len(fileSet))
	if persona != "" {
		fmt.Fprintf(&b, "  Persona: %s\n", persona)
	}
	b.WriteString("\n")

	if len(results) == 0 {
		b.WriteString("  No findings detected.\n\n")
	} else {
		// Group findings by file.
		fileResults := make(map[string][]sarif.Result)
		for _, r := range results {
			uri := prettyResultURI(r)
			fileResults[uri] = append(fileResults[uri], r)
		}

		// Sort file paths alphabetically.
		fileOrder := make([]string, 0, len(fileResults))
		for f := range fileResults {
			fileOrder = append(fileOrder, f)
		}
		sort.Strings(fileOrder)

		for _, file := range fileOrder {
			b.WriteString("  " + fileStyle.Render(file) + "\n")

			// Sort findings by start line within this file.
			fr := fileResults[file]
			sort.Slice(fr, func(i, j int) bool {
				li := prettyStartLine(fr[i])
				lj := prettyStartLine(fr[j])
				return li < lj
			})

			for _, r := range fr {
				line := prettyStartLine(r)

				var levelStr string
				switch r.Level {
				case "error":
					levelStr = errorStyle.Render("error  ")
				case "warning":
					levelStr = warningStyle.Render("warning")
				case "note":
					levelStr = noteStyle.Render("note   ")
				default:
					levelStr = r.Level
				}

				conf := ""
				if r.Properties != nil {
					if c, ok := r.Properties["gavel/confidence"].(float64); ok {
						conf = dimStyle.Render(fmt.Sprintf("(%.2f)", c))
					}
				}

				fmt.Fprintf(&b, "    %-6s %s  %-7s  %-30s %s\n",
					fmt.Sprintf("%d:1", line), levelStr, r.RuleID, r.Message.Text, conf)
			}
			b.WriteString("\n")
		}
	}

	// Summary footer.
	b.WriteString("  " + dimStyle.Render(separator) + "\n")

	errorCount := 0
	warningCount := 0
	noteCount := 0
	for _, r := range results {
		switch r.Level {
		case "error":
			errorCount++
		case "warning":
			warningCount++
		case "note":
			noteCount++
		}
	}

	var parts []string
	if errorCount > 0 {
		word := "errors"
		if errorCount == 1 {
			word = "error"
		}
		parts = append(parts, errorStyle.Render(fmt.Sprintf("%d %s", errorCount, word)))
	}
	if warningCount > 0 {
		word := "warnings"
		if warningCount == 1 {
			word = "warning"
		}
		parts = append(parts, warningStyle.Render(fmt.Sprintf("%d %s", warningCount, word)))
	}
	if noteCount > 0 {
		word := "notes"
		if noteCount == 1 {
			word = "note"
		}
		parts = append(parts, noteStyle.Render(fmt.Sprintf("%d %s", noteCount, word)))
	}

	if len(parts) > 0 {
		b.WriteString("  " + strings.Join(parts, ", ") + "\n")
	}
	b.WriteString("\n")

	return []byte(b.String()), nil
}

// prettyResultURI extracts the file URI from the first location of a SARIF result.
func prettyResultURI(r sarif.Result) string {
	if len(r.Locations) > 0 {
		return r.Locations[0].PhysicalLocation.ArtifactLocation.URI
	}
	return ""
}

// prettyStartLine extracts the start line from the first location of a SARIF result.
func prettyStartLine(r sarif.Result) int {
	if len(r.Locations) > 0 {
		return r.Locations[0].PhysicalLocation.Region.StartLine
	}
	return 0
}
