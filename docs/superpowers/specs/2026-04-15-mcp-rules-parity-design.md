# MCP Rules Parity — Design

**Issue:** [chris-regnier/gavel#105](https://github.com/chris-regnier/gavel/issues/105)
**Status:** Approved
**Date:** 2026-04-15

## Problem

The MCP server's `analyze_file` and `analyze_directory` tools silently skip all regex and AST rules. They invoke `analyzer.NewAnalyzer(h.client)` (LLM-only) at `internal/mcp/server.go:986`, whereas the CLI (`cmd/gavel/analyze.go:108-219`) and `AnalyzeService` (`internal/service/analyze.go:50-56`) both construct a `TieredAnalyzer` with `WithInstantPatterns(loadedRules)`.

Consequences:
- The 19 built-in default rules (embedded `default_rules.yaml`) never fire via MCP.
- Custom rules under `.gavel/rules/` never fire via MCP.
- CI (which uses the CLI) produces different results from agent IDE integrations (which use MCP) for identical inputs — defeating the fast/deterministic rule layer's role as a first line of defense.

The same bug exists in the scoped-change flow at `internal/mcp/server.go:602`: a `TieredAnalyzer` is constructed, but without `WithInstantPatterns`, so only the embedded defaults would fire — and even those are absent because the MCP server never loads rules at all.

## Scope

**In scope (this spec):**
- Load rules once at MCP server startup, mirroring the CLI's tier-merging behavior.
- Thread loaded rules through all three analysis paths in `internal/mcp/server.go`: `handleAnalyzeFile` → `runAnalysis`, `handleAnalyzeDirectory` → `runAnalysis`, and `handleAnalyzeDiff`'s scoped flow.
- Include rule descriptors in SARIF `tool.driver.rules` to match CLI output.
- Add an MCP-side `--rules-dir` flag mirroring the CLI's override.
- Tests covering rule-firing via MCP handlers.

**Out of scope (follow-up issue):**
- Refactoring MCP handlers to route through `AnalyzeService.Analyze`. This would eliminate duplicated orchestration between CLI, service, and MCP but is a larger change: MCP builds its BAML client once at startup (the service uses a per-call factory), handles baseline/suppression inline, and the scoped-diff flow doesn't fit `AnalyzeRequest` cleanly. A separate issue will be filed after this fix lands.

## Design

### 1. Startup rule loading (`cmd/gavel/mcp.go`)

Add a `--rules-dir` flag to the `mcp` command:

```go
cmd.Flags().StringVar(&mcpRulesDir, "rules-dir", "",
    "Directory containing custom rules (default: <dir of --project-config>/rules)")
```

In `runMCP`, after loading config and before creating the MCP server:

```go
userRulesDir := filepath.Join(os.Getenv("HOME"), ".config", "gavel", "rules")
projectRulesDir := mcpRulesDir
if projectRulesDir == "" {
    projectRulesDir = filepath.Join(filepath.Dir(mcpProjectConfig), "rules")
}
loadedRules, err := rules.LoadRules(userRulesDir, projectRulesDir)
if err != nil {
    return fmt.Errorf("loading rules: %w", err)
}
```

Pass `loadedRules` into `ServerConfig`:

```go
mcpServer := gavelmcp.NewMCPServer(gavelmcp.ServerConfig{
    Config:  cfg,
    Store:   fs,
    RegoDir: mcpRegoDir,
    Rules:   loadedRules,
})
```

Rationale for default derivation: the CLI uses `filepath.Join(flagPolicyDir, "rules")` where `flagPolicyDir = ".gavel"`. MCP's analog is the dir holding `--project-config`, which defaults to `.gavel/policies.yaml` — same effective location.

### 2. `ServerConfig` and `handlers` (`internal/mcp/server.go`)

```go
type ServerConfig struct {
    Config  *config.Config
    Store   store.Store
    RegoDir string
    RootDir string
    Rules   []rules.Rule // NEW
}

type handlers struct {
    cfg    ServerConfig
    client analyzer.BAMLClient
    rules  []rules.Rule // NEW
}
```

In `NewMCPServer`:

```go
h := &handlers{
    cfg:    cfg,
    client: analyzer.NewBAMLLiveClient(cfg.Config.Provider),
    rules:  cfg.Rules,
}
```

### 3. `runAnalysis` uses `TieredAnalyzer`

Replace:

```go
a := analyzer.NewAnalyzer(h.client)
return a.Analyze(ctx, artifacts, h.cfg.Config.Policies, personaPrompt)
```

with:

```go
opts := []analyzer.TieredAnalyzerOption{}
if len(h.rules) > 0 {
    opts = append(opts, analyzer.WithInstantPatterns(h.rules))
}
ta := analyzer.NewTieredAnalyzer(h.client, opts...)
return ta.Analyze(ctx, artifacts, h.cfg.Config.Policies, personaPrompt)
```

This mirrors `AnalyzeService.Analyze` at `internal/service/analyze.go:50-56`.

### 4. Scoped-diff flow

At `internal/mcp/server.go:602`, update the `TieredAnalyzer` used for `RunPatternMatching` to include loaded rules:

```go
instantOpts := []analyzer.TieredAnalyzerOption{}
if len(h.rules) > 0 {
    instantOpts = append(instantOpts, analyzer.WithInstantPatterns(h.rules))
}
ta := analyzer.NewTieredAnalyzer(h.client, instantOpts...)
instantResults := ta.RunPatternMatching(fullArtifact)
```

### 5. SARIF descriptor builder

Replace the policy-only `buildRules` helper with a descriptor builder that merges both policy and rule descriptors, mirroring `service.buildDescriptors` at `internal/service/analyze.go:216-231`:

```go
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

Update all three callers (`handleAnalyzeFile`, `handleAnalyzeDirectory`, `handleAnalyzeDiff`) to call `buildDescriptors(h.cfg.Config.Policies, h.rules)` and remove `buildRules`.

### 6. Test coverage

**New tests in `internal/mcp/server_test.go`:**

- **Built-in rule fires via `handleAnalyzeFile`:** Create a fixture file containing a hardcoded credential pattern that matches a built-in rule (e.g., `S2068`). Invoke `handleAnalyzeFile` with a mock BAML client returning no LLM findings. Assert that:
  - The stored SARIF log contains a result with the expected rule ID.
  - The `tool.driver.rules` list contains the rule's descriptor.
  - The returned `findings` count is ≥ 1.

- **Custom rule fires via `handleAnalyzeDirectory`:** Construct `ServerConfig.Rules` with a small custom regex rule directly (bypassing filesystem load). Point the handler at a fixture directory with a matching file. Assert the custom rule fires.

- **Scoped-diff flow fires rules:** For `handleAnalyzeDiff` with a line range covering a rule violation, assert the rule fires and is included in the result.

- **Existing tests:** `TestNewMCPServer`, existing `handleAnalyzeFile`/`handleAnalyzeDirectory` tests must still pass — mocks return no LLM findings, so rule-firing is additive unless fixtures contain violations (in which case tests must be updated).

**Test helper `newTestHandlers` (line 63)** will get an optional `rules []rules.Rule` parameter or a separate constructor for rule-aware tests, keeping existing test call sites unchanged.

### 7. Follow-up issue

After this lands, file a new issue titled roughly "Refactor MCP handlers to use AnalyzeService" referencing #105. Scope: replace duplicated orchestration in `internal/mcp/server.go` (`runAnalysis`, suppression/baseline/store plumbing) with calls through `service.AnalyzeService.Analyze`. Flag trade-offs: client-factory vs. long-lived client, scoped-diff flow fit, baseline/suppression path.

## Acceptance criteria

(from #105, reiterated here)

1. `gavel_analyze_file` via MCP on a file violating a built-in rule (e.g., `S2068`) produces the SARIF finding.
2. `gavel_analyze_directory` via MCP with `.gavel/rules/*.yaml` custom rules produces findings for those rules.
3. SARIF from MCP matches CLI output for the same input (modulo tier/ordering).
4. All existing MCP tests pass.
5. New tests assert rule-firing via `handleAnalyzeFile`, `handleAnalyzeDirectory`, and `handleAnalyzeDiff`.
6. Follow-up issue for Approach B is filed.

## Open questions

None — approach and scope approved.
