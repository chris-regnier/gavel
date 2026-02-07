// internal/lsp/analyzer_test.go
package lsp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/cache"
	"github.com/chris-regnier/gavel/internal/config"
)

// mockBAMLClient implements analyzer.BAMLClient for testing
type mockBAMLClient struct {
	findings []analyzer.Finding
	err      error
}

func (m *mockBAMLClient) AnalyzeCode(ctx context.Context, code string, policies string) ([]analyzer.Finding, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.findings, nil
}

// countingMockClient tracks how many times AnalyzeCode is called
type countingMockClient struct {
	mockBAMLClient
	callCount int
}

func (c *countingMockClient) AnalyzeCode(ctx context.Context, code string, policies string) ([]analyzer.Finding, error) {
	c.callCount++
	return c.mockBAMLClient.AnalyzeCode(ctx, code, policies)
}

// mockCache implements cache.CacheManager for testing
type mockCache struct {
	store map[string]*cache.CacheEntry
}

func newMockCache() *mockCache {
	return &mockCache{
		store: make(map[string]*cache.CacheEntry),
	}
}

func (m *mockCache) Get(ctx context.Context, key cache.CacheKey) (*cache.CacheEntry, error) {
	entry, ok := m.store[key.Hash()]
	if !ok {
		return nil, cache.ErrCacheMiss
	}
	return entry, nil
}

func (m *mockCache) Put(ctx context.Context, entry *cache.CacheEntry) error {
	m.store[entry.Key.Hash()] = entry
	return nil
}

func (m *mockCache) Delete(ctx context.Context, key cache.CacheKey) error {
	delete(m.store, key.Hash())
	return nil
}

func TestAnalyzerWrapper(t *testing.T) {
	// Create mock client with findings
	mockClient := &mockBAMLClient{
		findings: []analyzer.Finding{
			{
				RuleID:         "SEC001",
				Level:          "error",
				Message:        "SQL injection vulnerability",
				FilePath:       "", // Empty means use default path
				StartLine:      10,
				EndLine:        11,
				Recommendation: "Use parameterized queries",
				Explanation:    "Direct string concatenation in SQL",
				Confidence:     0.95,
			},
		},
	}

	// Create config with some policies
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Name: "ollama",
			Ollama: config.OllamaConfig{
				Model: "test-model",
			},
		},
		Policies: map[string]config.Policy{
			"security": {
				Enabled:     true,
				Severity:    "error",
				Instruction: "Check for security issues",
			},
		},
	}

	wrapper := NewAnalyzerWrapper(mockClient, cfg)

	// Run analysis
	results, err := wrapper.Analyze(context.Background(), "/test.go", "package main\n")
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Verify results
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.RuleID != "SEC001" {
		t.Errorf("Expected RuleID SEC001, got %s", result.RuleID)
	}
	if result.Level != "error" {
		t.Errorf("Expected level error, got %s", result.Level)
	}
	if result.Message.Text != "SQL injection vulnerability" {
		t.Errorf("Expected message 'SQL injection vulnerability', got %s", result.Message.Text)
	}

	// Check SARIF location
	if len(result.Locations) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(result.Locations))
	}
	loc := result.Locations[0]
	if loc.PhysicalLocation.ArtifactLocation.URI != "/test.go" {
		t.Errorf("Expected URI /test.go, got %s", loc.PhysicalLocation.ArtifactLocation.URI)
	}
	if loc.PhysicalLocation.Region.StartLine != 10 {
		t.Errorf("Expected StartLine 10, got %d", loc.PhysicalLocation.Region.StartLine)
	}

	// Check gavel properties
	if result.Properties == nil {
		t.Fatal("Expected Properties to be set")
	}
	if conf, ok := result.Properties["gavel/confidence"].(float64); !ok || conf != 0.95 {
		t.Errorf("Expected confidence 0.95, got %v", result.Properties["gavel/confidence"])
	}
	if rec, ok := result.Properties["gavel/recommendation"].(string); !ok || rec != "Use parameterized queries" {
		t.Errorf("Expected recommendation, got %v", result.Properties["gavel/recommendation"])
	}
}

