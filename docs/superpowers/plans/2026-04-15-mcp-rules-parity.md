# MCP Rules Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `analyze_file`, `analyze_directory`, and the scoped-diff MCP flow apply regex and AST rules (both embedded defaults and user/project overrides) in addition to LLM policies, matching CLI behavior as specified in [#105](https://github.com/chris-regnier/gavel/issues/105).

**Architecture:** Add `Rules []rules.Rule` to `mcp.ServerConfig`, thread it onto the `handlers` struct, construct a `TieredAnalyzer` with `WithInstantPatterns(h.rules)` in `runAnalysis` and the scoped-diff flow, and include rule descriptors in SARIF output. Load rules once at startup in `cmd/gavel/mcp.go` (mirroring the CLI) behind a new `--rules-dir` flag. Tests use a package-local mock `BAMLClient` so the LLM tier succeeds deterministically without changing production error semantics.

**Tech Stack:** Go, `internal/rules` (tiered rule loading), `internal/analyzer` (`TieredAnalyzer`, `WithInstantPatterns`, `BAMLClient`), `internal/sarif` (descriptors), `github.com/mark3labs/mcp-go` (MCP server), `mcptest` (MCP test harness), `testify`.

---

## File Structure

**Files to modify:**
- `internal/mcp/server.go` — Add `Rules` to `ServerConfig`, `rules` to `handlers`, wire into `runAnalysis` (line ~986), fix scoped-diff flow (line ~602), replace `buildRules` with `buildDescriptors`.
- `internal/mcp/server_test.go` — Update `newTestHandlers` to accept rules and an optional client; add a local mock BAML client; add new tests for rule-firing.
- `cmd/gavel/mcp.go` — Add `--rules-dir` flag, load rules via `rules.LoadRules`, pass into `ServerConfig`.

**No new files** — all changes fit into existing files with clear responsibilities. Production error semantics are preserved.

---

## Task 1: Thread `Rules` and a test-only client override through `handlers`

**Files:**
- Modify: `internal/mcp/server.go` (imports, `ServerConfig`, `NewMCPServer` body, `handlers` struct)
- Modify: `internal/mcp/server_test.go` (`newTestHandlers`, add local mock client)

No production behavior change — this is pure plumbing. The test-only client override enables Task 2 onwards to assert rule-firing without relying on the real BAML client.

- [ ] **Step 1: Add `internal/rules` import to `internal/mcp/server.go`**

Open `internal/mcp/server.go` and replace the import block at lines 5-25 with:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
	"github.com/chris-regnier/gavel/internal/suppression"
)
```

- [ ] **Step 2: Add `Rules` to `ServerConfig`**

Replace the `ServerConfig` struct at `internal/mcp/server.go:29-35` with:

```go
// ServerConfig holds configuration for the MCP server.
type ServerConfig struct {
	Config  *config.Config
	Store   store.Store
	RegoDir string        // Directory for custom Rego policies (empty = default embedded policy)
	RootDir string        // Root directory for path validation (empty = cwd)
	Rules   []rules.Rule  // Loaded regex/AST rules for the instant analysis tier (nil = use embedded defaults)
}
```

- [ ] **Step 3: Add `rules` to `handlers` and initialize in `NewMCPServer`**

Replace the `handlers` struct at `internal/mcp/server.go:75-79` with:

```go
// handlers holds the server config and implements all tool/resource/prompt handlers.
type handlers struct {
	cfg    ServerConfig
	client analyzer.BAMLClient
	rules  []rules.Rule
}
```

Replace the `h := &handlers{...}` initialization at `internal/mcp/server.go:47-50` (inside `NewMCPServer`) with:

```go
	h := &handlers{
		cfg:    cfg,
		client: analyzer.NewBAMLLiveClient(cfg.Config.Provider),
		rules:  cfg.Rules,
	}
```

- [ ] **Step 4: Add a local mock BAML client to the MCP test file**

In `internal/mcp/server_test.go`, add the `rules` package import. The current imports (lines 3-22) should become:

```go
import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
	"github.com/chris-regnier/gavel/internal/suppression"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

Append a local mock BAML client just after the `testStore` helper (around line 62, immediately before `newTestHandlers`):

