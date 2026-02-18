package output

import (
	"encoding/json"
	"fmt"
)

// JSONFormatter renders analysis output as indented JSON of the verdict.
type JSONFormatter struct{}

// Format serializes the verdict as pretty-printed JSON with a trailing newline
// for shell friendliness (e.g. piping to jq).
func (f *JSONFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	if result == nil || result.Verdict == nil {
		return nil, fmt.Errorf("json formatter: verdict is required")
	}
	data, err := json.MarshalIndent(result.Verdict, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
