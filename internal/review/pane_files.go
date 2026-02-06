package review

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// File pane styles
	filePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1)

	fileItemStyle = lipgloss.NewStyle().
			PaddingLeft(2)

	selectedFileStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("170")).
				Bold(true).
				PaddingLeft(1)

	fileCountStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	activePaneStyle = lipgloss.NewStyle().
			BorderForeground(lipgloss.Color("170"))
)

// renderFilesPane renders the file tree pane with findings count
func (m ReviewModel) renderFilesPane(width, height int) string {
	var b strings.Builder

	files := m.getFileList()

	// Header
	b.WriteString(lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		Render("Files"))
	b.WriteString("\n\n")

	// File list
	for i, file := range files {
		count := len(m.files[file])
		indicator := "  "
		style := fileItemStyle

		if i == m.currentFile {
			indicator = "▸ "
			style = selectedFileStyle
		}

		line := fmt.Sprintf("%s%s %s",
			indicator,
			file,
			fileCountStyle.Render(fmt.Sprintf("(%d)", count)))

		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	// Apply border based on active pane
	paneStyle := filePaneStyle
	if m.activePane == PaneFiles {
		paneStyle = filePaneStyle.Copy().BorderForeground(lipgloss.Color("170"))
	}

	return paneStyle.
		Width(width - 2).
		Height(height - 2).
		Render(b.String())
}

// getFileList returns a sorted list of file paths
func (m *ReviewModel) getFileList() []string {
	files := make([]string, 0, len(m.files))
	for file := range m.files {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}
