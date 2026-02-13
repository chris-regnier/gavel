package analyzer

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
)

func TestTieredAnalyzer_ASTRules_FunctionLength(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}
	ta := NewTieredAnalyzer(mock)

	// Build a Go file with a long function (60 lines)
	source := "package main\n\nfunc longFunc() {\n"
	for i := 0; i < 57; i++ {
		source += "\tx := 1\n"
	}
	source += "}\n"

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: source,
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	var instantResults []TieredResult
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierInstant {
			instantResults = append(instantResults, result)
		}
	}

	found := false
	for _, tr := range instantResults {
		for _, r := range tr.Results {
			if r.RuleID == "AST001" {
				found = true
				if ruleType, ok := r.Properties["gavel/rule-type"].(string); !ok || ruleType != "ast" {
					t.Errorf("expected rule-type 'ast', got %v", r.Properties["gavel/rule-type"])
				}
			}
		}
	}

	if !found {
		t.Error("expected to find AST001 function-length finding")
	}
}

func TestTieredAnalyzer_ASTRules_UnsupportedLanguage(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}
	ta := NewTieredAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "data.csv",
		Content: "col1,col2\nval1,val2",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierInstant {
			for _, r := range result.Results {
				if r.Properties != nil {
					if ruleType, ok := r.Properties["gavel/rule-type"].(string); ok && ruleType == "ast" {
						t.Error("unexpected AST finding for .csv file")
					}
				}
			}
		}
	}
}

func TestTieredAnalyzer_ASTRules_CustomConfig(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}

	customRules := []rules.Rule{{
		ID:         "AST001",
		Name:       "function-length",
		Type:       rules.RuleTypeAST,
		ASTCheck:   "function-length",
		ASTConfig:  map[string]interface{}{"max_lines": 3},
		Level:      "warning",
		Message:    "Function too long",
		Confidence: 1.0,
	}}

	ta := NewTieredAnalyzer(mock, WithInstantPatterns(customRules))

	source := `package main

func medium() {
	a := 1
	b := 2
	c := 3
	d := 4
	e := 5
}
`
	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: source,
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	var found bool
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierInstant {
			for _, r := range result.Results {
				if r.RuleID == "AST001" {
					found = true
				}
			}
		}
	}

	if !found {
		t.Error("expected AST001 finding with max_lines=3 threshold")
	}
}
