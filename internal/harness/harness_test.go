package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
)

func TestLoadHarnessConfig(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantRuns int
		wantVars int
	}{
		{
			name: "basic config",
			yaml: `
variants:
  - name: baseline
runs: 5
packages:
  - internal/mcp
`,
			wantRuns: 5,
			wantVars: 1,
		},
		{
			name: "default runs",
			yaml: `
variants:
  - name: a
  - name: b
packages:
  - pkg1
`,
			wantRuns: 3,
			wantVars: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadHarnessConfig([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("LoadHarnessConfig() error = %v", err)
			}
			if cfg.Runs != tt.wantRuns {
				t.Errorf("Runs = %d, want %d", cfg.Runs, tt.wantRuns)
			}
			if len(cfg.Variants) != tt.wantVars {
				t.Errorf("Variants count = %d, want %d", len(cfg.Variants), tt.wantVars)
			}
		})
	}
}

func TestVariantMergeWithConfig(t *testing.T) {
	base := &config.Config{
		Persona:      "code-reviewer",
		StrictFilter: true,
		Policies: map[string]config.Policy{
			"test-policy": {
				Description: "Base policy",
				Severity:    "warning",
				Enabled:     true,
			},
		},
		Provider: config.ProviderConfig{
			Name: "ollama",
			Ollama: config.OllamaConfig{
				Model:   "qwen2.5-coder:7b",
				BaseURL: "http://localhost:11434",
			},
		},
	}

	tests := []struct {
		name    string
		variant VariantConfig
		check   func(t *testing.T, result *config.Config)
	}{
		{
			name: "override persona",
			variant: VariantConfig{
				Persona: "architect",
			},
			check: func(t *testing.T, result *config.Config) {
				if result.Persona != "architect" {
					t.Errorf("Persona = %s, want architect", result.Persona)
				}
			},
		},
		{
			name: "override strict_filter false",
			variant: VariantConfig{
				StrictFilter: ptrBool(false),
			},
			check: func(t *testing.T, result *config.Config) {
				if result.StrictFilter {
					t.Error("StrictFilter should be false")
				}
			},
		},
		{
			name: "add policy",
			variant: VariantConfig{
				Policies: map[string]config.Policy{
					"new-policy": {
						Description: "New policy",
						Severity:    "error",
						Enabled:     true,
					},
				},
			},
			check: func(t *testing.T, result *config.Config) {
				if _, ok := result.Policies["new-policy"]; !ok {
					t.Error("new-policy should be present")
				}
				if _, ok := result.Policies["test-policy"]; !ok {
					t.Error("test-policy should still be present")
				}
			},
		},
		{
			name: "override provider model",
			variant: VariantConfig{
				Provider: &ProviderOverride{
					Ollama: &struct {
						Model   string `yaml:"model,omitempty"`
						BaseURL string `yaml:"base_url,omitempty"`
					}{
						Model: "qwen2.5-coder:14b",
					},
				},
			},
			check: func(t *testing.T, result *config.Config) {
				if result.Provider.Ollama.Model != "qwen2.5-coder:14b" {
					t.Errorf("Model = %s, want qwen2.5-coder:14b", result.Provider.Ollama.Model)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.variant.MergeWithConfig(base)
			tt.check(t, result)
		})
	}
}

func TestMetricsWriteJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "results.jsonl")

	m1 := RunMetrics{Run: 1, Variant: "baseline", Target: "pkg1", Total: 10}
	m2 := RunMetrics{Run: 2, Variant: "baseline", Target: "pkg1", Total: 15}

	if err := m1.WriteJSONL(path); err != nil {
		t.Fatalf("WriteJSONL() error = %v", err)
	}
	if err := m2.WriteJSONL(path); err != nil {
		t.Fatalf("WriteJSONL() error = %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// Simpler verification: check file has 2 lines
	lines := len(splitLines(string(data)))
	if lines != 2 {
		t.Errorf("Expected 2 lines, got %d", lines)
	}
}

func TestSummarize(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "results.jsonl")

	// Write test records
	records := []RunMetrics{
		{Run: 1, Variant: "baseline", Target: "pkg1", Total: 10, LLM: 8, Errors: 3, HighConfErrors: 2, AvgConfidence: 0.85},
		{Run: 2, Variant: "baseline", Target: "pkg1", Total: 12, LLM: 10, Errors: 4, HighConfErrors: 3, AvgConfidence: 0.82},
		{Run: 1, Variant: "variant", Target: "pkg1", Total: 8, LLM: 6, Errors: 2, HighConfErrors: 1, AvgConfidence: 0.90},
		{Run: 2, Variant: "variant", Target: "pkg1", Total: 6, LLM: 4, Errors: 1, HighConfErrors: 0, AvgConfidence: 0.88},
	}

	// Write all records to one file
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	for _, r := range records {
		data, _ := json.Marshal(r)
		f.Write(append(data, '\n'))
	}
	f.Close()

	summary, err := Summarize(path)
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}

	if len(summary.Variants) != 2 {
		t.Errorf("Expected 2 variants, got %d", len(summary.Variants))
	}

	// Check baseline stats
	var baseline *VariantSummary
	for i, v := range summary.Variants {
		if v.Name == "baseline" {
			baseline = &summary.Variants[i]
			break
		}
	}
	if baseline == nil {
		t.Fatal("baseline variant not found")
	}
	if baseline.Total.Mean != 11 { // (10+12)/2
		t.Errorf("baseline Total.Mean = %v, want 11", baseline.Total.Mean)
	}
}

