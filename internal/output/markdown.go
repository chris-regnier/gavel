package output

import "errors"

// MarkdownFormatter renders analysis output as GitHub-Flavored Markdown.
type MarkdownFormatter struct{}

// Format produces Markdown output.
// TODO: implement GFM rendering with collapsible findings.
func (f *MarkdownFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	return nil, errors.New("markdown formatter not yet implemented")
}
