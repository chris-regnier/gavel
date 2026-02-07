package review

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View implements tea.Model
func (m ReviewModel) View() string {
	// If no size set, return basic view
	if m.width == 0 || m.height == 0 {
		fileCount := len(m.files)
		findingCount := len(m.findings)

		fileText := "files"
		if fileCount == 1 {
			fileText = "file"
		}

		findingText := "findings"
		if findingCount == 1 {
			findingText = "finding"
		}

		return fmt.Sprintf("PR Review: %d %s, %d %s\n\nPress q to quit",
			fileCount, fileText, findingCount, findingText)
	}

	// Calculate dimensions for three-pane layout
	// Files pane: 25% width, full height
	// Code pane: 45% width, full height
	// Details pane: 30% width, full height

	filesPaneWidth := m.width / 4
	detailsPaneWidth := (m.width * 3) / 10
	codePaneWidth := m.width - filesPaneWidth - detailsPaneWidth

	paneHeight := m.height - 3 // Leave room for status bar

	// Render each pane
	filesPane := m.renderFilesPane(filesPaneWidth, paneHeight)
	codePane := m.renderCodePane(codePaneWidth, paneHeight)
	detailsPane := m.renderDetailsPane(detailsPaneWidth, paneHeight)

	// Compose horizontally
	panes := lipgloss.JoinHorizontal(
		lipgloss.Top,
		filesPane,
		codePane,
		detailsPane,
	)

	// Status bar
	statusBar := m.renderStatusBar()

	// Compose vertically
	return lipgloss.JoinVertical(
		lipgloss.Left,
		panes,
		statusBar,
	)
}

// renderStatusBar renders the bottom status bar with help text
func (m ReviewModel) renderStatusBar() string {
	var statusParts []string

	// Navigation
	statusParts = append(statusParts, "n/p: next/prev")

	// Review actions
	statusParts = append(statusParts, "a: accept")
	statusParts = append(statusParts, "r: reject")

	// Pane switching
	statusParts = append(statusParts, "tab: switch pane")

	// Filter
	statusParts = append(statusParts, "e/w/f: filter")

	// Quit
	statusParts = append(statusParts, "q: quit")

	status := strings.Join(statusParts, " │ ")

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Width(m.width).
		Render(status)
}