```go
// mockBAMLClient is a deterministic BAMLClient used in tests so the LLM
// tier succeeds (returning a configurable slice of findings) without
// making real network calls.
type mockBAMLClient struct {
	findings []analyzer.Finding
	err      error
}

func (m *mockBAMLClient) AnalyzeCode(_ context.Context, _, _, _, _ string) ([]analyzer.Finding, error) {
	return m.findings, m.err
}
```

- [ ] **Step 5: Update `newTestHandlers` to accept a client and rules**

Replace `newTestHandlers` at `internal/mcp/server_test.go:63-75` with:

```go
// testHandlerOpts configures optional behavior for newTestHandlers.
// Both fields are zero-valued by default, preserving existing call-site
// behavior (live BAML client, no rules).
type testHandlerOpts struct {
	client analyzer.BAMLClient
	rules  []rules.Rule
}

// newTestHandlers creates handlers with the same wiring as NewMCPServer,
// so tests stay aligned with production registration. Pass a
// testHandlerOpts to inject a mock BAML client or preloaded rules.
func newTestHandlers(t *testing.T, cfg *config.Config, fs store.Store, rootDir string, opts ...testHandlerOpts) *handlers {
	t.Helper()
	var o testHandlerOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	client := o.client
	if client == nil {
		client = analyzer.NewBAMLLiveClient(cfg.Provider)
	}
	return &handlers{
		cfg: ServerConfig{
			Config:  cfg,
			Store:   fs,
			RootDir: rootDir,
			Rules:   o.rules,
		},
		client: client,
		rules:  o.rules,
	}
}
```

Existing call sites use three positional args (`cfg, fs, rootDir`) — they continue to compile because `opts` is variadic and defaults to the zero `testHandlerOpts`.

- [ ] **Step 6: Verify compilation**

Run: `go build ./...`
Expected: clean build, no errors.

- [ ] **Step 7: Verify existing tests still pass**

Run: `go test ./internal/mcp/... -count=1`
Expected: all existing tests pass unchanged.

- [ ] **Step 8: Commit**

```bash
git add internal/mcp/server.go internal/mcp/server_test.go
git commit -m "mcp: plumb rules and test-only client override through handlers (#105)

Pure plumbing for #105. Production behavior is unchanged — the new Rules
field defaults to nil and the BAML client is still built from the
configured provider. Later tasks wire these rules into analysis.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Failing test for rule-firing via `analyze_file`

**Files:**
- Modify: `internal/mcp/server_test.go` (append new test)

- [ ] **Step 1: Write the failing test**

Append to `internal/mcp/server_test.go` just before the `// --- analyze_diff tests ---` comment (around line 914):

```go
// TestAnalyzeFileTool_InstantRulesFire verifies that regex rules from
// ServerConfig.Rules fire via handleAnalyzeFile alongside the LLM tier.
// Regression test for #105.
func TestAnalyzeFileTool_InstantRulesFire(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "creds.go")
	// Matches built-in rule S2068 (hardcoded-credentials).
	require.NoError(t, os.WriteFile(testFile, []byte(`package main

var password = "hunter2hunter2"
`), 0644))

	cfg := testConfig()
	fs := testStore(t)

	defaultRules, err := rules.DefaultRules()
	require.NoError(t, err)

	h := newTestHandlers(t, cfg, fs, tmpDir, testHandlerOpts{
		client: &mockBAMLClient{},
		rules:  defaultRules,
	})

	ctx := context.Background()
	req := mcpgo.CallToolRequest{}
	req.Params.Name = "analyze_file"
	req.Params.Arguments = map[string]any{"path": testFile}

	result, err := h.handleAnalyzeFile(ctx, req)
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %+v", result)

	text := result.Content[0].(mcpgo.TextContent).Text
	var summary map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text), &summary))

	id, ok := summary["id"].(string)
	require.True(t, ok, "summary missing id: %s", text)

	sarifLog, err := fs.ReadSARIF(ctx, id)
	require.NoError(t, err)
	require.Len(t, sarifLog.Runs, 1)

	var foundS2068 bool
	for _, r := range sarifLog.Runs[0].Results {
		if r.RuleID == "S2068" {
			foundS2068 = true
			break
		}
	}
	assert.True(t, foundS2068, "expected S2068 finding, got results: %+v", sarifLog.Runs[0].Results)

	// Rule descriptor must appear in tool.driver.rules.
	var descriptorS2068 bool
	for _, d := range sarifLog.Runs[0].Tool.Driver.Rules {
		if d.ID == "S2068" {
			descriptorS2068 = true
			break
		}
	}
	assert.True(t, descriptorS2068, "expected S2068 rule descriptor in tool.driver.rules")
}
```

