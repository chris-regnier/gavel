package review

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestNewReviewModel(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID: "test-rule",
						Level:  "error",
						Message: sarif.Message{
							Text: "Test finding",
						},
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{
										URI: "test.go",
									},
									Region: sarif.Region{
										StartLine: 10,
										EndLine:   12,
									},
								},
							},
						},
						Properties: map[string]interface{}{
							"gavel/confidence":     0.9,
							"gavel/explanation":    "Test explanation",
							"gavel/recommendation": "Test recommendation",
						},
					},
				},
			},
		},
	}

	model := NewReviewModel(log)

	if model.sarif == nil {
		t.Fatal("Expected sarif log to be set")
	}

	if len(model.findings) != 1 {
		t.Fatalf("Expected 1 finding, got %d", len(model.findings))
	}

	if len(model.files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(model.files))
	}

	if model.currentFile != 0 {
		t.Errorf("Expected currentFile=0, got %d", model.currentFile)
	}

	if model.currentFinding != 0 {
		t.Errorf("Expected currentFinding=0, got %d", model.currentFinding)
	}

	if model.activePane != PaneFiles {
		t.Errorf("Expected activePane=PaneFiles, got %v", model.activePane)
	}
}
