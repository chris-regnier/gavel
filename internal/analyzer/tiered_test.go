package analyzer

import (
	"context"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
)

type tieredMockClient struct {
	findings  []Finding
	callCount atomic.Int32
	delay     time.Duration
}

func (m *tieredMockClient) AnalyzeCode(ctx context.Context, code string, policies string, personaPrompt string, additionalContext string) ([]Finding, error) {
	m.callCount.Add(1)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.findings, nil
}

func TestTieredAnalyzer_InstantTier_PatternMatching(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}
	ta := NewTieredAnalyzer(mock)

	// Code with detectable patterns
	code := `package main

func process() {
	data, _ := getData() // Error ignored
	// TODO: fix this later
	password := "secret123"
	fmt.Println("debug")
}`

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: code,
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

	if len(instantResults) != 1 {
		t.Fatalf("expected 1 instant result, got %d", len(instantResults))
	}

	// Should have found multiple patterns
	if len(instantResults[0].Results) < 2 {
		t.Errorf("expected at least 2 pattern matches, got %d", len(instantResults[0].Results))
	}

	// Check that specific patterns were found
	foundPatterns := make(map[string]bool)
	for _, r := range instantResults[0].Results {
		foundPatterns[r.RuleID] = true
		t.Logf("Found pattern: %s", r.RuleID)
	}

	// These patterns should definitely match (using new standardized IDs)
	expectedPatterns := []string{"S1086", "S1135"} // error-ignored, todo-fixme
	for _, p := range expectedPatterns {
		if !foundPatterns[p] {
			t.Errorf("expected to find pattern %s", p)
		}
	}
}

func TestTieredAnalyzer_InstantTier_CacheHit(t *testing.T) {
	mock := &tieredMockClient{
		findings: []Finding{{RuleID: "test", Level: "warning", Message: "Test"}},
	}
	ta := NewTieredAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "package main",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	// First call - populates cache via comprehensive tier
	for range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
	}

	// Second call - should hit cache in instant tier
	var cacheHit bool
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierInstant && result.FromCache {
			cacheHit = true
		}
	}

	if !cacheHit {
		t.Error("expected cache hit in instant tier on second call")
	}

	stats := ta.Stats()
	if stats.InstantHits != 1 {
		t.Errorf("expected 1 instant hit, got %d", stats.InstantHits)
	}
}

func TestTieredAnalyzer_ComprehensiveTier(t *testing.T) {
	mock := &tieredMockClient{
		findings: []Finding{{
			RuleID:     "comprehensive-finding",
			Level:      "warning",
			Message:    "Found by LLM",
			Confidence: 0.9,
		}},
	}
	ta := NewTieredAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "package main\n\nfunc main() {}",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	var comprehensiveResults []TieredResult
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierComprehensive {
			comprehensiveResults = append(comprehensiveResults, result)
		}
	}

	if len(comprehensiveResults) != 1 {
		t.Fatalf("expected 1 comprehensive result, got %d", len(comprehensiveResults))
	}

	if comprehensiveResults[0].Error != nil {
		t.Fatalf("unexpected error: %v", comprehensiveResults[0].Error)
	}

	if len(comprehensiveResults[0].Results) != 1 {
		t.Errorf("expected 1 finding, got %d", len(comprehensiveResults[0].Results))
	}

	// Check tier tag
	tier, ok := comprehensiveResults[0].Results[0].Properties["gavel/tier"].(string)
	if !ok || tier != "comprehensive" {
		t.Error("expected tier tag 'comprehensive'")
	}
}

func TestTieredAnalyzer_FastTier(t *testing.T) {
	fastMock := &tieredMockClient{
		findings: []Finding{{RuleID: "fast-finding", Level: "warning", Message: "Fast check"}},
		delay:    10 * time.Millisecond,
	}
	comprehensiveMock := &tieredMockClient{
		findings: []Finding{{RuleID: "comprehensive-finding", Level: "error", Message: "Full check"}},
		delay:    50 * time.Millisecond,
	}

	ta := NewTieredAnalyzer(comprehensiveMock, WithFastClient(fastMock))

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "package main",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	var tiers []Tier
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		tiers = append(tiers, result.Tier)
	}

	// Should have instant, fast, and comprehensive
	if len(tiers) != 3 {
		t.Errorf("expected 3 tier results, got %d: %v", len(tiers), tiers)
	}

	stats := ta.Stats()
	if stats.FastCalls != 1 {
		t.Errorf("expected 1 fast call, got %d", stats.FastCalls)
	}
}

func TestTieredAnalyzer_ProgressiveOrder(t *testing.T) {
	fastMock := &tieredMockClient{
		findings: []Finding{{RuleID: "fast"}},
		delay:    20 * time.Millisecond,
	}
	comprehensiveMock := &tieredMockClient{
		findings: []Finding{{RuleID: "comprehensive"}},
		delay:    50 * time.Millisecond,
	}

	ta := NewTieredAnalyzer(comprehensiveMock, WithFastClient(fastMock))

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "// TODO: test",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	var order []Tier
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		order = append(order, result.Tier)
	}

	// Should be in order: instant, fast, comprehensive
	expected := []Tier{TierInstant, TierFast, TierComprehensive}
	if len(order) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(order))
	}

	for i, tier := range order {
		if tier != expected[i] {
			t.Errorf("position %d: expected %v, got %v", i, expected[i], tier)
		}
	}
}

