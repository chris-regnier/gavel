package review

import (
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestRenderFilesPane(t *testing.T) {
	// Create test model with multiple files
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID: "test-rule-1",
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
						RuleID: "test-rule-2",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "file1.go"},
									Region:           sarif.Region{StartLine: 20},
								},
							},
						},
					},
					{
						RuleID: "test-rule-3",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "file2.go"},
									Region:           sarif.Region{StartLine: 5},
								},
							},
						},
					},
				},
			},
		},
	}

	model := NewReviewModel(log)
	model.activePane = PaneFiles
	model.currentFile = 0

	output := model.renderFilesPane(80, 20)

	// Should contain file names
	if !strings.Contains(output, "file1.go") {
		t.Error("Expected output to contain 'file1.go'")
	}
	if !strings.Contains(output, "file2.go") {
		t.Error("Expected output to contain 'file2.go'")
	}

	// Should show finding counts
	if !strings.Contains(output, "2") {
		t.Error("Expected output to show count of 2 findings for file1.go")
	}
	if !strings.Contains(output, "1") {
		t.Error("Expected output to show count of 1 finding for file2.go")
	}
}

func TestRenderFilesPaneSelection(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID: "test-rule",
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
						RuleID: "test-rule",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "file2.go"},
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
	model.activePane = PaneFiles
	model.currentFile = 1

	output := model.renderFilesPane(80, 20)

	// Output should indicate selection (implementation will determine exact format)
	if len(output) == 0 {
		t.Error("Expected non-empty output")
	}
}

func TestGetFileList(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID: "test-rule",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "b.go"},
									Region:           sarif.Region{StartLine: 10},
								},
							},
						},
					},
					{
						RuleID: "test-rule",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "a.go"},
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
	files := model.getFileList()

	// Should return sorted file list
	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	// Should be sorted alphabetically
	if files[0] != "a.go" {
		t.Errorf("Expected first file to be 'a.go', got '%s'", files[0])
	}
	if files[1] != "b.go" {
		t.Errorf("Expected second file to be 'b.go', got '%s'", files[1])
	}
}
