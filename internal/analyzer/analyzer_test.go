package analyzer

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
	gavelcontext "github.com/chris-regnier/gavel/internal/context"
	"github.com/chris-regnier/gavel/internal/input"
)

type mockBAMLClient struct {
	findings []Finding
	err      error
}

func (m *mockBAMLClient) AnalyzeCode(ctx context.Context, code string, policies string, additionalContext string) ([]Finding, error) {
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

	results, err := a.Analyze(context.Background(), artifacts, policies, nil)
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

	results, err := a.Analyze(context.Background(), []input.Artifact{{Content: "code"}}, policies, nil)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil results for all-disabled policies, got %v", results)
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

func TestAnalyzer_WithAdditionalContext(t *testing.T) {
	// Create temp directory with context files
	tmpDir := t.TempDir()
	readmePath := tmpDir + "/README.md"
	if err := os.WriteFile(readmePath, []byte("# Test README"), 0644); err != nil {
		t.Fatal(err)
	}
	
	var capturedContext string
	
	mockCapture := &mockBAMLClientCapture{
		capturedContext: &capturedContext,
		findings:        []Finding{},
	}
	
	a := NewAnalyzer(mockCapture)
	
	artifacts := []input.Artifact{
		{Path: "main.go", Content: "package main", Kind: input.KindFile},
	}
	
	policies := map[string]config.Policy{
		"test-rule": {
			Description: "Test",
			Severity:    "warning",
			Instruction: "Test instruction",
			Enabled:     true,
			AdditionalContexts: []config.ContextSelector{
				{Pattern: "*.md"},
			},
		},
	}
	
	contextLoader := gavelcontext.NewLoader(tmpDir)
	_, err := a.Analyze(context.Background(), artifacts, policies, contextLoader)
	if err != nil {
		t.Fatal(err)
	}
	
	if capturedContext == "" {
		t.Error("expected context to be passed, got empty string")
	}
	
	if !strings.Contains(capturedContext, "README.md") {
		t.Errorf("expected context to contain README.md, got: %s", capturedContext)
	}
	
	if !strings.Contains(capturedContext, "Test README") {
		t.Errorf("expected context to contain README content, got: %s", capturedContext)
	}
}

// mockBAMLClientCapture is a mock that captures the additionalContext parameter
type mockBAMLClientCapture struct {
	capturedContext *string
	findings        []Finding
	err             error
}

func (m *mockBAMLClientCapture) AnalyzeCode(ctx context.Context, code string, policies string, additionalContext string) ([]Finding, error) {
	*m.capturedContext = additionalContext
	return m.findings, m.err
}