- [ ] **Step 2: Run and confirm the test fails**

Run: `go test ./internal/mcp/... -run TestAnalyzeFileTool_InstantRulesFire -v -count=1`
Expected: FAIL. `runAnalysis` currently uses `analyzer.NewAnalyzer(h.client)` (LLM-only via mock, which returns zero findings), so no S2068 finding appears. The descriptor assertion also fails because `buildRules` emits policy descriptors only.

- [ ] **Step 3: Commit the failing test**

```bash
git add internal/mcp/server_test.go
git commit -m "test(mcp): failing test for rule-firing via analyze_file (#105)

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Switch `runAnalysis` to `TieredAnalyzer`

**Files:**
- Modify: `internal/mcp/server.go:986-1004` (`runAnalysis` function)

No change to error semantics — callers of `runAnalysis` still return early on non-nil error (matching CLI).

- [ ] **Step 1: Replace `runAnalysis` body**

Replace the body of `runAnalysis` at `internal/mcp/server.go:986-1004`:

```go
func (h *handlers) runAnalysis(ctx context.Context, artifacts []input.Artifact, persona string) ([]sarif.Result, error) {
	personaPrompt, err := analyzer.GetPersonaPrompt(ctx, persona)
	if err != nil {
		return nil, fmt.Errorf("loading persona %s: %w", persona, err)
	}

	// Append applicability filter if enabled (default).
	// Prose personas get a writing-appropriate filter; code personas get the original.
	if h.cfg.Config.StrictFilter {
		if analyzer.IsProsePersona(persona) {
			personaPrompt += analyzer.ProseApplicabilityFilterPrompt
		} else {
			personaPrompt += analyzer.ApplicabilityFilterPrompt
		}
	}

	opts := []analyzer.TieredAnalyzerOption{}
	if len(h.rules) > 0 {
		opts = append(opts, analyzer.WithInstantPatterns(h.rules))
	}

	ta := analyzer.NewTieredAnalyzer(h.client, opts...)
	return ta.Analyze(ctx, artifacts, h.cfg.Config.Policies, personaPrompt)
}
```

- [ ] **Step 2: Run the Task 2 test**

Run: `go test ./internal/mcp/... -run TestAnalyzeFileTool_InstantRulesFire -v -count=1`
Expected: the S2068 **finding** assertion passes, but the **descriptor** assertion still fails (Task 5 fixes descriptors). Confirm the output shows only the descriptor assertion failing.

- [ ] **Step 3: Run the full MCP test suite**

Run: `go test ./internal/mcp/... -count=1`
Expected: all existing tests pass. The only failure is `TestAnalyzeFileTool_InstantRulesFire` on the descriptor assertion.

Known-sensitive test: `TestAnalyzeFileTool_RealFile` (line 790) — uses a trivial Go file with no rule matches. `h.rules` is nil, so `WithInstantPatterns` isn't applied and the `TieredAnalyzer` uses its embedded default patterns. The trivial file still matches nothing, and the live BAML client still fails to reach the configured ollama endpoint, so `runAnalysis` returns `(nil_or_empty, err)` and the handler returns an error (unchanged behavior from the test's perspective).

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/server.go
git commit -m "mcp: use TieredAnalyzer in runAnalysis (#105)

Switches analyze_file / analyze_directory / analyze_diff's main LLM path
from NewAnalyzer (LLM-only) to NewTieredAnalyzer so regex and AST rules
fire alongside LLM policies. Error semantics unchanged — partial-tier
errors still propagate to the caller (matching CLI behavior).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Fix scoped-diff instant-tier to use loaded rules

**Files:**
- Modify: `internal/mcp/server.go:600-603` (inside `handleAnalyzeDiff`)

- [ ] **Step 1: Replace the `TieredAnalyzer` construction**

At `internal/mcp/server.go:600-603`, replace:

```go
	// Run instant tier on full file, filter findings to changed lines
	fullArtifact := input.Artifact{Path: path, Content: string(content), Kind: input.KindFile}
	ta := analyzer.NewTieredAnalyzer(h.client)
	instantResults := ta.RunPatternMatching(fullArtifact)
