# Finding Suppression Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add project-local finding suppression to Gavel so users and AI agents can dismiss findings by rule ID (globally or per-file), with suppressions stored in `.gavel/suppressions.yaml` and applied as SARIF-native annotations.

**Architecture:** New `internal/suppression/` package handles Load/Save/Match/Apply. CLI commands (`suppress`, `unsuppress`, `suppressions`) and MCP tools provide the interface. Suppressions are stamped onto SARIF results at analyze time and re-applied at judge time. The Rego evaluator filters suppressed results.

**Tech Stack:** Go, gopkg.in/yaml.v3, Cobra CLI, mcp-go, OPA/Rego

**Spec:** `docs/superpowers/specs/2026-03-20-finding-suppression-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/suppression/suppression.go` | Suppression type, Load, Save, Match, Apply, NormalizePath |
| Create | `internal/suppression/suppression_test.go` | Unit tests for all suppression logic |
| Modify | `internal/sarif/sarif.go:45-52` | Add SARIFSuppression type and Suppressions field to Result |
| Modify | `internal/evaluator/default.rego:1-16` | Add unsuppressed_results filter, update decision rules |
| Modify | `internal/evaluator/evaluator.go:98-124` | Filter suppressed results from RelevantFindings, update reason string |
| Create | `internal/evaluator/evaluator_suppression_test.go` | Test Rego and Go-side suppression filtering |
| Create | `cmd/gavel/suppress.go` | CLI commands: suppress, unsuppress, suppressions |
| Create | `cmd/gavel/suppress_test.go` | CLI command tests |
| Modify | `cmd/gavel/analyze.go:201-210` | Load and apply suppressions after SARIF assembly |
| Modify | `cmd/gavel/judge.go:97-113` | Re-apply suppressions before Rego evaluation |
| Modify | `internal/mcp/server.go:36-67` | Register 3 new suppression tools |
| Modify | `internal/mcp/server_test.go` | Test new MCP tools |

---

### Task 1: SARIF Suppression Type

Add the SARIF-native suppression struct and field to the Result type.

**Files:**
- Modify: `internal/sarif/sarif.go:45-52`

- [ ] **Step 1: Add SARIFSuppression type and update Result**

In `internal/sarif/sarif.go`, add the new type after the `Result` struct and add the field:

```go
type SARIFSuppression struct {
	Kind          string                 `json:"kind"`
	Justification string                 `json:"justification,omitempty"`
	Properties    map[string]interface{} `json:"properties,omitempty"`
}
```

Add to `Result` struct:
```go
Suppressions []SARIFSuppression `json:"suppressions,omitempty"`
```

- [ ] **Step 2: Verify existing tests still pass**

Run: `go test ./internal/sarif/ -v`
Expected: All existing tests PASS (the new field is omitempty, so serialization is backward-compatible)

- [ ] **Step 3: Commit**

```bash
git add internal/sarif/sarif.go
git commit -m "feat(sarif): add SARIFSuppression type and Suppressions field to Result"
```

---

### Task 2: Suppression Package — Types, Load, Save

Create the core suppression package with the data type and YAML I/O.

**Files:**
- Create: `internal/suppression/suppression.go`
- Create: `internal/suppression/suppression_test.go`

- [ ] **Step 1: Write failing tests for Load and Save**

Create `internal/suppression/suppression_test.go`:

```go
package suppression

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEmpty(t *testing.T) {
	// Load from a directory with no suppressions.yaml returns empty list, no error
	dir := t.TempDir()
	supps, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, supps)
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	entries := []Suppression{
		{
			RuleID:  "S1001",
			Reason:  "too noisy",
			Created: time.Now().UTC().Truncate(time.Second),
			Source:  "cli:user:testuser",
		},
		{
			RuleID:  "G101",
			File:    "internal/auth/tokens.go",
			Reason:  "false positive",
			Created: time.Now().UTC().Truncate(time.Second),
			Source:  "mcp:agent:claude-code",
		},
	}

	require.NoError(t, Save(dir, entries))

	loaded, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, loaded, 2)
	assert.Equal(t, "S1001", loaded[0].RuleID)
	assert.Equal(t, "", loaded[0].File)
	assert.Equal(t, "G101", loaded[1].RuleID)
	assert.Equal(t, "internal/auth/tokens.go", loaded[1].File)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/suppression/ -v`
Expected: FAIL (package does not exist yet)

- [ ] **Step 3: Implement Suppression type, Load, Save**

Create `internal/suppression/suppression.go`:

