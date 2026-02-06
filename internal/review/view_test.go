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