```

with:

```go
	// Run instant tier on full file, filter findings to changed lines.
	// Pass loaded rules so custom rules fire alongside embedded defaults.
	fullArtifact := input.Artifact{Path: path, Content: string(content), Kind: input.KindFile}
	instantOpts := []analyzer.TieredAnalyzerOption{}
	if len(h.rules) > 0 {
		instantOpts = append(instantOpts, analyzer.WithInstantPatterns(h.rules))
	}
	ta := analyzer.NewTieredAnalyzer(h.client, instantOpts...)
	instantResults := ta.RunPatternMatching(fullArtifact)
```

- [ ] **Step 2: Run existing analyze_diff tests**

Run: `go test ./internal/mcp/... -run TestAnalyzeDiff -v -count=1`
Expected: all pass.

- [ ] **Step 3: Commit**

```bash
git add internal/mcp/server.go
git commit -m "mcp: pass loaded rules to scoped-diff instant tier (#105)

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Replace `buildRules` with `buildDescriptors`

**Files:**
- Modify: `internal/mcp/server.go:325-326, 397-398, 646-648, 1062-1074`

Mirror `service.buildDescriptors` at `internal/service/analyze.go:216-231`.

- [ ] **Step 1: Replace the `buildRules` function**

At `internal/mcp/server.go:1062-1074`, replace:

```go
func buildRules(policies map[string]config.Policy) []sarif.ReportingDescriptor {
	var rules []sarif.ReportingDescriptor
	for name, p := range policies {
		if p.Enabled {
			rules = append(rules, sarif.ReportingDescriptor{
				ID:               name,
				ShortDescription: sarif.Message{Text: p.Description},
				DefaultConfig:    &sarif.ReportingConfiguration{Level: p.Severity},
			})
		}
	}
	return rules
}
```

with:

```go
// buildDescriptors assembles SARIF reportingDescriptors from both enabled
// policies and loaded rules. Rule descriptors carry help/helpUri populated
// from the rule's remediation, CWE, and reference metadata.
func buildDescriptors(policies map[string]config.Policy, loadedRules []rules.Rule) []sarif.ReportingDescriptor {
	var descriptors []sarif.ReportingDescriptor
	for name, p := range policies {
		if p.Enabled {
			descriptors = append(descriptors, sarif.ReportingDescriptor{
				ID:               name,
				ShortDescription: sarif.Message{Text: p.Description},
				DefaultConfig:    &sarif.ReportingConfiguration{Level: p.Severity},
			})
		}
	}
	for _, r := range loadedRules {
		descriptors = append(descriptors, r.ToSARIFDescriptor())
	}
	return descriptors
}
```

- [ ] **Step 2: Update the three call sites**

At `internal/mcp/server.go:325-326` (inside `handleAnalyzeFile`), replace:

```go
	// Build SARIF and store so judge can evaluate later
	rules := buildRules(h.cfg.Config.Policies)
	sarifLog := sarif.Assemble(results, rules, "file", persona)
```

with:

```go
	// Build SARIF and store so judge can evaluate later.
	descriptors := buildDescriptors(h.cfg.Config.Policies, h.rules)
	sarifLog := sarif.Assemble(results, descriptors, "file", persona)
```

At `internal/mcp/server.go:397-398` (inside `handleAnalyzeDirectory`), replace:

```go
	// Build SARIF and store
	rules := buildRules(h.cfg.Config.Policies)
	sarifLog := sarif.Assemble(results, rules, "directory", persona)
```

with:

```go
	// Build SARIF and store.
	descriptors := buildDescriptors(h.cfg.Config.Policies, h.rules)
	sarifLog := sarif.Assemble(results, descriptors, "directory", persona)
```

At `internal/mcp/server.go:646-648` (inside `handleAnalyzeDiff`), replace:

```go
	// Build SARIF, apply suppressions, store, return summary
	rules := buildRules(h.cfg.Config.Policies)
	sarifLog := sarif.Assemble(allResults, rules, "diff", persona)
```

with:

```go
	// Build SARIF, apply suppressions, store, return summary.
	descriptors := buildDescriptors(h.cfg.Config.Policies, h.rules)
	sarifLog := sarif.Assemble(allResults, descriptors, "diff", persona)
```

- [ ] **Step 3: Run the Task 2 test and verify it passes end-to-end**

Run: `go test ./internal/mcp/... -run TestAnalyzeFileTool_InstantRulesFire -v -count=1`
Expected: PASS. Both the S2068 finding and descriptor assertions succeed.