func TestTieredAnalyzer_Analyze_Deduplicated(t *testing.T) {
	// Mock returns finding with same rule ID as instant tier pattern (S1135)
	mock := &tieredMockClient{
		findings: []Finding{{
			RuleID:    "S1135", // Same as instant tier TODO pattern
			Level:     "note",
			Message:   "LLM found TODO",
			StartLine: 1,
			EndLine:   1,
			FilePath:  "test.go",
		}},
	}
	ta := NewTieredAnalyzer(mock)

	// Code that will trigger pattern match for TODO
	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "// TODO: implement this",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	results, err := ta.Analyze(context.Background(), artifacts, policies, "persona")
	if err != nil {
		t.Fatal(err)
	}

	// Both instant and comprehensive will find the TODO with same rule ID
	// Deduplication should keep only comprehensive (higher tier)
	todoCount := 0
	for _, r := range results {
		if r.RuleID == "S1135" {
			todoCount++
			// Should be from comprehensive tier (higher priority)
			if tier, ok := r.Properties["gavel/tier"].(string); ok {
				if tier != "comprehensive" {
					t.Errorf("expected comprehensive tier, got %s", tier)
				}
			}
		}
	}

	if todoCount != 1 {
		t.Errorf("expected 1 deduplicated TODO finding, got %d", todoCount)
	}
}

func TestTieredAnalyzer_CustomPatterns(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}
	ta := NewTieredAnalyzer(mock)

	// Add custom pattern
	ta.AddPattern(PatternRule{
		ID:         "custom-pattern",
		Pattern:    regexp.MustCompile(`CUSTOM_MARKER`),
		Level:      "error",
		Message:    "Custom marker found",
		Confidence: 1.0,
	})

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "// CUSTOM_MARKER: check this",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	var found bool
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierInstant {
			for _, r := range result.Results {
				if r.RuleID == "custom-pattern" {
					found = true
				}
			}
		}
	}

	if !found {
		t.Error("expected to find custom pattern")
	}
}

func TestTieredAnalyzer_DisableInstant(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}
	ta := NewTieredAnalyzer(mock, WithInstantEnabled(false))

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "// TODO: test",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	var hasInstant bool
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierInstant {
			hasInstant = true
		}
	}

	if hasInstant {
		t.Error("instant tier should be disabled")
	}
}

func TestTieredAnalyzer_ContextCancellation(t *testing.T) {
	mock := &tieredMockClient{
		findings: []Finding{},
		delay:    1 * time.Second,
	}
	ta := NewTieredAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "package main",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var gotError bool
	for result := range ta.AnalyzeProgressive(ctx, artifacts, policies, "persona") {
		if result.Error != nil {
			gotError = true
		}
	}

	if !gotError {
		t.Error("expected context cancellation error")
	}
}

func TestTieredAnalyzer_MultipleArtifacts(t *testing.T) {
	mock := &tieredMockClient{
		findings: []Finding{{RuleID: "test", Level: "warning"}},
	}
	ta := NewTieredAnalyzer(mock)

	artifacts := []input.Artifact{
		{Path: "file1.go", Content: "// TODO: one"},
		{Path: "file2.go", Content: "// TODO: two"},
		{Path: "file3.go", Content: "// TODO: three"},
	}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	resultsByFile := make(map[string]int)
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		resultsByFile[result.FilePath]++
	}

	// Each file should have instant + comprehensive = 2 results
	for _, path := range []string{"file1.go", "file2.go", "file3.go"} {
		if resultsByFile[path] != 2 {
			t.Errorf("expected 2 tier results for %s, got %d", path, resultsByFile[path])
		}
	}
}

func TestTieredAnalyzer_Stats(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}
	ta := NewTieredAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "package main",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	// First call
	for range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
	}

	// Second call (should hit cache)
	for range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
	}

	stats := ta.Stats()

	if stats.InstantMisses != 1 {
		t.Errorf("expected 1 instant miss, got %d", stats.InstantMisses)
	}
	if stats.InstantHits != 1 {
		t.Errorf("expected 1 instant hit, got %d", stats.InstantHits)
	}
	if stats.ComprehensiveCalls != 2 {
		t.Errorf("expected 2 comprehensive calls, got %d", stats.ComprehensiveCalls)
	}
}

func TestTier_String(t *testing.T) {
	tests := []struct {
		tier Tier
		want string
	}{
		{TierInstant, "instant"},
		{TierFast, "fast"},
		{TierComprehensive, "comprehensive"},
		{Tier(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("Tier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func BenchmarkTieredAnalyzer_InstantTier(b *testing.B) {
	mock := &tieredMockClient{findings: []Finding{}}
	ta := NewTieredAnalyzer(mock, WithInstantEnabled(true))

	artifact := input.Artifact{
		Path: "test.go",
		Content: `package main

func process() {
	data, _ := getData()
	// TODO: fix
	password := "secret"
}`,
		Kind: input.KindFile,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		results := ta.runPatternMatching(artifact)
		_ = results
	}
}

func BenchmarkTieredAnalyzer_ProgressiveAnalysis(b *testing.B) {
	mock := &tieredMockClient{findings: []Finding{{RuleID: "test"}}}
	ta := NewTieredAnalyzer(mock)

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: "package main\n\nfunc main() {}",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		}
	}
}
