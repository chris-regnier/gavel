package review

import (
	"github.com/chris-regnier/gavel/internal/sarif"
)

// Pane represents which pane is currently active
type Pane int

const (
	PaneFiles Pane = iota
	PaneCode
	PaneDetails
)

// Filter represents the severity filter
type Filter int

const (
	FilterAll Filter = iota
	FilterErrors
	FilterWarnings
)

// ReviewModel is the bubbletea model for the review TUI
type ReviewModel struct {
	sarif    *sarif.Log
	findings []sarif.Result
	files    map[string][]sarif.Result

	currentFile    int
	currentFinding int
	activePane     Pane
	filter         Filter

	accepted map[string]bool
	rejected map[string]bool
	comments map[string]string

	width  int
	height int
}

// NewReviewModel creates a new ReviewModel from a SARIF log
func NewReviewModel(log *sarif.Log) *ReviewModel {
	m := &ReviewModel{
		sarif:      log,
		findings:   []sarif.Result{},
		files:      make(map[string][]sarif.Result),
		activePane: PaneFiles,
		filter:     FilterAll,
		accepted:   make(map[string]bool),
		rejected:   make(map[string]bool),
		comments:   make(map[string]string),
	}

	// Extract findings and group by file
	if len(log.Runs) > 0 {
		for _, result := range log.Runs[0].Results {
			m.findings = append(m.findings, result)

			// Only group by file if location information is complete
			if len(result.Locations) > 0 &&
				result.Locations[0].PhysicalLocation.ArtifactLocation.URI != "" {
				filePath := result.Locations[0].PhysicalLocation.ArtifactLocation.URI
				m.files[filePath] = append(m.files[filePath], result)
			}
		}
	}

	return m
}