- [ ] **Step 4: Run the full MCP test suite**

Run: `go test ./internal/mcp/... -count=1`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/server.go
git commit -m "mcp: include rule descriptors in SARIF output (#105)

Replaces buildRules with buildDescriptors, mirroring service.buildDescriptors
so MCP SARIF carries both policy and rule descriptors in tool.driver.rules
(with help/helpUri populated from remediation and references).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: Additional coverage — custom rules via `analyze_directory` and scoped-diff

**Files:**
- Modify: `internal/mcp/server_test.go` (append two new tests)

- [ ] **Step 1: Write custom-rule test for `analyze_directory`**

Append to `internal/mcp/server_test.go` just before `// --- analyze_diff tests ---`, immediately after `TestAnalyzeFileTool_InstantRulesFire`:

```go
// TestAnalyzeDirectoryTool_CustomRulesFire verifies that custom rules
// provided via ServerConfig.Rules (not the embedded defaults) fire via
// handleAnalyzeDirectory. Regression test for #105.
func TestAnalyzeDirectoryTool_CustomRulesFire(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "note.go")
	require.NoError(t, os.WriteFile(target, []byte(`package main

// TODO_CUSTOM: refactor this before merge
func x() {}
`), 0644))

	customRuleYAML := []byte(`rules:
  - id: "CUSTOM001"
    name: "todo-custom-marker"
    category: "maintainability"
    pattern: "TODO_CUSTOM"
    level: "warning"
    confidence: 0.9
    message: "Custom TODO_CUSTOM marker present"
`)
	rf, err := rules.ParseRuleFile(customRuleYAML)
	require.NoError(t, err)
	require.Len(t, rf.Rules, 1)

	cfg := testConfig()
	fs := testStore(t)
	h := newTestHandlers(t, cfg, fs, tmpDir, testHandlerOpts{
		client: &mockBAMLClient{},
		rules:  rf.Rules,
	})

	ctx := context.Background()
	req := mcpgo.CallToolRequest{}
	req.Params.Name = "analyze_directory"
	req.Params.Arguments = map[string]any{"path": tmpDir}

	result, err := h.handleAnalyzeDirectory(ctx, req)
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %+v", result)

	text := result.Content[0].(mcpgo.TextContent).Text
	var summary map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text), &summary))

	id, ok := summary["id"].(string)
	require.True(t, ok)

	sarifLog, err := fs.ReadSARIF(ctx, id)
	require.NoError(t, err)
	require.Len(t, sarifLog.Runs, 1)

	var foundCustom bool
	for _, r := range sarifLog.Runs[0].Results {
		if r.RuleID == "CUSTOM001" {
			foundCustom = true
			break
		}
	}
	assert.True(t, foundCustom, "expected CUSTOM001 finding, got: %+v", sarifLog.Runs[0].Results)
}
```

- [ ] **Step 2: Write scoped-diff rule-firing test**

Append immediately after the Step 1 test:

```go
// TestAnalyzeDiffTool_InstantRulesFire verifies that instant-tier rules
// fire via handleAnalyzeDiff on changed-line ranges. Regression test for #105.
func TestAnalyzeDiffTool_InstantRulesFire(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "creds.go")
	// Line 3 matches built-in rule S2068 (hardcoded-credentials).
	require.NoError(t, os.WriteFile(testFile, []byte(`package main

var password = "hunter2hunter2"
`), 0644))

	cfg := testConfig()
	fs := testStore(t)

	defaultRules, err := rules.DefaultRules()
	require.NoError(t, err)

	h := newTestHandlers(t, cfg, fs, tmpDir, testHandlerOpts{
		client: &mockBAMLClient{},
		rules:  defaultRules,
	})

	ctx := context.Background()
	req := mcpgo.CallToolRequest{}
	req.Params.Name = "analyze_diff"
	req.Params.Arguments = map[string]any{
		"path":       testFile,
		"line_start": float64(3),
		"line_end":   float64(3),
	}

	result, err := h.handleAnalyzeDiff(ctx, req)
	require.NoError(t, err)
	require.False(t, result.IsError, "expected success: %+v", result)

	text := result.Content[0].(mcpgo.TextContent).Text
	var summary map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text), &summary))

	id, ok := summary["id"].(string)
	require.True(t, ok)

	sarifLog, err := fs.ReadSARIF(ctx, id)
	require.NoError(t, err)
	require.Len(t, sarifLog.Runs, 1)

	var foundS2068 bool
	for _, r := range sarifLog.Runs[0].Results {
		if r.RuleID == "S2068" {
			foundS2068 = true
			break
		}
	}
	assert.True(t, foundS2068, "expected S2068 finding from instant tier, got: %+v", sarifLog.Runs[0].Results)
}
```

