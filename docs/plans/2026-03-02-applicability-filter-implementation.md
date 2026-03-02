# Applicability Filter Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add an applicability filter to the BAML prompt that suppresses theoretical, speculative, and severity-miscalibrated findings from LLM analysis.

**Architecture:** The filter text is a constant appended to the persona prompt string on the Go side when `Config.StrictFilter` is true (default). No BAML template, interface, or code generation changes needed — the filter flows into the BAML prompt via the existing `{{ personaPrompt }}` interpolation.

**Tech Stack:** Go, YAML config, BAML (unchanged)

---

### Task 1: Add `ApplicabilityFilterPrompt` constant

**Files:**
- Modify: `internal/analyzer/personas.go`
- Test: `internal/analyzer/personas_test.go` (create if needed)

**Step 1: Write the failing test**

Add to `internal/analyzer/personas_test.go`:

```go
package analyzer

import (
	"testing"
)

func TestApplicabilityFilterPrompt_NotEmpty(t *testing.T) {
	if ApplicabilityFilterPrompt == "" {
		t.Error("ApplicabilityFilterPrompt should not be empty")
	}
}

func TestApplicabilityFilterPrompt_ContainsKeyPhrases(t *testing.T) {
	phrases := []string{
		"PRACTICAL IMPACT",
		"CONCRETE EVIDENCE",
		"PROPORTIONAL SEVERITY",
		"do not report it",
	}
	for _, phrase := range phrases {
		if !strings.Contains(ApplicabilityFilterPrompt, phrase) {
			t.Errorf("ApplicabilityFilterPrompt missing phrase: %q", phrase)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/analyzer/ -run TestApplicabilityFilterPrompt -v`
Expected: FAIL — `ApplicabilityFilterPrompt` is not defined.

**Step 3: Write minimal implementation**

Add to `internal/analyzer/personas.go`, after the existing const block:

```go
// ApplicabilityFilterPrompt is an optional instruction block appended to persona
// prompts to suppress findings that are theoretical, speculative, or
// severity-miscalibrated. Controlled by Config.StrictFilter (default true).
const ApplicabilityFilterPrompt = `

===== APPLICABILITY FILTER =====
Before reporting any finding, apply this applicability test:

1. PRACTICAL IMPACT: Would this issue cause a real problem in a realistic
   production scenario? If it requires an unrealistic or adversarial
   configuration to trigger, do not report it.

