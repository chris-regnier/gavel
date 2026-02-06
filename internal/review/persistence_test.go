package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadReviewState(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "test-review.json")

	model := &ReviewModel{
		accepted: map[string]bool{
			"rule1:file.go:10": true,
		},
		rejected: map[string]bool{
			"rule2:file.go:20": true,
		},
		comments: map[string]string{
			"rule1:file.go:10": "Looks good",
			"rule2:file.go:20": "False positive",
		},
	}

	// Save
	if err := SaveReviewState(model, "test-sarif-id", stateFile); err != nil {
		t.Fatalf("SaveReviewState failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Fatal("State file was not created")
	}

	// Load
	loaded, err := LoadReviewState(stateFile)
	if err != nil {
		t.Fatalf("LoadReviewState failed: %v", err)
	}

	if loaded.SarifID != "test-sarif-id" {
		t.Errorf("Expected SarifID='test-sarif-id', got '%s'", loaded.SarifID)
	}

	if len(loaded.Findings) != 2 {
		t.Errorf("Expected 2 findings, got %d", len(loaded.Findings))
	}

	// Check accepted finding
	finding1 := loaded.Findings["rule1:file.go:10"]
	if finding1.Status != "accepted" {
		t.Errorf("Expected status='accepted', got '%s'", finding1.Status)
	}
	if finding1.Comment != "Looks good" {
		t.Errorf("Expected comment='Looks good', got '%s'", finding1.Comment)
	}

	// Check rejected finding
	finding2 := loaded.Findings["rule2:file.go:20"]
	if finding2.Status != "rejected" {
		t.Errorf("Expected status='rejected', got '%s'", finding2.Status)
	}
}
