package review

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Init implements tea.Model
func (m ReviewModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "n": // Next finding
			filtered := m.getFilteredFindings()
			if len(filtered) > 0 {
				m.currentFinding = (m.currentFinding + 1) % len(filtered)
			}

		case "p": // Previous finding
			filtered := m.getFilteredFindings()
			if len(filtered) > 0 {
				m.currentFinding--
				if m.currentFinding < 0 {
					m.currentFinding = len(filtered) - 1
				}
			}

		case "a": // Accept finding
			if len(m.findings) > 0 {
				findingID := m.getFindingID(m.currentFinding)
				m.accepted[findingID] = true
				delete(m.rejected, findingID)
			}

		case "r": // Reject finding
			if len(m.findings) > 0 {
				findingID := m.getFindingID(m.currentFinding)
				m.rejected[findingID] = true
				delete(m.accepted, findingID)
			}

		case "tab": // Switch panes
			m.activePane = (m.activePane + 1) % 3

		case "e": // Filter: errors only
			m.filter = FilterErrors
			m.currentFinding = 0 // Reset to first finding

		case "w": // Filter: warnings+
			m.filter = FilterWarnings
			m.currentFinding = 0 // Reset to first finding

		case "f": // Filter: all
			m.filter = FilterAll
			m.currentFinding = 0 // Reset to first finding
		}
	}

	return m, nil
}

// getFindingID generates a unique ID for a finding
func (m *ReviewModel) getFindingID(idx int) string {
	if idx < 0 || idx >= len(m.findings) {
		return ""
	}

	result := m.findings[idx]
	filePath := ""
	line := 0

	if len(result.Locations) > 0 {
		filePath = result.Locations[0].PhysicalLocation.ArtifactLocation.URI
		line = result.Locations[0].PhysicalLocation.Region.StartLine
	}

	return fmt.Sprintf("%s:%s:%d", result.RuleID, filePath, line)
}