```go
package suppression

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Suppression represents a single finding suppression entry.
type Suppression struct {
	RuleID  string    `yaml:"rule_id"`
	File    string    `yaml:"file,omitempty"`
	Reason  string    `yaml:"reason"`
	Created time.Time `yaml:"created"`
	Source  string    `yaml:"source"`
}

type suppressionFile struct {
	Suppressions []Suppression `yaml:"suppressions"`
}

const fileName = ".gavel/suppressions.yaml"

// Load reads suppressions from projectDir/.gavel/suppressions.yaml.
// Returns empty list (not error) if the file does not exist.
func Load(projectDir string) ([]Suppression, error) {
	path := filepath.Join(projectDir, fileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var f suppressionFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Suppressions, nil
}

// Save writes suppressions to projectDir/.gavel/suppressions.yaml.
func Save(projectDir string, suppressions []Suppression) error {
	path := filepath.Join(projectDir, fileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f := suppressionFile{Suppressions: suppressions}
	data, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/suppression/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/suppression/
git commit -m "feat(suppression): add Suppression type with Load and Save"
```

---

### Task 3: Suppression Package — NormalizePath and Match

Add path normalization and the matching logic.

**Files:**
- Modify: `internal/suppression/suppression.go`
- Modify: `internal/suppression/suppression_test.go`

- [ ] **Step 1: Write failing tests for NormalizePath and Match**

Add to `internal/suppression/suppression_test.go`:

```go
func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"internal/auth/tokens.go", "internal/auth/tokens.go"},
		{"./internal/auth/tokens.go", "internal/auth/tokens.go"},
		{"internal\\auth\\tokens.go", "internal/auth/tokens.go"},
		{"./foo/../internal/auth/tokens.go", "internal/auth/tokens.go"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, NormalizePath(tt.input), "input: %s", tt.input)
	}
}

func TestMatchGlobal(t *testing.T) {
	supps := []Suppression{
		{RuleID: "S1001", Reason: "noisy"},
	}
	// Global suppression matches any file
	assert.NotNil(t, Match(supps, "S1001", "any/file.go"))
	// Different rule does not match
	assert.Nil(t, Match(supps, "S2002", "any/file.go"))
}

func TestMatchPerFile(t *testing.T) {
	supps := []Suppression{
		{RuleID: "G101", File: "internal/auth/tokens.go", Reason: "false positive"},
	}
	// Exact file matches
	assert.NotNil(t, Match(supps, "G101", "internal/auth/tokens.go"))
	// Different file does not match
	assert.Nil(t, Match(supps, "G101", "internal/other.go"))
	// Different rule does not match
	assert.Nil(t, Match(supps, "S1001", "internal/auth/tokens.go"))
}

func TestMatchNormalizesPath(t *testing.T) {
	supps := []Suppression{
		{RuleID: "G101", File: "internal/auth/tokens.go", Reason: "fp"},
	}
	// Match normalizes input path
	assert.NotNil(t, Match(supps, "G101", "./internal/auth/tokens.go"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/suppression/ -run "TestNormalize|TestMatch" -v`
Expected: FAIL (functions not defined)

- [ ] **Step 3: Implement NormalizePath and Match**

Add to `internal/suppression/suppression.go`:

```go
// NormalizePath converts a file path to canonical form:
// relative, forward slashes, no leading "./", cleaned.
func NormalizePath(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	p = strings.TrimPrefix(p, "./")
	return p
}

// Match returns the first matching suppression for the given ruleID and filePath, or nil.
// Global suppressions (empty File) match any file.
// Per-file suppressions match only when the normalized paths are equal.
func Match(suppressions []Suppression, ruleID string, filePath string) *Suppression {
	normalizedPath := NormalizePath(filePath)
	for i := range suppressions {
		s := &suppressions[i]
		if s.RuleID != ruleID {
			continue
		}
		if s.File == "" {
			return s
		}
		if NormalizePath(s.File) == normalizedPath {
			return s
		}
	}
	return nil
}
```

