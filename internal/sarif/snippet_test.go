package sarif

import (
	"strings"
	"testing"
)

const sampleContent = `package main

import "fmt"

func main() {
	password := "secret123"
	fmt.Println(password)
	if err != nil {
	}
	return
}
`

func TestExtractSnippet_SingleLine(t *testing.T) {
	snip := ExtractSnippet(sampleContent, 6, 6)
	if snip == nil {
		t.Fatal("expected non-nil snippet")
	}
	if !strings.Contains(snip.Text, "password") {
		t.Errorf("expected snippet to contain 'password', got %q", snip.Text)
	}
}

func TestExtractSnippet_MultiLine(t *testing.T) {
	snip := ExtractSnippet(sampleContent, 6, 7)
	if snip == nil {
		t.Fatal("expected non-nil snippet")
	}
	if !strings.Contains(snip.Text, "password") || !strings.Contains(snip.Text, "Println") {
		t.Errorf("expected snippet to contain both lines, got %q", snip.Text)
	}
}

func TestExtractSnippet_EmptyContent(t *testing.T) {
	snip := ExtractSnippet("", 1, 1)
	if snip != nil {
		t.Error("expected nil snippet for empty content")
	}
}

func TestExtractSnippet_InvalidStartLine(t *testing.T) {
	snip := ExtractSnippet(sampleContent, 0, 1)
	if snip != nil {
		t.Error("expected nil snippet for startLine < 1")
	}
}

func TestExtractSnippet_StartLineBeyondContent(t *testing.T) {
	snip := ExtractSnippet(sampleContent, 999, 999)
	if snip != nil {
		t.Error("expected nil snippet for startLine beyond content")
	}
}

func TestExtractSnippet_EndLineClamped(t *testing.T) {
	snip := ExtractSnippet(sampleContent, 10, 999)
	if snip == nil {
		t.Fatal("expected non-nil snippet when endLine exceeds content")
	}
	if !strings.Contains(snip.Text, "return") {
		t.Errorf("expected snippet to contain last lines, got %q", snip.Text)
	}
}

func TestExtractSnippet_EndLineLessThanStartLine(t *testing.T) {
	// Should treat as single line
	snip := ExtractSnippet(sampleContent, 6, 3)
	if snip == nil {
		t.Fatal("expected non-nil snippet")
	}
	if !strings.Contains(snip.Text, "password") {
		t.Errorf("expected snippet for line 6, got %q", snip.Text)
	}
}

func TestExtractContextRegion_AddsContext(t *testing.T) {
	ctx := ExtractContextRegion(sampleContent, 6, 6)
	if ctx == nil {
		t.Fatal("expected non-nil context region")
	}
	if ctx.StartLine != 4 {
		t.Errorf("expected context startLine 4, got %d", ctx.StartLine)
	}
	if ctx.EndLine != 8 {
		t.Errorf("expected context endLine 8, got %d", ctx.EndLine)
	}
	if ctx.Snippet == nil {
		t.Fatal("expected context region to have snippet")
	}
	// Context should include the surrounding lines
	if !strings.Contains(ctx.Snippet.Text, "func main()") {
		t.Errorf("expected context to include func main(), got %q", ctx.Snippet.Text)
	}
}

func TestExtractContextRegion_ClampedAtStart(t *testing.T) {
	ctx := ExtractContextRegion(sampleContent, 1, 1)
	if ctx == nil {
		t.Fatal("expected non-nil context region")
	}
	if ctx.StartLine != 1 {
		t.Errorf("expected context startLine 1, got %d", ctx.StartLine)
	}
	if ctx.EndLine != 3 {
		t.Errorf("expected context endLine 3, got %d", ctx.EndLine)
	}
}

func TestExtractContextRegion_NilWhenIdentical(t *testing.T) {
	// Two-line file: context region would be identical to snippet
	content := "line1\nline2"
	ctx := ExtractContextRegion(content, 1, 2)
	if ctx != nil {
		t.Error("expected nil context region when identical to snippet region")
	}
}

func TestExtractContextRegion_EmptyContent(t *testing.T) {
	ctx := ExtractContextRegion("", 1, 1)
	if ctx != nil {
		t.Error("expected nil for empty content")
	}
}

func TestAddSnippets(t *testing.T) {
	results := []Result{
		{
			RuleID:  "test-rule",
			Level:   "warning",
			Message: Message{Text: "issue"},
			Locations: []Location{{
				PhysicalLocation: PhysicalLocation{
					ArtifactLocation: ArtifactLocation{URI: "main.go"},
					Region:           Region{StartLine: 6, EndLine: 6},
				},
			}},
		},
	}

	contentByPath := map[string]string{
		"main.go": sampleContent,
	}

	AddSnippets(results, contentByPath)

	loc := results[0].Locations[0].PhysicalLocation
	if loc.Region.Snippet == nil {
		t.Fatal("expected snippet to be populated")
	}
	if !strings.Contains(loc.Region.Snippet.Text, "password") {
		t.Errorf("expected snippet text with 'password', got %q", loc.Region.Snippet.Text)
	}
	if loc.ContextRegion == nil {
		t.Fatal("expected context region to be populated")
	}
}

func TestAddSnippets_NoLocations(t *testing.T) {
	results := []Result{
		{RuleID: "test-rule", Level: "warning", Message: Message{Text: "issue"}},
	}
	// Should not panic
	AddSnippets(results, map[string]string{"main.go": sampleContent})
}

func TestAddSnippets_MissingContent(t *testing.T) {
	results := []Result{
		{
			RuleID: "test-rule", Level: "warning",
			Locations: []Location{{
				PhysicalLocation: PhysicalLocation{
					ArtifactLocation: ArtifactLocation{URI: "unknown.go"},
					Region:           Region{StartLine: 1, EndLine: 1},
				},
			}},
		},
	}

	AddSnippets(results, map[string]string{})

	if results[0].Locations[0].PhysicalLocation.Region.Snippet != nil {
		t.Error("expected nil snippet for missing content")
	}
}
