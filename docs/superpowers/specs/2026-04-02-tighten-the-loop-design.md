# Tighten the Loop: Incremental Analysis & Progressive Diagnostics

**Date:** 2026-04-02
**Status:** Approved
**Goal:** Make Gavel the real-time quality layer for AI-assisted coding by tightening the feedback loop in LSP and MCP integrations.

## Motivation

Developers using AI coding tools (Claude Code, Copilot, Cursor) generate code faster than they can read. Gavel should act as a five-point harness — strapping the human into the cockpit while AI runs at full tilt. Two modes matter:

1. **Audit mode** — "Show me what's wrong with this codebase." Runs once, comprehensive, worth the wait.
2. **Guardian mode** — "Catch problems in real-time as files change." Sub-second for instant tier, a few seconds for LLM findings.

Guardian mode requires three improvements to the existing architecture:
- Incremental file-level analysis (don't re-analyze unchanged files)
- Progressive diagnostic publishing (show what you know immediately, refine as LLM results arrive)
- Diff-scoped MCP analysis (AI agents check only what they just changed)

## Scope

Three deliverables. Model benchmarking is out of scope (handled separately).

### 1. Incremental File-Level Analysis

**Current state:** The LSP server's `analyzeAndPublish()` runs the full tiered analyzer on every file save. The cache exists but operates at the analysis-request level. The `DebouncedWatcher` batches rapid saves (300ms debounce).

**Change:**

The LSP server maintains an in-memory file content hash map (`map[URI]string`). On `didSave` / `didChange`:

1. Hash the new file content (SHA256, same algorithm as existing `ContentKey`)
2. Compare against the stored hash for that URI
3. If unchanged: skip analysis, keep existing diagnostics
4. If changed: run tiered analysis for that single file only

The existing per-file cache in `internal/cache/` already keys on `FileHash + FilePath + Provider + Model + Policies`. If a file's content hasn't changed since last analysis, the cache returns previous SARIF results instantly — no LLM call, no regex/AST re-run.

When content has changed, the analyzer runs only on the single changed artifact. `TieredAnalyzer.Analyze()` already accepts a slice of artifacts — pass a slice of one.

Diagnostics for other files remain untouched. The LSP server's existing `results map[DocumentURI]` cache stores per-file diagnostics independently.

### 2. Progressive LSP Diagnostics

**Current state:** The LSP server waits for all tiers to complete before publishing diagnostics. `AnalyzeProgressive()` in `tiered.go` emits results via a channel as each tier finishes, but the LSP server uses the synchronous path.

**Change:**

Wire `analyzeAndPublish()` to use `AnalyzeProgressive()` and publish diagnostics incrementally:

1. **Instant tier (~0-100ms):** Publish regex + AST findings immediately. Developer sees squiggles within 100ms of saving.
2. **Fast tier (~100ms-2s, if configured):** Merge fast-tier findings with instant-tier diagnostics, re-publish. New findings appear, existing ones remain.
3. **Comprehensive tier (~2-30s):** Merge comprehensive findings, re-publish final diagnostics. Deduplicate overlapping findings (deduplication logic in SARIF assembly already handles this).

**Diagnostic source tagging:** Each diagnostic gets a `source` string indicating its tier: `"gavel/instant"`, `"gavel/fast"`, `"gavel/comprehensive"`. Developers see at a glance whether a finding is from pattern matching or LLM analysis. Editors can group/filter by source.

**Prompt hash for provenance:** The cache key and diagnostic metadata include a prompt hash — SHA256 of the concatenated persona prompt + policy instructions sent to the LLM. This enables:
- Cache invalidation on prompt change (edit persona or policies, cached comprehensive-tier results invalidate)
- Provenance tracking in diagnostics (`gavel/prompt_hash` property, enabling diff of findings across prompt versions)
- Future benchmark correlation (map accuracy metrics to specific prompt hashes)

The existing `CacheKey` struct gains a `PromptHash string` field computed from the combined persona + policy text.

**Cancellation:** If the developer saves again while the comprehensive tier is running, cancel the in-flight LLM call and restart for the new content. Per-file cancellation context (`context.CancelFunc` stored per URI) prevents stale LLM results from overwriting fresh instant-tier results. The debounced watcher handles rapid saves at the input level; this handles cancellation at the analysis level.

**Implementation detail:** `AnalyzeProgressive()` returns `<-chan TierResult`. The LSP server spawns a goroutine per analysis that reads from this channel and calls `publishDiagnostics` on each emission.

### 3. MCP `analyze_diff` Tool

**Current state:** The MCP server exposes `analyze_file` and `analyze_directory`, both analyzing full file content. An AI agent that wrote 5 lines into a 500-line file sends the whole file through analysis.

**Change:**

Add an `analyze_diff` tool scoped to changed regions:

**Input schema:**
```
analyze_diff(
  path: string,          # required: file path
  diff: string,          # unified diff text (required if line_start/line_end not set)
  line_start: int,       # start line (required with line_end, alternative to diff)
  line_end: int,         # end line (required with line_start)
  persona: string?,      # optional persona override
)
```

Exactly one of `diff` or `line_start`+`line_end` must be provided.

Two usage modes:
- **Diff mode:** Agent passes a unified diff. Gavel extracts changed hunks and analyzes only those regions with surrounding context.
- **Range mode:** Agent passes file path + line range. Gavel reads the file and scopes analysis to that region.

**Internal flow:**

1. Parse diff/range to identify changed lines
2. Extract changed regions with configurable context window (10 lines above/below for LLM context)
3. Run instant tier (regex + AST) on full file, filter findings to those touching changed lines
4. Run comprehensive tier with scoped content, instructing LLM to focus on the changed region
5. Return SARIF results with line numbers mapped back to original file

**Reuse:** The existing `internal/input` package already supports unified diff parsing (`--diff` CLI mode). The `analyze_diff` MCP tool reuses that parser.

**Agent workflow:** A Claude Code hook calls `analyze_diff` after every file write. Instant-tier findings arrive in <100ms, comprehensive findings in a few seconds — scoped to exactly what changed.

## Architecture

No new packages. Changes touch existing modules:

| Module | Change |
|--------|--------|
| `internal/lsp/server.go` | File hash tracking, progressive diagnostic publishing, per-file cancellation |
| `internal/lsp/watcher.go` | Content hash comparison before triggering analysis |
| `internal/lsp/diagnostic.go` | Tier-source tagging on diagnostics |
| `internal/cache/cache.go` | Add `PromptHash` to `CacheKey` struct |
| `internal/mcp/server.go` | New `analyze_diff` tool registration and handler |
| `internal/analyzer/tiered.go` | No changes needed — `AnalyzeProgressive()` already exists |
| `internal/input/` | Possibly extend diff parser for range-mode extraction |
| `internal/sarif/builder.go` | Add `gavel/prompt_hash` property to SARIF output |

## Data Flow

### Guardian Mode (LSP)
```
File Save → DebouncedWatcher → Hash Check (skip if unchanged)
  → AnalyzeProgressive(single artifact)
    → Instant tier → publishDiagnostics (source: gavel/instant)
    → Fast tier    → publishDiagnostics (source: gavel/fast)     [merge + dedup]
    → Comp tier    → publishDiagnostics (source: gavel/comprehensive) [merge + dedup]
  → Cancel on re-save
```

### Guardian Mode (MCP)
```
Agent writes file → analyze_diff(path, diff)
  → Parse diff → Extract changed regions + context
  → Instant tier (full file, filter to changed lines)
  → Comprehensive tier (scoped content)
  → Return SARIF (line numbers mapped to original)
```

## Testing Strategy

- **Incremental analysis:** Unit test that saving an unchanged file produces no new analysis calls. Test that changing one file in a multi-file workspace only triggers analysis for that file.
- **Progressive diagnostics:** Integration test that diagnostics are published at least twice (instant, then comprehensive) for a single save. Verify deduplication when both tiers find the same issue.
- **Cancellation:** Test that re-saving cancels in-flight comprehensive analysis and the final diagnostics reflect the latest content.
- **Prompt hash:** Test that changing persona or policies produces a different prompt hash and invalidates cache.
- **analyze_diff:** Test both diff mode and range mode. Verify findings are filtered to changed lines. Verify line number mapping is correct.

## Success Criteria

- Instant-tier diagnostics appear in <200ms from file save (LSP)
- Re-saving an unchanged file produces zero analysis calls
- AI agents using `analyze_diff` get scoped findings for only their changes
- Prompt hash enables tracking provenance across prompt versions
- No regression in existing `analyze_file` / `analyze_directory` behavior