Add `"strings"` to the imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/suppression/ -run "TestNormalize|TestMatch" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/suppression/
git commit -m "feat(suppression): add NormalizePath and Match"
```

---

### Task 4: Suppression Package — Apply

Add the Apply function that stamps SARIF results with suppression annotations.

**Files:**
- Modify: `internal/suppression/suppression.go`
- Modify: `internal/suppression/suppression_test.go`

- [ ] **Step 1: Write failing tests for Apply**

Add to `internal/suppression/suppression_test.go`:

```go
import (
	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestApplyStampsMatchingResults(t *testing.T) {
	supps := []Suppression{
		{RuleID: "S1001", Reason: "noisy", Source: "cli:user:test", Created: time.Now().UTC()},
	}
	log := &sarif.Log{
		Runs: []sarif.Run{{
			Results: []sarif.Result{
				{RuleID: "S1001", Level: "warning", Message: sarif.Message{Text: "found"}},
				{RuleID: "G101", Level: "error", Message: sarif.Message{Text: "other"}},
			},
		}},
	}

	Apply(supps, log)

	assert.Len(t, log.Runs[0].Results[0].Suppressions, 1)
	assert.Equal(t, "external", log.Runs[0].Results[0].Suppressions[0].Kind)
	assert.Contains(t, log.Runs[0].Results[0].Suppressions[0].Justification, "noisy")
	assert.Empty(t, log.Runs[0].Results[1].Suppressions)
}

func TestApplyClearsStaleSuppressions(t *testing.T) {
	// Result already has a suppression from a previous apply
	log := &sarif.Log{
		Runs: []sarif.Run{{
			Results: []sarif.Result{
				{
					RuleID:  "S1001",
					Level:   "warning",
					Message: sarif.Message{Text: "found"},
					Suppressions: []sarif.SARIFSuppression{
						{Kind: "external", Justification: "old reason"},
					},
				},
			},
		}},
	}

	// Apply with empty suppression list clears the stale annotation
	Apply(nil, log)
	assert.Empty(t, log.Runs[0].Results[0].Suppressions)
}

func TestApplyPerFile(t *testing.T) {
	supps := []Suppression{
		{RuleID: "G101", File: "auth/tokens.go", Reason: "fp", Source: "mcp:agent:test", Created: time.Now().UTC()},
	}
	log := &sarif.Log{
		Runs: []sarif.Run{{
			Results: []sarif.Result{
				{
					RuleID:  "G101",
					Level:   "warning",
					Message: sarif.Message{Text: "cred"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "auth/tokens.go"},
						},
					}},
				},
				{
					RuleID:  "G101",
					Level:   "warning",
					Message: sarif.Message{Text: "cred2"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "other/file.go"},
						},
					}},
				},
			},
		}},
	}

	Apply(supps, log)
	assert.Len(t, log.Runs[0].Results[0].Suppressions, 1)
	assert.Empty(t, log.Runs[0].Results[1].Suppressions)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/suppression/ -run "TestApply" -v`
Expected: FAIL (Apply not defined)

- [ ] **Step 3: Implement Apply**

Add to `internal/suppression/suppression.go`:

```go
import "github.com/chris-regnier/gavel/internal/sarif"

