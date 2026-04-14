package analyzer

import (
	"context"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
)

type mockBAMLClient struct {
	findings []Finding
	err      error
	lastCode string // captures the code arg from the most recent call
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

func TestAnalyzer_EmitsFixWhenReplacementPresent(t *testing.T) {
	mock := &mockBAMLClient{
		findings: []Finding{
			{
				RuleID:             "hardcoded-credentials",
				Level:              "error",
				Message:            "Hardcoded password",
				FilePath:           "config.go",
				StartLine:          42,
				EndLine:            42,
				Recommendation:     "Use an environment variable",
				Confidence:         0.95,
				FixReplacementText: `password := os.Getenv("DB_PASSWORD")`,
			},
		},
	}

	a := NewAnalyzer(mock)
	artifacts := []input.Artifact{
		{Path: "config.go", Content: "package main\n\n" + strings.Repeat("line\n", 50), Kind: input.KindFile},
	}
	policies := map[string]config.Policy{
		"hardcoded-credentials": {
			Severity:    "error",
			Instruction: "No hardcoded credentials",
			Enabled:     true,
		},
	}

	results, err := a.Analyze(context.Background(), artifacts, policies, "test persona")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if len(r.Fixes) != 1 {
		t.Fatalf("expected 1 fix on result, got %d", len(r.Fixes))
	}
	fix := r.Fixes[0]
	if fix.Description.Text != "Use an environment variable" {
		t.Errorf("fix description should mirror recommendation, got %q", fix.Description.Text)
	}
	if len(fix.ArtifactChanges) != 1 {
		t.Fatalf("expected 1 artifactChange, got %d", len(fix.ArtifactChanges))
	}
	ac := fix.ArtifactChanges[0]
	if ac.ArtifactLocation.URI != "config.go" {
		t.Errorf("expected artifactLocation URI 'config.go', got %q", ac.ArtifactLocation.URI)
	}
	if len(ac.Replacements) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(ac.Replacements))
	}
	rep := ac.Replacements[0]
	if rep.DeletedRegion.StartLine != 42 || rep.DeletedRegion.EndLine != 42 {
		t.Errorf("deletedRegion should span finding region, got start=%d end=%d",
			rep.DeletedRegion.StartLine, rep.DeletedRegion.EndLine)
	}
	if rep.InsertedContent == nil || rep.InsertedContent.Text != `password := os.Getenv("DB_PASSWORD")` {
		t.Errorf("insertedContent not set correctly: %+v", rep.InsertedContent)
	}
}

func TestAnalyzer_NoFixWhenReplacementEmpty(t *testing.T) {
	mock := &mockBAMLClient{
		findings: []Finding{
			{
				RuleID:             "structural-issue",
				Level:              "note",
				Message:            "Consider restructuring this module",
				FilePath:           "pkg/foo.go",
				StartLine:          5,
				EndLine:            50,
				Recommendation:     "Extract smaller functions",
				Confidence:         0.6,
				FixReplacementText: "", // no machine-applicable fix
			},
		},
	}

	a := NewAnalyzer(mock)
	artifacts := []input.Artifact{
		{Path: "pkg/foo.go", Content: strings.Repeat("line\n", 60), Kind: input.KindFile},
	}
	policies := map[string]config.Policy{
		"structural-issue": {Severity: "note", Instruction: "Design feedback", Enabled: true},
	}

	results, err := a.Analyze(context.Background(), artifacts, policies, "test persona")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Fixes) != 0 {
		t.Errorf("expected no fixes when FixReplacementText is empty, got %d", len(results[0].Fixes))
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