- [ ] **Step 3: Run the new tests**

Run: `go test ./internal/mcp/... -run 'TestAnalyzeDirectoryTool_CustomRulesFire|TestAnalyzeDiffTool_InstantRulesFire' -v -count=1`
Expected: both PASS.

- [ ] **Step 4: Run the full MCP test suite**

Run: `go test ./internal/mcp/... -count=1`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/server_test.go
git commit -m "test(mcp): cover custom-rule and scoped-diff rule firing (#105)

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Load rules at MCP startup in `cmd/gavel/mcp.go`

**Files:**
- Modify: `cmd/gavel/mcp.go` (add `--rules-dir` flag, call `rules.LoadRules`, pass into `ServerConfig`)

- [ ] **Step 1: Add the `rules` import**

Replace the import block at `cmd/gavel/mcp.go:3-17` with:

```go
import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/config"
	gavelmcp "github.com/chris-regnier/gavel/internal/mcp"
	"github.com/chris-regnier/gavel/internal/rules"
	"github.com/chris-regnier/gavel/internal/store"
)
```

- [ ] **Step 2: Add `mcpRulesDir` and the `--rules-dir` flag**

Replace the var block at `cmd/gavel/mcp.go:19-24`:

```go
var (
	mcpMachineConfig string
	mcpProjectConfig string
	mcpOutputDir     string
	mcpRegoDir       string
	mcpRulesDir      string
)
```

Inside `newMCPCmd`, replace the flag registrations at `cmd/gavel/mcp.go:59-62`:

```go
	cmd.Flags().StringVar(&mcpMachineConfig, "machine-config", "", "Machine-level config file (default: $HOME/.config/gavel/policies.yaml)")
	cmd.Flags().StringVar(&mcpProjectConfig, "project-config", ".gavel/policies.yaml", "Project-level config file")
	cmd.Flags().StringVar(&mcpOutputDir, "output", ".gavel/results", "Output directory for results")
	cmd.Flags().StringVar(&mcpRegoDir, "rego-dir", "", "Directory containing custom Rego policies (default: embedded policy)")
	cmd.Flags().StringVar(&mcpRulesDir, "rules-dir", "", "Directory containing custom rule YAML files (default: sibling 'rules/' directory of --project-config)")
```

- [ ] **Step 3: Load rules and pass into `ServerConfig`**

In `runMCP`, after `fs := store.NewFileStore(mcpOutputDir)` (currently `cmd/gavel/mcp.go:92`) and before `mcpServer := gavelmcp.NewMCPServer(...)` (currently line 95), insert:

```go
	// Load rules (embedded defaults + user overrides + project overrides).
	// Mirrors the CLI's tier-merging behavior in cmd/gavel/analyze.go.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	userRulesDir := filepath.Join(home, ".config", "gavel", "rules")

	projectRulesDir := mcpRulesDir
	if projectRulesDir == "" {
		projectRulesDir = filepath.Join(filepath.Dir(mcpProjectConfig), "rules")
	}

	loadedRules, err := rules.LoadRules(userRulesDir, projectRulesDir)
	if err != nil {
		return fmt.Errorf("loading rules: %w", err)
	}
```

Then replace the `mcpServer := gavelmcp.NewMCPServer(...)` block (currently lines 95-99):

```go
	// Create MCP server
	mcpServer := gavelmcp.NewMCPServer(gavelmcp.ServerConfig{
		Config:  cfg,
		Store:   fs,
		RegoDir: mcpRegoDir,
		Rules:   loadedRules,
	})
```

Note: `cmd/gavel/mcp.go` currently declares a local `err` inside the `home, err := ...` block. Confirm that the surrounding function already has an `err` in scope (from `config.LoadTiered`). If not, this insertion introduces `err` via `:=`, which is fine for the first use but subsequent `err = ...` lines must use `=`. As written, the three `err :=` declarations shadow-free because each is the first use in its own block — double-check during implementation.

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 5: Verify CLI tests pass**