// Apply clears existing suppression annotations on all results, then stamps
// matching results with SARIF-native suppression entries. This clear-then-apply
// approach ensures removed suppressions take effect correctly.
func Apply(suppressions []Suppression, log *sarif.Log) {
	for i := range log.Runs {
		for j := range log.Runs[i].Results {
			r := &log.Runs[i].Results[j]
			// Clear existing suppressions
			r.Suppressions = nil

			// Extract file path from first location
			filePath := ""
			if len(r.Locations) > 0 {
				filePath = r.Locations[0].PhysicalLocation.ArtifactLocation.URI
			}

			s := Match(suppressions, r.RuleID, filePath)
			if s == nil {
				continue
			}

			r.Suppressions = []sarif.SARIFSuppression{
				{
					Kind:          "external",
					Justification: s.Reason,
					Properties: map[string]interface{}{
						"gavel/source":  s.Source,
						"gavel/created": s.Created.Format(time.RFC3339),
					},
				},
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/suppression/ -run "TestApply" -v`
Expected: PASS

- [ ] **Step 5: Run all suppression tests**

Run: `go test ./internal/suppression/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/suppression/
git commit -m "feat(suppression): add Apply to stamp SARIF results with suppression annotations"
```

---

### Task 5: Rego Policy Update

Update the default Rego policy to filter suppressed results.

**Files:**
- Modify: `internal/evaluator/default.rego`

- [ ] **Step 1: Write the updated Rego policy**

Replace the contents of `internal/evaluator/default.rego`:

```rego
package gavel.gate

import rego.v1

default decision := "review"

_suppressed(result) if {
	suppressions := object.get(result, "suppressions", [])
	count(suppressions) > 0
}

unsuppressed_results contains result if {
	some result in input.runs[0].results
	not _suppressed(result)
}

decision := "reject" if {
	some result in unsuppressed_results
	result.level == "error"
	result.properties["gavel/confidence"] > 0.8
}

decision := "merge" if {
	count(unsuppressed_results) == 0
}
```

- [ ] **Step 2: Run existing evaluator tests**

Run: `go test ./internal/evaluator/ -v`
Expected: PASS (existing tests should still pass because existing test SARIF has no suppressions field, so all results are unsuppressed)

- [ ] **Step 3: Commit**

```bash
git add internal/evaluator/default.rego
git commit -m "feat(evaluator): update Rego policy to filter suppressed results"
```

---

### Task 6: Evaluator Go-Side Filtering

Update the evaluator to exclude suppressed results from RelevantFindings and update the verdict reason string.

**Files:**
- Modify: `internal/evaluator/evaluator.go:98-124`
- Create: `internal/evaluator/evaluator_suppression_test.go`

- [ ] **Step 1: Write failing test for suppression filtering in evaluator**

Create `internal/evaluator/evaluator_suppression_test.go`:

```go
package evaluator

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateSuppressedResultsExcluded(t *testing.T) {
	ctx := context.Background()
	eval, err := NewEvaluator(ctx, "")
	require.NoError(t, err)

	log := &sarif.Log{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Runs: []sarif.Run{{
			Tool: sarif.Tool{Driver: sarif.Driver{Name: "gavel", Version: "test"}},
			Results: []sarif.Result{
				{
					RuleID:  "S1001",
					Level:   "error",
					Message: sarif.Message{Text: "suppressed error"},
					Properties: map[string]interface{}{
						"gavel/confidence": 0.9,
					},
					Suppressions: []sarif.SARIFSuppression{
						{Kind: "external", Justification: "intentional"},
					},
				},
			},
		}},
	}

	verdict, err := eval.Evaluate(ctx, log)
	require.NoError(t, err)
	// Suppressed high-confidence error should not trigger reject
	assert.Equal(t, "merge", verdict.Decision)
	assert.Empty(t, verdict.RelevantFindings)
	assert.Contains(t, verdict.Reason, "1 suppressed")
}

func TestEvaluateMixedSuppressedAndUnsuppressed(t *testing.T) {
	ctx := context.Background()
	eval, err := NewEvaluator(ctx, "")
	require.NoError(t, err)

	log := &sarif.Log{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Runs: []sarif.Run{{
			Tool: sarif.Tool{Driver: sarif.Driver{Name: "gavel", Version: "test"}},
			Results: []sarif.Result{
				{
					RuleID:  "S1001",
					Level:   "warning",
					Message: sarif.Message{Text: "active warning"},
				},
				{
					RuleID:  "S1002",
					Level:   "error",
					Message: sarif.Message{Text: "suppressed"},
					Suppressions: []sarif.SARIFSuppression{
						{Kind: "external", Justification: "fp"},
					},
				},
			},
		}},
	}

	verdict, err := eval.Evaluate(ctx, log)
	require.NoError(t, err)
	assert.Equal(t, "review", verdict.Decision)
	// Only the unsuppressed warning should be relevant
	assert.Len(t, verdict.RelevantFindings, 1)
	assert.Equal(t, "S1001", verdict.RelevantFindings[0].RuleID)
	assert.Contains(t, verdict.Reason, "1 suppressed")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/evaluator/ -run "TestEvaluateSuppress" -v`
Expected: FAIL (Go-side filtering not implemented yet, Rego may pass but verdict reason won't match)

- [ ] **Step 3: Update evaluator.go to filter suppressed results**

In `internal/evaluator/evaluator.go`, modify the section at lines 98-124:

```go
	var relevant []sarif.Result
	suppressedCount := 0
	if len(log.Runs) > 0 {
		for _, r := range log.Runs[0].Results {
			if len(r.Suppressions) > 0 {
				suppressedCount++
				continue
			}
			if decision == "reject" && r.Level == "error" {
				relevant = append(relevant, r)
			} else if decision == "review" && (r.Level == "warning" || r.Level == "error") {
				relevant = append(relevant, r)
			}
		}
	}

	resultCount := 0
	if len(log.Runs) > 0 {
		resultCount = len(log.Runs[0].Results)
	}
	unsuppressedCount := resultCount - suppressedCount

	span.SetAttributes(
		attribute.String("gavel.decision", decision),
		attribute.Int("gavel.finding_count", resultCount),
		attribute.Int("gavel.relevant_count", len(relevant)),
		attribute.Int("gavel.suppressed_count", suppressedCount),
	)

	reason := fmt.Sprintf("Decision: %s based on %d findings", decision, unsuppressedCount)
	if suppressedCount > 0 {
		reason += fmt.Sprintf(", %d suppressed", suppressedCount)
	}

	return &store.Verdict{
		Decision:         decision,
		Reason:           reason,
		RelevantFindings: relevant,
	}, nil
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/evaluator/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/evaluator/
git commit -m "feat(evaluator): filter suppressed results from RelevantFindings and verdict reason"
```

---

### Task 7: Pipeline Integration — analyze and judge

Wire suppression loading and application into the analyze and judge commands.

**Files:**
- Modify: `cmd/gavel/analyze.go:201-210`
- Modify: `cmd/gavel/judge.go:97-113`

- [ ] **Step 1: Update analyze.go to apply suppressions after SARIF assembly**

In `cmd/gavel/analyze.go`, after line 201 (`sarifLog := sarif.Assemble(...)`) and before line 203 (`fs := store.NewFileStore(...)`), add:

```go
	// Apply suppressions
	// flagPolicyDir defaults to ".gavel" — Load expects the project root (parent dir)
	// and appends ".gavel/suppressions.yaml" internally, so use filepath.Dir(flagPolicyDir).
	suppressionRoot := filepath.Dir(flagPolicyDir)
	supps, err := suppression.Load(suppressionRoot)
	if err != nil {
		slog.Warn("failed to load suppressions", "err", err)
	}
	suppression.Apply(supps, sarifLog)

	suppressedCount := 0
	for _, run := range sarifLog.Runs {
		for _, r := range run.Results {
			if len(r.Suppressions) > 0 {
				suppressedCount++
			}
		}
	}
```

Add import: `"github.com/chris-regnier/gavel/internal/suppression"`

Update the summary output JSON to include `"suppressed": suppressedCount`.

**Important:** `flagPolicyDir` defaults to `".gavel"` (the `.gavel/` directory itself). Since `suppression.Load(projectDir)` appends `.gavel/suppressions.yaml` internally, you must pass the **project root**, not `.gavel/`. Use `filepath.Dir(flagPolicyDir)` which resolves `".gavel"` → `"."` (the project root).

- [ ] **Step 2: Update judge.go to re-apply suppressions before evaluation**

In `cmd/gavel/judge.go`, **after the ReadSARIF error check block** (after line 103, NOT between the ReadSARIF call and its error check), and before line 105 (`eval, err := evaluator.NewEvaluator(...)`), add:

```go
	// Re-apply current suppressions (clears stale, applies new)
	// flagJudgePolicyDir defaults to ".gavel" — use filepath.Dir to get project root
	suppressionRoot := filepath.Dir(flagJudgePolicyDir)
	supps, err := suppression.Load(suppressionRoot)
	if err != nil {
		slog.Warn("failed to load suppressions", "err", err)
	}
	suppression.Apply(supps, sarifLog)
```

Add import: `"github.com/chris-regnier/gavel/internal/suppression"`

**Important:** `flagJudgePolicyDir` defaults to `".gavel"`. Use `filepath.Dir(flagJudgePolicyDir)` to get `"."` (project root). Do NOT use `flagJudgeOutput` (which is `.gavel/results/`).

- [ ] **Step 3: Verify existing tests still pass**

Run: `go test ./cmd/gavel/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/gavel/analyze.go cmd/gavel/judge.go
git commit -m "feat: wire suppression loading into analyze and judge pipelines"
```

---

### Task 8: CLI Commands — suppress, unsuppress, suppressions

Add the three CLI commands for managing suppressions.

**Files:**
- Create: `cmd/gavel/suppress.go`
- Create: `cmd/gavel/suppress_test.go`

- [ ] **Step 1: Write tests for CLI commands**

Create `cmd/gavel/suppress_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chris-regnier/gavel/internal/suppression"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuppressCreatesEntry(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	err := runSuppress(dir, "S1001", "", "too noisy")
	require.NoError(t, err)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	require.Len(t, supps, 1)
	assert.Equal(t, "S1001", supps[0].RuleID)
	assert.Equal(t, "too noisy", supps[0].Reason)
	assert.Equal(t, "", supps[0].File)
	assert.Contains(t, supps[0].Source, "cli:user:")
}

func TestSuppressPerFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	err := runSuppress(dir, "G101", "internal/auth/tokens.go", "false positive")
	require.NoError(t, err)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	require.Len(t, supps, 1)
	assert.Equal(t, "internal/auth/tokens.go", supps[0].File)
}

func TestSuppressDuplicateUpdates(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	require.NoError(t, runSuppress(dir, "S1001", "", "first reason"))
	require.NoError(t, runSuppress(dir, "S1001", "", "updated reason"))

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	require.Len(t, supps, 1)
	assert.Equal(t, "updated reason", supps[0].Reason)
}

func TestUnsuppress(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	require.NoError(t, runSuppress(dir, "S1001", "", "noisy"))
	err := runUnsuppress(dir, "S1001", "")
	require.NoError(t, err)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	assert.Empty(t, supps)
}

func TestUnsuppressNotFound(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	err := runUnsuppress(dir, "NONEXISTENT", "")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/gavel/ -run "TestSuppress|TestUnsuppress" -v`
Expected: FAIL

- [ ] **Step 3: Implement suppress.go**

Create `cmd/gavel/suppress.go`:

```go
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/suppression"
)

var (
	flagSuppressFile   string
	flagSuppressReason string
	flagSuppressSource string
)

func init() {
	suppressCmd.Flags().StringVar(&flagSuppressFile, "file", "", "Restrict suppression to this file path")
	suppressCmd.Flags().StringVar(&flagSuppressReason, "reason", "", "Reason for suppression (required)")
	suppressCmd.MarkFlagRequired("reason")

	unsuppressCmd.Flags().StringVar(&flagSuppressFile, "file", "", "Remove file-specific suppression only")

	suppressionsCmd.Flags().StringVar(&flagSuppressSource, "source", "", "Filter by source prefix (e.g., \"mcp:\")")

	rootCmd.AddCommand(suppressCmd)
	rootCmd.AddCommand(unsuppressCmd)
	rootCmd.AddCommand(suppressionsCmd)
}

var suppressCmd = &cobra.Command{
	Use:   "suppress <rule-id>",
	Short: "Suppress a finding rule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		return runSuppress(dir, args[0], flagSuppressFile, flagSuppressReason)
	},
}

var unsuppressCmd = &cobra.Command{
	Use:   "unsuppress <rule-id>",
	Short: "Remove a finding suppression",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		return runUnsuppress(dir, args[0], flagSuppressFile)
	},
}

var suppressionsCmd = &cobra.Command{
	Use:   "suppressions",
	Short: "List active suppressions",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		return runListSuppressions(dir, flagSuppressSource)
	},
}

