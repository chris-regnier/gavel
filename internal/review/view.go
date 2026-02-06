package review

import (
	"fmt"
)

// View implements tea.Model
func (m ReviewModel) View() string {
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