Run: `go test ./cmd/... -count=1`
Expected: all pass.

- [ ] **Step 6: Verify the `--rules-dir` flag is registered**

Run: `go run ./cmd/gavel mcp --help 2>&1 | grep -- --rules-dir`
Expected: a line containing `--rules-dir string   Directory containing custom rule YAML files ...`

- [ ] **Step 7: Commit**

```bash
git add cmd/gavel/mcp.go
git commit -m "cmd/gavel: load rules at MCP startup with --rules-dir (#105)

Mirrors the CLI's tier-merging behavior (embedded defaults + user +
project). Without this, the MCP server's analyze tools would only fire
the TieredAnalyzer's embedded defaults — not user or project overrides.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Full repo verification

**Files:** none (validation only)

- [ ] **Step 1: Full test suite**

Run: `task test`
Expected: all pass.

- [ ] **Step 2: Lint**

Run: `task lint`
Expected: no vet errors.

- [ ] **Step 3: CI parity check**

Run: `task check`
Expected: generate + lint + test + cross-compile all succeed.

---

## Task 9: File follow-up issue for Approach B

**Files:** none (GitHub issue creation)

- [ ] **Step 1: Create the follow-up issue**

Run:

```bash
gh issue create --repo chris-regnier/gavel \
  --title "Refactor MCP handlers to use AnalyzeService" \
  --body "$(cat <<'EOF'
## Context

Follow-up on #105 (Approach A shipped in that issue). Approach B was deferred: consolidate duplicated analysis orchestration between CLI (`cmd/gavel/analyze.go`), `internal/service/analyze.go`, and `internal/mcp/server.go`.

## Proposal

Route MCP handlers (`handleAnalyzeFile`, `handleAnalyzeDirectory`, `handleAnalyzeDiff`) through `service.AnalyzeService.Analyze` so there is one analysis entry point. This eliminates the triple-maintenance burden that caused #105 (rule wiring was added to CLI and service but forgotten in MCP).

## Known trade-offs to resolve in design

- MCP builds its BAML client once at startup; `AnalyzeService` uses a per-call `ClientFactory`. Decide whether to share or diverge.
- MCP applies baseline and suppressions inline with store handles on the handler. `AnalyzeService` already handles baseline; suppression either moves into the service or stays with the caller.
- `handleAnalyzeDiff`'s scoped flow (full-file instant + window-scoped comprehensive + per-tier filtering and offsetting) does not fit `AnalyzeRequest` cleanly. Either expose a service-level entrypoint for scoped diffs or keep that flow separate.

## Acceptance criteria

- A single orchestration entry point for file/dir/diff analysis used by CLI, service HTTP, and MCP.
- No regression in #105's acceptance criteria (rules fire via MCP).
- Existing MCP, CLI, and service tests pass.

## Out of scope

Protocol-level changes to the MCP surface (tool names, arguments, response shapes).
EOF
)"
```

Expected: issue is created and its URL is printed.

- [ ] **Step 2: Reference the new issue from the PR description**

When opening the PR for this plan, include `Follow-up: <URL of new issue>` so the relationship is visible.

---

## Self-Review Notes

Spec coverage check:

- [x] Built-in rules fire via `analyze_file` — Task 2 test + Task 3 fix.
- [x] Custom rules fire via `analyze_directory` — Task 6 test + Task 3 fix.
- [x] Scoped-diff flow fires rules — Task 4 fix + Task 6 test.
- [x] SARIF descriptors match CLI — Task 5.
- [x] Existing MCP tests pass — validated in Tasks 1, 3, 4, 5, 6, 8.
- [x] Rule loading at startup with `--rules-dir` — Task 7.
- [x] Follow-up issue for Approach B — Task 9.

Placeholder scan: every code step includes the exact code to write; no TBDs, no "similar to Task N".

Type/signature consistency:
- `Rules []rules.Rule` in `ServerConfig` ✓
- `rules []rules.Rule` in `handlers` ✓
- `buildDescriptors(policies map[string]config.Policy, loadedRules []rules.Rule)` matches `service.buildDescriptors` ✓
- `testHandlerOpts{client, rules}` used consistently across Tasks 2 and 6 ✓
- `mockBAMLClient` implements `analyzer.BAMLClient` via `AnalyzeCode(ctx, code, policies, personaPrompt, additionalContext)` — matches the interface at `internal/analyzer/analyzer.go:15-17` ✓
