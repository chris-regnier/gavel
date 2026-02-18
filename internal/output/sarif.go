package output

import "errors"

// SARIFFormatter renders analysis output as a SARIF 2.1.0 JSON document.
type SARIFFormatter struct{}

// Format produces SARIF output.
// TODO: implement full SARIF serialization from result.SARIFLog.
func (f *SARIFFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	return nil, errors.New("sarif formatter not yet implemented")
}
