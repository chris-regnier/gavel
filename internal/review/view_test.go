package review

import (
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestView_RendersBasicInfo(t *testing.T) {
	model := &ReviewModel{
		findings: []sarif.Result{
			{
				RuleID: "test-rule",
				Message: sarif.Message{
					Text: "Test finding",
				},
			},
		},
		files: map[string][]sarif.Result{
			"test.go": {
				{RuleID: "test-rule"},
			},
		},
	}

	view := model.View()

	// Check for file count
	if !strings.Contains(view, "1 file") {
		t.Error("View should contain file count")
	}

	// Check for finding count
	if !strings.Contains(view, "1 finding") {
		t.Error("View should contain finding count")
	}
}

func TestView_ThreePaneLayout(t *testing.T) {
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
									ArtifactLocation: sarif.ArtifactLocation{URI: "test.go"},
									Region:           sarif.Region{StartLine: 10},
								},
							},
						},
					},
				},
			},
		},
	}

	model := NewReviewModel(log)
	model.width = 120
	model.height = 40

	view := model.View()

	// View should compose all three panes
	if len(view) == 0 {
		t.Error("Expected non-empty view")
	}

	// Should contain pane content indicators
	// (exact content depends on implementation, checking for reasonable output)
	if len(view) < 100 {
		t.Error("View seems too short for a three-pane layout")
	}
}
