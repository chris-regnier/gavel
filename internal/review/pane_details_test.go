package review

import (
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestRenderDetailsPane(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID:  "test-rule",
						Level:   "error",
						Message: sarif.Message{Text: "Test finding message"},
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "test.go"},
									Region:           sarif.Region{StartLine: 10},
								},
							},
						},
						Properties: map[string]interface{}{
							"gavel/recommendation": "Fix this issue",
							"gavel/explanation":    "This is a test explanation",
							"gavel/confidence":     0.85,
						},
					},
				},
			},
		},
	}

	model := NewReviewModel(log)
	model.activePane = PaneDetails
	model.currentFinding = 0

	output := model.renderDetailsPane(80, 20)

	// Should contain finding details
	if !strings.Contains(output, "test-rule") {
		t.Error("Expected output to contain rule ID")
	}
	if !strings.Contains(strings.ToLower(output), "error") {
		t.Error("Expected output to contain level (case-insensitive)")
	}
	if !strings.Contains(output, "Test finding message") {
		t.Error("Expected output to contain message")
	}
}

func TestRenderDetailsPaneWithReview(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID:  "test-rule",
						Level:   "warning",
						Message: sarif.Message{Text: "Test finding"},
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
	model.activePane = PaneDetails
	model.currentFinding = 0

	// Mark as accepted
	findingID := model.getFindingID(0)
	model.accepted[findingID] = true

	output := model.renderDetailsPane(80, 20)

	// Should show review status
	if !strings.Contains(output, "Accepted") && !strings.Contains(output, "✓") {
		t.Error("Expected output to indicate accepted status")
	}
}

func TestRenderDetailsPaneEmpty(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{},
			},
		},
	}

	model := NewReviewModel(log)
	model.activePane = PaneDetails

	output := model.renderDetailsPane(80, 20)

	// Should handle no findings gracefully
	if len(output) == 0 {
		t.Error("Expected non-empty output even with no findings")
	}
}

func TestRenderDetailsPaneWithGavelProperties(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID:  "test-rule",
						Level:   "error",
						Message: sarif.Message{Text: "Test message"},
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "test.go"},
									Region:           sarif.Region{StartLine: 10},
								},
							},
						},
						Properties: map[string]interface{}{
							"gavel/recommendation": "Use better approach",
							"gavel/explanation":    "Detailed explanation here",
							"gavel/confidence":     0.92,
						},
					},
				},
			},
		},
	}

	model := NewReviewModel(log)
	model.activePane = PaneDetails
	model.currentFinding = 0

	output := model.renderDetailsPane(80, 20)

	// Should display gavel-specific properties
	if !strings.Contains(output, "Recommendation") {
		t.Error("Expected output to contain recommendation section")
	}
	if !strings.Contains(output, "Confidence") {
		t.Error("Expected output to contain confidence")
	}
}
