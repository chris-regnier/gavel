package review

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Code pane styles
	codePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1)

	lineNumberStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Width(4).
			Align(lipgloss.Right)

	highlightedLineStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236"))

	codeHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))
)

const (
	contextLines = 5 // Lines to show before and after the finding
)

// renderCodePane renders the code view with syntax highlighting
func (m ReviewModel) renderCodePane(width, height int) string {
	var b strings.Builder

	// Header
	b.WriteString(codeHeaderStyle.Render("Code"))
	b.WriteString("\n\n")

	if len(m.findings) == 0 {
		b.WriteString("No findings to display")
	} else if m.currentFinding >= len(m.findings) {
		b.WriteString("Invalid finding index")
	} else {
		finding := m.findings[m.currentFinding]

		if len(finding.Locations) == 0 {
			b.WriteString("No location information")
		} else {
			location := finding.Locations[0].PhysicalLocation
			filePath := location.ArtifactLocation.URI
			line := location.Region.StartLine

			// Show file and line info
			fileInfo := fmt.Sprintf("%s:%d", filepath.Base(filePath), line)
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("241")).
				Render(fileInfo))
			b.WriteString("\n\n")

			// Read and display code with context
			code := m.readCodeWithContext(filePath, line)
			b.WriteString(code)
		}
	}

	// Apply border based on active pane
	paneStyle := codePaneStyle
	if m.activePane == PaneCode {
		paneStyle = codePaneStyle.Copy().BorderForeground(lipgloss.Color("170"))
	}

	return paneStyle.
		Width(width - 2).
		Height(height - 2).
		Render(b.String())
}

// readCodeWithContext reads a file and returns lines around the target line with syntax highlighting
func (m *ReviewModel) readCodeWithContext(filePath string, targetLine int) string {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err)
	}
	defer file.Close()

	// Determine lexer based on file extension
	lexer := lexers.Match(filePath)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	// Read all lines
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("Error scanning file: %v", err)
	}

	// Calculate range
	startLine := targetLine - contextLines
	if startLine < 1 {
		startLine = 1
	}
	endLine := targetLine + contextLines
	if endLine > len(lines) {
		endLine = len(lines)
	}

	var b strings.Builder

	// Format each line
	for i := startLine; i <= endLine; i++ {
		lineNum := lineNumberStyle.Render(fmt.Sprintf("%d", i))
		lineContent := ""

		if i <= len(lines) {
			// Apply syntax highlighting
			highlighted, err := highlightLine(lines[i-1], lexer)
			if err != nil {
				lineContent = lines[i-1]
			} else {
				lineContent = highlighted
			}
		}

		// Highlight the target line
		if i == targetLine {
			lineContent = highlightedLineStyle.Render(lineContent)
			lineNum = highlightedLineStyle.Render(lineNum)
		}

		b.WriteString(fmt.Sprintf("%s │ %s\n", lineNum, lineContent))
	}

	return b.String()
}

// highlightLine applies syntax highlighting to a single line of code
func highlightLine(line string, lexer chroma.Lexer) (string, error) {
	iterator, err := lexer.Tokenise(nil, line)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	formatter := formatters.TTY16m
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}

	err = formatter.Format(&b, style, iterator)
	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(b.String(), "\n"), nil
}
