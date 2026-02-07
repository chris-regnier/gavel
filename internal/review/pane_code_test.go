package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestRenderCodePane(t *testing.T) {
	// Create a temporary Go file for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	testCode := `package main

import "fmt"

func main() {
	fmt.Println("Hello, world!")
}
`
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID: "test-rule",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: testFile},
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
	model.activePane = PaneCode
	model.currentFinding = 0

	output := model.renderCodePane(80, 20)

	// Should contain some code content
	if len(output) == 0 {
		t.Error("Expected non-empty code pane output")
	}

	// Should indicate it's the code pane
	if !strings.Contains(output, "Code") {
		t.Error("Expected output to contain 'Code' header")
	}
}

func TestRenderCodePaneNoFile(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID: "test-rule",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: "/nonexistent/file.go"},
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
	model.activePane = PaneCode
	model.currentFinding = 0

	output := model.renderCodePane(80, 20)

	// Should handle missing file gracefully
	if len(output) == 0 {
		t.Error("Expected non-empty output even for missing file")
	}
}

func TestRenderCodePaneWithContext(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	testCode := `package main

import "fmt"

func main() {
	fmt.Println("Line 6")
	fmt.Println("Line 7")
	fmt.Println("Line 8")  // Finding on this line
	fmt.Println("Line 9")
	fmt.Println("Line 10")
}
`
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID: "test-rule",
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{URI: testFile},
									Region:           sarif.Region{StartLine: 8},
								},
							},
						},
					},
				},
			},
		},
	}

	model := NewReviewModel(log)
	model.activePane = PaneCode
	model.currentFinding = 0

	output := model.renderCodePane(80, 30)

	// Should show context lines around the finding
	if !strings.Contains(output, "Line 8") {
		t.Error("Expected output to contain the line with the finding")
	}
}