func runSuppress(projectDir, ruleID, file, reason string) error {
	supps, err := suppression.Load(projectDir)
	if err != nil {
		return fmt.Errorf("loading suppressions: %w", err)
	}

	normalizedFile := ""
	if file != "" {
		normalizedFile = suppression.NormalizePath(file)
	}

	source := "cli:user:" + currentUsername()

	// Check for existing entry with same rule_id + file
	for i := range supps {
		if supps[i].RuleID == ruleID && suppression.NormalizePath(supps[i].File) == normalizedFile {
			supps[i].Reason = reason
			supps[i].Created = time.Now().UTC().Truncate(time.Second)
			supps[i].Source = source
			if err := suppression.Save(projectDir, supps); err != nil {
				return fmt.Errorf("saving suppressions: %w", err)
			}
			slog.Info("updated suppression", "rule_id", ruleID, "file", normalizedFile)
			return nil
		}
	}

	// Add new entry
	entry := suppression.Suppression{
		RuleID:  ruleID,
		File:    normalizedFile,
		Reason:  reason,
		Created: time.Now().UTC().Truncate(time.Second),
		Source:  source,
	}
	supps = append(supps, entry)

	if err := suppression.Save(projectDir, supps); err != nil {
		return fmt.Errorf("saving suppressions: %w", err)
	}
	slog.Info("added suppression", "rule_id", ruleID, "file", normalizedFile)
	return nil
}

