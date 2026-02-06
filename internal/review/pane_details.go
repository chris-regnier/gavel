package review

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Details pane styles
	detailsPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63")).
				Padding(1)

	detailsHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("170"))

	severityErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)

	severityWarningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)

	reviewStatusAcceptedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("46")).
					Bold(true)

	reviewStatusRejectedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("196")).
					Bold(true)

	confidenceStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75"))
)

// renderDetailsPane renders the finding details with markdown formatting
func (m ReviewModel) renderDetailsPane(width, height int) string {
	var b strings.Builder

	// Header
	b.WriteString(detailsHeaderStyle.Render("Details"))
	b.WriteString("\n\n")

	if len(m.findings) == 0 {
		b.WriteString("No findings to display")
	} else if m.currentFinding >= len(m.findings) {
		b.WriteString("Invalid finding index")
	} else {
		finding := m.findings[m.currentFinding]

		// Rule ID and severity
		severityStyle := lipgloss.NewStyle()
		switch finding.Level {
		case "error":
			severityStyle = severityErrorStyle
		case "warning":
			severityStyle = severityWarningStyle
		}

		b.WriteString(fmt.Sprintf("**Rule:** %s  ", finding.RuleID))
		b.WriteString(severityStyle.Render(strings.ToUpper(finding.Level)))
		b.WriteString("\n\n")

		// Review status
		findingID := m.getFindingID(m.currentFinding)
		if m.accepted[findingID] {
			b.WriteString(reviewStatusAcceptedStyle.Render("✓ Accepted"))
			b.WriteString("\n\n")
		} else if m.rejected[findingID] {
			b.WriteString(reviewStatusRejectedStyle.Render("✗ Rejected"))
			b.WriteString("\n\n")
		}

		// Message
		if finding.Message.Text != "" {
			b.WriteString("**Message:**\n")
			b.WriteString(finding.Message.Text)
			b.WriteString("\n\n")
		}

		// Gavel-specific properties
		if finding.Properties != nil {
			if rec, ok := finding.Properties["gavel/recommendation"].(string); ok && rec != "" {
				b.WriteString("**Recommendation:**\n")
				b.WriteString(rec)
				b.WriteString("\n\n")
			}

			if expl, ok := finding.Properties["gavel/explanation"].(string); ok && expl != "" {
				b.WriteString("**Explanation:**\n")
				b.WriteString(expl)
				b.WriteString("\n\n")
			}

			if conf, ok := finding.Properties["gavel/confidence"].(float64); ok {
				confPercent := int(conf * 100)
				b.WriteString("**Confidence:** ")
				b.WriteString(confidenceStyle.Render(fmt.Sprintf("%d%%", confPercent)))
				b.WriteString("\n")
			}
		}

		// Location info
		if len(finding.Locations) > 0 {
			loc := finding.Locations[0].PhysicalLocation
			b.WriteString("\n**Location:**\n")
			b.WriteString(fmt.Sprintf("%s:%d", loc.ArtifactLocation.URI, loc.Region.StartLine))
		}
	}

	// Render markdown content with glamour
	content := b.String()
	rendered, err := renderMarkdown(content, width-4)
	if err != nil {
		// Fallback to plain text if markdown rendering fails
		rendered = content
	}

	// Apply border based on active pane
	paneStyle := detailsPaneStyle
	if m.activePane == PaneDetails {
		paneStyle = detailsPaneStyle.Copy().BorderForeground(lipgloss.Color("170"))
	}

	return paneStyle.
		Width(width - 2).
		Height(height - 2).
		Render(rendered)
}

// renderMarkdown renders markdown text using glamour
func renderMarkdown(text string, width int) (string, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", err
	}

	out, err := r.Render(text)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}