2. CONCRETE EVIDENCE: Is there concrete evidence in the code that this is
   an actual problem? If it is purely speculative ("this might not be
   thread-safe", "this could theoretically fail"), do not report it.

3. PROPORTIONAL SEVERITY: Assign severity proportional to actual impact.
   Test hygiene issues are "note" level. Theoretical concerns that survive
   tests 1-2 are "warning" at most. Reserve "error" for clear,
   demonstrable defects.
===== END FILTER =====`
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/analyzer/ -run TestApplicabilityFilterPrompt -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/analyzer/personas.go internal/analyzer/personas_test.go
git commit -m "feat: add ApplicabilityFilterPrompt constant for noise reduction"
```

---

### Task 2: Add `StrictFilter` to Config

**Files:**
- Modify: `internal/config/config.go` (add field to `Config` struct)
- Modify: `internal/config/defaults.go` (set default to true)
- Modify: `internal/config/config.go` (handle in `MergeConfigs`)
- Test: `internal/config/config_test.go`

**Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestSystemDefaults_StrictFilterEnabled(t *testing.T) {
	defaults := SystemDefaults()
	if !defaults.StrictFilter {
		t.Error("expected StrictFilter to default to true")
	}
}

func TestMergeConfigs_StrictFilterOverride(t *testing.T) {
	system := &Config{StrictFilter: true}
	project := &Config{StrictFilter: false}
	// StrictFilter is a bool — when a higher tier explicitly provides the
	// config section, its value should be used.
	merged := MergeConfigs(system, project)
	if merged.StrictFilter {
		t.Error("expected project to override StrictFilter to false")
	}
}

func TestMergeConfigs_StrictFilterPreserved(t *testing.T) {
	system := &Config{StrictFilter: true}
	project := &Config{} // no strict_filter key — zero value
	merged := MergeConfigs(system, project)
	if !merged.StrictFilter {
		t.Error("expected StrictFilter to remain true when project doesn't set it")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestSystemDefaults_StrictFilter -v && go test ./internal/config/ -run TestMergeConfigs_StrictFilter -v`
Expected: FAIL — `StrictFilter` field does not exist.

**Step 3: Write minimal implementation**

In `internal/config/config.go`, add field to `Config` struct:

```go
type Config struct {
	Provider     ProviderConfig    `yaml:"provider"`
	Persona      string            `yaml:"persona"`
	StrictFilter bool              `yaml:"strict_filter"`
	Policies     map[string]Policy `yaml:"policies"`
	LSP          LSPConfig         `yaml:"lsp"`
	RemoteCache  RemoteCacheConfig `yaml:"remote_cache"`
	Telemetry    TelemetryConfig   `yaml:"telemetry"`
}
```

In `internal/config/defaults.go`, add to `SystemDefaults()`:

```go
StrictFilter: true,
```

In `internal/config/config.go` `MergeConfigs()`, add handling after the persona merge block. Since `StrictFilter` is a bool with zero value `false`, we need a way to distinguish "explicitly set to false" from "not set". The simplest approach: only override if the higher-tier config has `StrictFilter` explicitly set. Since YAML will set `false` when present and leave default `false` when absent, we can detect this by checking if the strict_filter section was explicitly present. However, for simplicity and consistency with the existing pattern (where bool fields like `Telemetry.Enabled` use a similar approach), we use a pragmatic rule: if any other field in the config is set, or if `StrictFilter` is explicitly true, apply it.

Actually, the simplest correct approach: since `StrictFilter` defaults to `true` in SystemDefaults, and the zero value is `false`, a higher-tier config with `strict_filter: false` will correctly override. But a config that doesn't mention `strict_filter` at all will also have `false` as the zero value, incorrectly overriding.

To handle this cleanly, we use the same pattern as the telemetry section: only apply the bool if the config section appears to be "intentionally present." For `StrictFilter`, since it's a top-level bool, we'll use a pointer type (`*bool`) in a helper struct, or more simply: we never override `StrictFilter` with `false` unless the higher-tier config has at least one other field set (indicating it's a real config file, not an empty struct).

The simplest pragmatic approach consistent with the codebase: just always take the higher-tier value for `StrictFilter` if that tier has any non-empty field (indicating it's a loaded config, not a nil/empty one). Since `MergeConfigs` already skips `nil` configs, any non-nil config should be treated as intentional:

```go
// Merge strict_filter - if higher tier has any configuration at all,
// use its StrictFilter value (handles both true and false overrides).
// This works because nil configs are already skipped, and SystemDefaults
// sets it to true.
if cfg.Provider.Name != "" || cfg.Persona != "" || len(cfg.Policies) > 0 ||
   cfg.Telemetry.Endpoint != "" || cfg.RemoteCache.URL != "" || cfg.StrictFilter {
	result.StrictFilter = cfg.StrictFilter
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run "TestSystemDefaults_StrictFilter|TestMergeConfigs_StrictFilter" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/defaults.go internal/config/config_test.go
git commit -m "feat: add StrictFilter config field (default true)"
```

---

### Task 3: Wire filter into CLI analyze command

**Files:**
- Modify: `cmd/gavel/analyze.go`

**Step 1: Write the implementation**

In `cmd/gavel/analyze.go`, after the `personaPrompt` is obtained (line ~113) and before it's passed to the analyzer (line ~177), append the filter:

```go
// Get persona prompt from BAML
personaPrompt, err := analyzer.GetPersonaPrompt(ctx, cfg.Persona)
if err != nil {
	return fmt.Errorf("loading persona %s: %w", cfg.Persona, err)
}

// Append applicability filter if enabled (default)
if cfg.StrictFilter {
	personaPrompt += analyzer.ApplicabilityFilterPrompt
}
```

This is a 3-line addition between two existing blocks.

**Step 2: Run existing tests to verify no regressions**

Run: `go test ./cmd/gavel/ -v` (or `go build ./cmd/gavel/` if no cmd tests exist)
Expected: PASS / builds cleanly

**Step 3: Commit**

```bash
git add cmd/gavel/analyze.go
git commit -m "feat: wire applicability filter into CLI analyze command"
```

---

### Task 4: Wire filter into MCP server

**Files:**
- Modify: `internal/mcp/server.go`

**Step 1: Write the implementation**

In `internal/mcp/server.go`, in the `runAnalysis` helper (line ~474), append the filter after getting the persona prompt:

```go
func (h *handlers) runAnalysis(ctx context.Context, artifacts []input.Artifact, persona string) ([]sarif.Result, error) {
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, persona)
	if err != nil {
		return nil, fmt.Errorf("loading persona %s: %w", persona, err)
	}

	// Append applicability filter if enabled (default)
	if h.cfg.Config.StrictFilter {
		personaPrompt += analyzer.ApplicabilityFilterPrompt
	}

	a := analyzer.NewAnalyzer(h.client)
	return a.Analyze(ctx, artifacts, h.cfg.Config.Policies, personaPrompt)
}
```

**Step 2: Run MCP tests to verify no regressions**

Run: `go test ./internal/mcp/ -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/mcp/server.go
git commit -m "feat: wire applicability filter into MCP server"
```

---

### Task 5: Add integration-style test for filter wiring

**Files:**
- Test: `internal/analyzer/personas_test.go`

**Step 1: Write the test**

Add to `internal/analyzer/personas_test.go`:

```go
func TestGetPersonaPrompt_WithFilter(t *testing.T) {
	personas := []string{"code-reviewer", "architect", "security"}
	for _, persona := range personas {
		prompt, err := GetPersonaPrompt(context.Background(), persona)
		if err != nil {
			t.Fatalf("GetPersonaPrompt(%s): %v", persona, err)
		}

		// Simulate what the caller does when StrictFilter is true
		filtered := prompt + ApplicabilityFilterPrompt

		if !strings.Contains(filtered, "APPLICABILITY FILTER") {
			t.Errorf("filtered %s prompt missing filter block", persona)
		}
		if !strings.Contains(filtered, "CONFIDENCE GUIDANCE") {
			t.Errorf("filtered %s prompt lost original confidence guidance", persona)
		}
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/analyzer/ -run TestGetPersonaPrompt_WithFilter -v`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/analyzer/personas_test.go
git commit -m "test: add integration test for persona prompt with applicability filter"
```

---

### Task 6: Run full test suite and verify

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 2: Run linter**

Run: `go vet ./...`
Expected: No issues

**Step 3: Build**

Run: `task build`
Expected: Clean build

**Step 4: Final commit (if any fixups needed)**

```bash
git add -A
git commit -m "fix: address test/lint issues from applicability filter"
```