func runUnsuppress(projectDir, ruleID, file string) error {
	supps, err := suppression.Load(projectDir)
	if err != nil {
		return fmt.Errorf("loading suppressions: %w", err)
	}

	normalizedFile := ""
	if file != "" {
		normalizedFile = suppression.NormalizePath(file)
	}

	for i := range supps {
		if supps[i].RuleID == ruleID && suppression.NormalizePath(supps[i].File) == normalizedFile {
			supps = append(supps[:i], supps[i+1:]...)
			if err := suppression.Save(projectDir, supps); err != nil {
				return fmt.Errorf("saving suppressions: %w", err)
			}
			slog.Info("removed suppression", "rule_id", ruleID, "file", normalizedFile)
			return nil
		}
	}

	return fmt.Errorf("no suppression found for rule %s (file: %q)", ruleID, file)
}

func runListSuppressions(projectDir, sourceFilter string) error {
	supps, err := suppression.Load(projectDir)
	if err != nil {
		return fmt.Errorf("loading suppressions: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "RULE\tFILE\tSOURCE\tREASON")

	for _, s := range supps {
		if sourceFilter != "" && !strings.HasPrefix(s.Source, sourceFilter) {
			continue
		}
		file := "(all)"
		if s.File != "" {
			file = s.File
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.RuleID, file, s.Source, s.Reason)
	}
	w.Flush()
	return nil
}

func currentUsername() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/gavel/ -run "TestSuppress|TestUnsuppress" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/gavel/suppress.go cmd/gavel/suppress_test.go
git commit -m "feat: add suppress, unsuppress, and suppressions CLI commands"
```

---

### Task 9: MCP Tools

Add the three suppression tools to the MCP server.

**Files:**
- Modify: `internal/mcp/server.go:36-67`
- Modify: `internal/mcp/server_test.go`

- [ ] **Step 1: Write failing tests for MCP suppression tools**

Add to `internal/mcp/server_test.go`, following the existing test infrastructure pattern. The tests use `mcptest.NewUnstartedServer`, `registerAll`, and `callTool(ctx, client, name, args)`:

```go
func setupTestServerWithDir(t *testing.T, rootDir string) *mcptest.Server {
	t.Helper()

	cfg := testConfig()
	fs := store.NewFileStore(filepath.Join(rootDir, ".gavel", "results"))
	h := newTestHandlers(t, cfg, fs, rootDir)

	testServer := mcptest.NewUnstartedServer(t)
	registerAll(testServer, h)

	if err := testServer.Start(context.Background()); err != nil {
		t.Fatalf("starting test server: %v", err)
	}
	t.Cleanup(testServer.Close)

	return testServer
}

func TestSuppressFindingTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	ts := setupTestServerWithDir(t, dir)
	client := ts.Client()
	ctx := context.Background()

	result, err := callTool(ctx, client, "suppress_finding", map[string]any{
		"rule_id": "S1001",
		"reason":  "too noisy",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	require.Len(t, supps, 1)
	assert.Equal(t, "S1001", supps[0].RuleID)
	assert.Contains(t, supps[0].Source, "mcp:agent:")
}

func TestListSuppressionsTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	require.NoError(t, suppression.Save(dir, []suppression.Suppression{
		{RuleID: "S1001", Reason: "test", Source: "cli:user:test", Created: time.Now().UTC()},
	}))

	ts := setupTestServerWithDir(t, dir)
	client := ts.Client()
	ctx := context.Background()

	result, err := callTool(ctx, client, "list_suppressions", nil)
	require.NoError(t, err)
	// Result text should contain the rule ID
	assert.NotNil(t, result)
}

func TestUnsuppressFindingTool(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".gavel"), 0o755))

	require.NoError(t, suppression.Save(dir, []suppression.Suppression{
		{RuleID: "S1001", Reason: "test", Source: "cli:user:test", Created: time.Now().UTC()},
	}))

	ts := setupTestServerWithDir(t, dir)
	client := ts.Client()
	ctx := context.Background()

	result, err := callTool(ctx, client, "unsuppress_finding", map[string]any{
		"rule_id": "S1001",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)

	supps, err := suppression.Load(dir)
	require.NoError(t, err)
	assert.Empty(t, supps)
}
```

**Important:** The `registerAll` function in `server_test.go` must be updated to include the new suppression tools (same pattern as existing tools). Add these lines:

```go
ts.AddTool(suppressFindingTool(), h.handleSuppressFinding)
ts.AddTool(listSuppressionsTool(), h.handleListSuppressions)
ts.AddTool(unsuppressFindingTool(), h.handleUnsuppressFinding)
```

Add imports: `"github.com/chris-regnier/gavel/internal/suppression"` and `"time"`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mcp/ -run "TestSuppress|TestList|TestUnsuppress" -v`
Expected: FAIL

