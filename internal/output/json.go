package output

import (
	"encoding/json"
	"fmt"
)

// JSONFormatter renders analysis output as indented JSON of the verdict.
type JSONFormatter struct{}

// Format serializes the verdict as pretty-printed JSON.
func (f *JSONFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	if result == nil || result.Verdict == nil {
		return nil, fmt.Errorf("json formatter: verdict is required")
	}
	return json.MarshalIndent(result.Verdict, "", "  ")
}
