package review

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestUpdate_NextFinding(t *testing.T) {
	model := &ReviewModel{
		findings:       make([]sarif.Result, 3), // 3 findings
		currentFinding: 0,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	updatedModel, _ := model.Update(msg)

	m := updatedModel.(ReviewModel)
	if m.currentFinding != 1 {
		t.Errorf("Expected currentFinding=1, got %d", m.currentFinding)
	}
}

func TestUpdate_PreviousFinding(t *testing.T) {
	model := &ReviewModel{
		findings:       make([]sarif.Result, 3),
		currentFinding: 1,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	updatedModel, _ := model.Update(msg)

	m := updatedModel.(ReviewModel)
	if m.currentFinding != 0 {
		t.Errorf("Expected currentFinding=0, got %d", m.currentFinding)
	}
}

func TestUpdate_AcceptFinding(t *testing.T) {
	model := &ReviewModel{
		findings: []sarif.Result{
			{
				RuleID: "test-rule",
				Locations: []sarif.Location{
					{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{
								URI: "test.go",
							},
							Region: sarif.Region{
								StartLine: 10,
							},
						},
					},
				},
			},
		},
		currentFinding: 0,
		accepted:       make(map[string]bool),
		rejected:       make(map[string]bool),
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updatedModel, _ := model.Update(msg)

	m := updatedModel.(ReviewModel)
	expectedID := "test-rule:test.go:10"
	if !m.accepted[expectedID] {
		t.Errorf("Expected finding %s to be marked as accepted", expectedID)
	}
}