func TestAnalyzerWrapperWithCache(t *testing.T) {
	// Create counting mock client
	mockClient := &countingMockClient{
		mockBAMLClient: mockBAMLClient{
			findings: []analyzer.Finding{
				{
					RuleID:    "TEST001",
					Level:     "warning",
					Message:   "Test finding",
					FilePath:  "", // Empty means use default path
					StartLine: 5,
					EndLine:   5,
				},
			},
		},
	}

	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Name: "ollama",
			Ollama: config.OllamaConfig{
				Model: "test-model",
			},
		},
		Policies: map[string]config.Policy{
			"style": {
				Enabled:     true,
				Severity:    "warning",
				Instruction: "Check code style",
			},
		},
	}

	mockCache := newMockCache()
	wrapper := NewAnalyzerWrapper(mockClient, cfg).WithCache(mockCache)

	ctx := context.Background()
	content := "package main\nfunc main() {}\n"

	// First call - should invoke analyzer
	results1, err := wrapper.Analyze(ctx, "/test.go", content)
	if err != nil {
		t.Fatalf("First analyze failed: %v", err)
	}
	if len(results1) != 1 {
		t.Fatalf("Expected 1 result from first call, got %d", len(results1))
	}
	if mockClient.callCount != 1 {
		t.Errorf("Expected 1 call to AnalyzeCode, got %d", mockClient.callCount)
	}

	// Second call with same content - should hit cache
	results2, err := wrapper.Analyze(ctx, "/test.go", content)
	if err != nil {
		t.Fatalf("Second analyze failed: %v", err)
	}
	if len(results2) != 1 {
		t.Fatalf("Expected 1 result from second call, got %d", len(results2))
	}
	if mockClient.callCount != 1 {
		t.Errorf("Expected call count to remain 1 (cache hit), got %d", mockClient.callCount)
	}

	// Verify cached results match
	if results1[0].RuleID != results2[0].RuleID {
		t.Errorf("Cached result RuleID mismatch: %s vs %s", results1[0].RuleID, results2[0].RuleID)
	}

	// Third call with different content - should invoke analyzer again
	results3, err := wrapper.Analyze(ctx, "/test.go", "package main\n// modified\nfunc main() {}\n")
	if err != nil {
		t.Fatalf("Third analyze failed: %v", err)
	}
	if len(results3) != 1 {
		t.Fatalf("Expected 1 result from third call, got %d", len(results3))
	}
	if mockClient.callCount != 2 {
		t.Errorf("Expected 2 calls to AnalyzeCode (cache miss), got %d", mockClient.callCount)
	}
}

func TestBuildCacheKey(t *testing.T) {
	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Name: "ollama",
			Ollama: config.OllamaConfig{
				Model: "test-model",
			},
		},
		Policies: map[string]config.Policy{
			"security": {
				Enabled:     true,
				Severity:    "error",
				Instruction: "Check for security issues",
			},
			"style": {
				Enabled:     false, // Disabled, should not be included
				Severity:    "warning",
				Instruction: "Check code style",
			},
		},
	}

	wrapper := NewAnalyzerWrapper(&mockBAMLClient{}, cfg)

	// Test cache key generation
	path := "/test.go"
	content := "package main\n"
	key := wrapper.buildCacheKey(path, content)

	// Verify key fields
	if key.FilePath != path {
		t.Errorf("Expected FilePath %s, got %s", path, key.FilePath)
	}

	// Verify file hash
	h := sha256.Sum256([]byte(content))
	expectedHash := hex.EncodeToString(h[:])
	if key.FileHash != expectedHash {
		t.Errorf("Expected FileHash %s, got %s", expectedHash, key.FileHash)
	}

	if key.Provider != "ollama" {
		t.Errorf("Expected Provider ollama, got %s", key.Provider)
	}
	if key.Model != "test-model" {
		t.Errorf("Expected Model test-model, got %s", key.Model)
	}

	// Verify only enabled policy is included
	if len(key.Policies) != 1 {
		t.Fatalf("Expected 1 policy in cache key, got %d", len(key.Policies))
	}
	if _, ok := key.Policies["security"]; !ok {
		t.Error("Expected 'security' policy in cache key")
	}
	if _, ok := key.Policies["style"]; ok {
		t.Error("Did not expect disabled 'style' policy in cache key")
	}
}

func TestFormatPolicies(t *testing.T) {
	cfg := &config.Config{
		Policies: map[string]config.Policy{
			"security": {
				Enabled:     true,
				Severity:    "error",
				Instruction: "Check for security issues",
			},
			"style": {
				Enabled:     true,
				Severity:    "warning",
				Instruction: "Check code style",
			},
			"disabled": {
				Enabled:     false,
				Severity:    "info",
				Instruction: "This should not appear",
			},
		},
	}

	wrapper := NewAnalyzerWrapper(&mockBAMLClient{}, cfg)
	formatted := wrapper.formatPolicies()

	// Should contain enabled policies
	if !strings.Contains(formatted, "security") {
		t.Error("Expected formatted policies to contain 'security'")
	}
	if !strings.Contains(formatted, "Check for security issues") {
		t.Error("Expected formatted policies to contain security instruction")
	}
	if !strings.Contains(formatted, "style") {
		t.Error("Expected formatted policies to contain 'style'")
	}

	// Should not contain disabled policy
	if strings.Contains(formatted, "disabled") {
		t.Error("Did not expect formatted policies to contain 'disabled'")
	}
	if strings.Contains(formatted, "This should not appear") {
		t.Error("Did not expect formatted policies to contain disabled instruction")
	}
}