func TestPctDelta(t *testing.T) {
	tests := []struct {
		value    float64
		baseline float64
		want     float64
	}{
		{110, 100, 10},
		{90, 100, -10},
		{100, 100, 0},
		{50, 0, 0}, // avoid divide by zero
	}

	for _, tt := range tests {
		got := pctDelta(tt.value, tt.baseline)
		if got != tt.want {
			t.Errorf("pctDelta(%v, %v) = %v, want %v", tt.value, tt.baseline, got, tt.want)
		}
	}
}

func ptrBool(b bool) *bool {
	return &b
}

func TestRepositoryConfig(t *testing.T) {
	// Test that we can load a config with repos
	yaml := `
repos:
  - name: juice-shop
    url: https://github.com/juice-shop/juice-shop
    branch: master
  - name: dvwa
    url: https://github.com/digininja/DVWA
    depth: 5

targets:
  - repo: juice-shop
    paths:
      - server
      - frontend
  - path: internal/mcp

variants:
  - name: baseline
`

	cfg, err := LoadHarnessConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadHarnessConfig() error = %v", err)
	}

	if len(cfg.Repos) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(cfg.Repos))
	}

	if cfg.Repos[0].Name != "juice-shop" {
		t.Errorf("Repo name = %s, want juice-shop", cfg.Repos[0].Name)
	}

	if cfg.Repos[0].Branch != "master" {
		t.Errorf("Branch = %s, want master", cfg.Repos[0].Branch)
	}

	if cfg.Repos[1].Depth != 5 {
		t.Errorf("Depth = %d, want 5", cfg.Repos[1].Depth)
	}

	if len(cfg.Targets) != 2 {
		t.Errorf("Expected 2 targets, got %d", len(cfg.Targets))
	}

	if cfg.Targets[0].Repo != "juice-shop" {
		t.Errorf("Target repo = %s, want juice-shop", cfg.Targets[0].Repo)
	}

	if len(cfg.Targets[0].Paths) != 2 {
		t.Errorf("Expected 2 paths, got %d", len(cfg.Targets[0].Paths))
	}
}

func TestResolveTargets(t *testing.T) {
	rm := NewRepositoryManager(t.TempDir())

	// Simulate already-cloned repos
	rm.repos["juice-shop"] = "/tmp/juice-shop"
	rm.repos["dvwa"] = "/tmp/dvwa"

	tests := []struct {
		name     string
		targets  []TargetConfig
		packages []string
		want     int
		wantErr  bool
	}{
		{
			name:     "local packages only",
			packages: []string{"internal/mcp", "internal/store"},
			want:     2,
		},
		{
			name: "targets with local path",
			targets: []TargetConfig{
				{Path: "internal/mcp"},
			},
			want: 1,
		},
		{
			name: "targets with repo",
			targets: []TargetConfig{
				{Repo: "juice-shop"},
			},
			want: 1,
		},
		{
			name: "targets with repo and subpaths",
			targets: []TargetConfig{
				{Repo: "juice-shop", Paths: []string{"server", "frontend"}},
			},
			want: 2,
		},
		{
			name: "unknown repo",
			targets: []TargetConfig{
				{Repo: "unknown"},
			},
			wantErr: true,
		},
		{
			name: "mixed targets and packages",
			targets: []TargetConfig{
				{Path: "internal/mcp"},
				{Repo: "dvwa"},
			},
			packages: []string{"internal/store"},
			want:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths, err := rm.ResolveTargets(tt.targets, tt.packages)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveTargets() error = %v", err)
			}
			if len(paths) != tt.want {
				t.Errorf("Got %d paths, want %d", len(paths), tt.want)
			}
		})
	}
}

func TestSanitizeRepoName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"juice-shop", "juice-shop"},
		{"foo/bar", "foo-bar"},
		{"foo:bar", "foo-bar"},
		{"foo bar", "foo-bar"},
		{"org/repo-name", "org-repo-name"},
	}

	for _, tt := range tests {
		got := sanitizeRepoName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeRepoName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range []byte(s) {
		_ = line
	}
	// Simplified split
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
