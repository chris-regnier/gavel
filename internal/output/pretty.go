package output

import "errors"

// PrettyFormatter renders analysis output as colored, human-readable
// terminal output suitable for interactive use.
type PrettyFormatter struct{}

// Format produces pretty terminal output.
// TODO: implement lipgloss-colored output grouped by file.
func (f *PrettyFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	return nil, errors.New("pretty formatter not yet implemented")
}
