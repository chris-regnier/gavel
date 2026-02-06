package review

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestGetFilteredFindings_All(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{RuleID: "rule1", Level: "error"},
					{RuleID: "rule2", Level: "warning"},
					{RuleID: "rule3", Level: "note"},
				},
			},
		},
	}

	model := NewReviewModel(log)
	model.filter = FilterAll

	filtered := model.getFilteredFindings()

	if len(filtered) != 3 {
		t.Errorf("Expected 3 findings with FilterAll, got %d", len(filtered))
	}
}

func TestGetFilteredFindings_ErrorsOnly(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{RuleID: "rule1", Level: "error"},
					{RuleID: "rule2", Level: "warning"},
					{RuleID: "rule3", Level: "error"},
					{RuleID: "rule4", Level: "note"},
				},
			},
		},
	}

	model := NewReviewModel(log)
	model.filter = FilterErrors

	filtered := model.getFilteredFindings()

	if len(filtered) != 2 {
		t.Errorf("Expected 2 error findings, got %d", len(filtered))
	}

	// Verify all are errors
	for _, finding := range filtered {
		if finding.Level != "error" {
			t.Errorf("Expected all findings to be errors, got %s", finding.Level)
		}
	}
}

func TestGetFilteredFindings_WarningsAndAbove(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{RuleID: "rule1", Level: "error"},
					{RuleID: "rule2", Level: "warning"},
					{RuleID: "rule3", Level: "note"},
				},
			},
		},
	}

	model := NewReviewModel(log)
	model.filter = FilterWarnings

	filtered := model.getFilteredFindings()

	if len(filtered) != 2 {
		t.Errorf("Expected 2 findings (error + warning), got %d", len(filtered))
	}

	// Verify no notes
	for _, finding := range filtered {
		if finding.Level == "note" {
			t.Error("Expected no 'note' level findings in warnings filter")
		}
	}
}

func TestGetFilteredFiles(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID: "rule1",
						Level:  "error",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "file1.go"},
									Region:           sarif.Region{StartLine: 10},
								},
							},
						},
					},
					{
						RuleID: "rule2",
						Level:  "warning",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "file2.go"},
									Region:           sarif.Region{StartLine: 20},
								},
							},
						},
					},
					{
						RuleID: "rule3",
						Level:  "note",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "file3.go"},
									Region:           sarif.Region{StartLine: 30},
								},
							},
						},
					},
				},
			},
		},
	}

	model := NewReviewModel(log)

	// Test FilterAll
	model.filter = FilterAll
	files := model.getFilteredFiles()
	if len(files) != 3 {
		t.Errorf("Expected 3 files with FilterAll, got %d", len(files))
	}

	// Test FilterErrors
	model.filter = FilterErrors
	files = model.getFilteredFiles()
	if len(files) != 1 {
		t.Errorf("Expected 1 file with FilterErrors, got %d", len(files))
	}
	if files["file1.go"] == nil {
		t.Error("Expected file1.go to be present in FilterErrors")
	}

	// Test FilterWarnings
	model.filter = FilterWarnings
	files = model.getFilteredFiles()
	if len(files) != 2 {
		t.Errorf("Expected 2 files with FilterWarnings, got %d", len(files))
	}
}
