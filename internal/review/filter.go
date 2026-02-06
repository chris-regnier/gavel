package review

import "github.com/chris-regnier/gavel/internal/sarif"

// getFilteredFindings returns findings filtered by current filter setting
func (m *ReviewModel) getFilteredFindings() []sarif.Result {
	switch m.filter {
	case FilterErrors:
		return m.filterByLevel([]string{"error"})
	case FilterWarnings:
		return m.filterByLevel([]string{"error", "warning"})
	case FilterAll:
		return m.findings
	default:
		return m.findings
	}
}

// filterByLevel returns findings matching the specified severity levels
func (m *ReviewModel) filterByLevel(levels []string) []sarif.Result {
	var filtered []sarif.Result

	for _, finding := range m.findings {
		for _, level := range levels {
			if finding.Level == level {
				filtered = append(filtered, finding)
				break
			}
		}
	}

	return filtered
}

// getFilteredFiles returns files grouped by findings, filtered by current filter
func (m *ReviewModel) getFilteredFiles() map[string][]sarif.Result {
	filtered := make(map[string][]sarif.Result)

	for filePath, findings := range m.files {
		var filteredFindings []sarif.Result

		for _, finding := range findings {
			// Apply filter to each finding
			switch m.filter {
			case FilterErrors:
				if finding.Level == "error" {
					filteredFindings = append(filteredFindings, finding)
				}
			case FilterWarnings:
				if finding.Level == "error" || finding.Level == "warning" {
					filteredFindings = append(filteredFindings, finding)
				}
			case FilterAll:
				filteredFindings = append(filteredFindings, finding)
			}
		}

		// Only include file if it has findings after filtering
		if len(filteredFindings) > 0 {
			filtered[filePath] = filteredFindings
		}
	}

	return filtered
}
