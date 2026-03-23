package analyzer

import (
	"context"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
)

type mockBAMLClient struct {
	findings   []Finding
	err        error
	lastCode   string // captures the code arg from the most recent call
}

func (m *mockBAMLClient) AnalyzeCode(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]Finding, error) {
	m.lastCode = code
	return m.findings, m.err
}

func TestAnalyzer_Analyze(t *testing.T) {
	mock := &mockBAMLClient{
		findings: []Finding{
			{
				RuleID:         "error-handling",
				Level:          "warning",
				Message:        "Function Foo ignores error from Bar()",
				FilePath:       "pkg/foo.go",
				StartLine:      10,
				EndLine:        12,
				Recommendation: "Return the error",
				Explanation:    "Bar() returns an error that is discarded",
				Confidence:     0.85,
			},
		},
	}

	a := NewAnalyzer(mock)
	artifacts := []input.Artifact{
		{Path: "pkg/foo.go", Content: "package pkg\n\nfunc Foo() { Bar() }\n", Kind: input.KindFile},
	}
	policies := map[string]config.Policy{
		"error-handling": {
			Description: "Handle errors",
			Severity:    "warning",
			Instruction: "Check error handling",
			Enabled:     true,
		},
	}

	results, err := a.Analyze(context.Background(), artifacts, policies, "test persona prompt")
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.RuleID != "error-handling" {
		t.Errorf("expected ruleId 'error-handling', got %q", r.RuleID)
	}
	if r.Properties["gavel/confidence"] != 0.85 {
		t.Errorf("expected confidence 0.85, got %v", r.Properties["gavel/confidence"])
	}
}

func TestAnalyzer_SkipsDisabledPolicies(t *testing.T) {
	mock := &mockBAMLClient{findings: nil}

	a := NewAnalyzer(mock)
	policies := map[string]config.Policy{
		"disabled-rule": {
			Instruction: "This should not run",
			Enabled:     false,
		},
	}

	results, err := a.Analyze(context.Background(), []input.Artifact{{Content: "code"}}, policies, "test persona prompt")
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil results for all-disabled policies, got %v", results)
	}
}

func TestAnalyzer_PrependsFilePathToCode(t *testing.T) {
	mock := &mockBAMLClient{findings: nil}
	a := NewAnalyzer(mock)

	artifacts := []input.Artifact{
		{Path: "pkg/foo.go", Content: "package pkg\n", Kind: input.KindFile},
	}
	policies := map[string]config.Policy{
		"rule": {Severity: "warning", Instruction: "check", Enabled: true},
	}

	_, err := a.Analyze(context.Background(), artifacts, policies, "persona")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(mock.lastCode, "// File: pkg/foo.go\n") {
		t.Errorf("expected code to start with file path header, got %q", mock.lastCode[:min(len(mock.lastCode), 60)])
	}
	if !strings.Contains(mock.lastCode, "package pkg") {
		t.Error("expected original content to be preserved after the header")
	}
}

func TestAnalyzer_EmptyPathSkipsHeader(t *testing.T) {
	mock := &mockBAMLClient{findings: nil}
	a := NewAnalyzer(mock)

	artifacts := []input.Artifact{
		{Path: "", Content: "package pkg\n", Kind: input.KindFile},
	}
	policies := map[string]config.Policy{
		"rule": {Severity: "warning", Instruction: "check", Enabled: true},
	}

	_, err := a.Analyze(context.Background(), artifacts, policies, "persona")
	if err != nil {
		t.Fatal(err)
	}

	if strings.HasPrefix(mock.lastCode, "// File:") {
		t.Error("expected no file path header for empty path")
	}
}

func TestFormatPolicies(t *testing.T) {
	policies := map[string]config.Policy{
		"rule-a": {Severity: "warning", Instruction: "Do A", Enabled: true},
		"rule-b": {Severity: "error", Instruction: "Do B", Enabled: false},
	}

	text := FormatPolicies(policies)
	if !strings.Contains(text, "rule-a") {
		t.Error("expected rule-a in output")
	}
	if strings.Contains(text, "rule-b") {
		t.Error("did not expect disabled rule-b in output")
	}
}