- [ ] **Step 3: Add suppression tool definitions and handlers to server.go**

Add tool definitions:

```go
func suppressFindingTool() mcp.Tool {
	return mcp.NewTool("suppress_finding",
		mcp.WithDescription("Suppress a finding rule. Adds an entry to .gavel/suppressions.yaml so matching findings are excluded from evaluation."),
		mcp.WithString("rule_id",
			mcp.Description("Rule ID to suppress (e.g., S1001)"),
			mcp.Required(),
		),
		mcp.WithString("file",
			mcp.Description("Restrict suppression to this file path (omit for global)"),
		),
		mcp.WithString("reason",
			mcp.Description("Justification for suppression"),
			mcp.Required(),
		),
	)
}

func listSuppressionsTool() mcp.Tool {
	return mcp.NewTool("list_suppressions",
		mcp.WithDescription("List all active finding suppressions from .gavel/suppressions.yaml."),
	)
}

func unsuppressFindingTool() mcp.Tool {
	return mcp.NewTool("unsuppress_finding",
		mcp.WithDescription("Remove a finding suppression entry."),
		mcp.WithString("rule_id",
			mcp.Description("Rule ID to unsuppress"),
			mcp.Required(),
		),
		mcp.WithString("file",
			mcp.Description("Remove file-specific suppression only (omit for global)"),
		),
	)
}
```

Register in `NewMCPServer`:

```go
s.AddTool(suppressFindingTool(), h.handleSuppressFinding)
s.AddTool(listSuppressionsTool(), h.handleListSuppressions)
s.AddTool(unsuppressFindingTool(), h.handleUnsuppressFinding)
```

Add handlers — these delegate to the same logic as the CLI commands. The handlers need the project root directory, which is available via `h.cfg.RootDir`. Source is set to `mcp:agent:gavel-mcp` (or a configurable name).

```go
func (h *handlers) handleSuppressFinding(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ruleID := request.GetString("rule_id", "")
	if ruleID == "" {
		return mcp.NewToolResultError("rule_id is required"), nil
	}
	reason := request.GetString("reason", "")
	if reason == "" {
		return mcp.NewToolResultError("reason is required"), nil
	}
	file := request.GetString("file", "")

	rootDir := h.cfg.RootDir
	if rootDir == "" {
		var err error
		rootDir, err = os.Getwd()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("getting working directory: %v", err)), nil
		}
	}

	supps, err := suppression.Load(rootDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("loading suppressions: %v", err)), nil
	}

	normalizedFile := ""
	if file != "" {
		normalizedFile = suppression.NormalizePath(file)
	}

	source := "mcp:agent:gavel-mcp"
	now := time.Now().UTC().Truncate(time.Second)

	// Update existing or add new
	found := false
	for i := range supps {
		if supps[i].RuleID == ruleID && suppression.NormalizePath(supps[i].File) == normalizedFile {
			supps[i].Reason = reason
			supps[i].Created = now
			supps[i].Source = source
			found = true
			break
		}
	}
	if !found {
		supps = append(supps, suppression.Suppression{
			RuleID:  ruleID,
			File:    normalizedFile,
			Reason:  reason,
			Created: now,
			Source:  source,
		})
	}

	if err := suppression.Save(rootDir, supps); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("saving suppressions: %v", err)), nil
	}

	out, _ := json.MarshalIndent(map[string]interface{}{
		"status":  "suppressed",
		"rule_id": ruleID,
		"file":    normalizedFile,
		"reason":  reason,
		"source":  source,
	}, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (h *handlers) handleListSuppressions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rootDir := h.cfg.RootDir
	if rootDir == "" {
		var err error
		rootDir, err = os.Getwd()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("getting working directory: %v", err)), nil
		}
	}

	supps, err := suppression.Load(rootDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("loading suppressions: %v", err)), nil
	}

	out, _ := json.MarshalIndent(supps, "", "  ")
	return mcp.NewToolResultText(string(out)), nil
}

func (h *handlers) handleUnsuppressFinding(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ruleID := request.GetString("rule_id", "")
	if ruleID == "" {
		return mcp.NewToolResultError("rule_id is required"), nil
	}
	file := request.GetString("file", "")

	rootDir := h.cfg.RootDir
	if rootDir == "" {
		var err error
		rootDir, err = os.Getwd()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("getting working directory: %v", err)), nil
		}
	}

	supps, err := suppression.Load(rootDir)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("loading suppressions: %v", err)), nil
	}

	normalizedFile := ""
	if file != "" {
		normalizedFile = suppression.NormalizePath(file)
	}

	for i := range supps {
		if supps[i].RuleID == ruleID && suppression.NormalizePath(supps[i].File) == normalizedFile {
			supps = append(supps[:i], supps[i+1:]...)
			if err := suppression.Save(rootDir, supps); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("saving suppressions: %v", err)), nil
			}
			out, _ := json.MarshalIndent(map[string]interface{}{
				"status":  "unsuppressed",
				"rule_id": ruleID,
				"file":    normalizedFile,
			}, "", "  ")
			return mcp.NewToolResultText(string(out)), nil
		}
	}

	return mcp.NewToolResultError(fmt.Sprintf("no suppression found for rule %s (file: %q)", ruleID, file)), nil
}
```

Add imports: `"github.com/chris-regnier/gavel/internal/suppression"` and `"time"`.

- [ ] **Step 4: Update MCP analyze and judge handlers to apply suppressions**

In `handleAnalyzeFile` and `handleAnalyzeDirectory`, after `sarif.Assemble()` and before `WriteSARIF()`, add suppression application (same pattern as Task 7 step 1).

In `handleJudge`, after `ReadSARIF()` and before `eval.Evaluate()`, add suppression re-application (same pattern as Task 7 step 2).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/mcp/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/
git commit -m "feat(mcp): add suppress_finding, list_suppressions, and unsuppress_finding tools"
```

---

### Task 10: Full Integration Verification

Run the full test suite and verify everything works together.

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All PASS

- [ ] **Step 2: Run linter**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 3: Build**

Run: `task build`
Expected: Successful build

- [ ] **Step 4: Manual smoke test**

```bash
# Create a test suppression
./dist/gavel suppress S1001 --reason "testing suppression"

# List it
./dist/gavel suppressions

# Remove it
./dist/gavel unsuppress S1001
```

- [ ] **Step 5: Commit any fixes from integration testing**

Only if needed. Otherwise, no commit.
