# SARIF `fixes` for auto-remediation

**Issue**: chris-regnier/gavel#78
**Status**: Design approved, ready for implementation plan
**Scope**: Narrow — BAML schema + SARIF structs + analyzer wiring. LSP consumption is a follow-up issue.

## Motivation

The BAML `AnalyzeCode` function already returns a free-text `recommendation` for each finding. SARIF 2.1.0 defines a structured `fixes` field that supports machine-applicable replacements. Emitting structured fixes unlocks:

- One-click quick-fixes in the Gavel LSP (follow-up issue)
- Auto-fix suggestions in GitHub Code Scanning
- CI-based auto-remediation workflows

The existing `gavel/recommendation` property stays in place for backward compatibility; the new `fixes` field is additive.

## Approach

The LLM produces one optional `fixReplacementText` per finding. It reuses the finding's own `startLine`/`endLine` as the region to replace. On output, Gavel maps this into the spec-compliant `Fix`/`ArtifactChange`/`Replacement` structure.

This keeps the prompt simple (one extra field, easy for small models to fill reliably) while emitting fully spec-compliant SARIF. Column-level precision and multi-region fixes are out of scope — they can be added later if needed.

## BAML Schema Change

In `baml_src/analyze.baml`, add one optional field to the `Finding` class:

```
class Finding {
  // ... existing fields ...
  fixReplacementText string?  // Optional: raw text to replace the flagged region with
}
```

Prompt instruction updates inside `AnalyzeCode`:

- When a finding has a clear, machine-applicable fix, provide raw replacement text spanning exactly `startLine` to `endLine`.
- For vague or structural findings ("consider restructuring"), leave `fixReplacementText` empty.
- Provide raw text without markdown code fences.

After schema changes, `task generate` regenerates `baml_client/`. In `internal/analyzer/bamlclient.go`, `convertFindings()` copies the new field into `analyzer.Finding`, which gains a corresponding `FixReplacementText string` field (empty string when absent).

## SARIF Struct Additions

In `internal/sarif/sarif.go`, add three new types per SARIF 2.1.0:

```go
type Fix struct {
    Description     Message          `json:"description,omitempty"`
    ArtifactChanges []ArtifactChange `json:"artifactChanges"`
}

type ArtifactChange struct {
    ArtifactLocation ArtifactLocation `json:"artifactLocation"`
    Replacements     []Replacement    `json:"replacements"`
}

type Replacement struct {
    DeletedRegion   Region           `json:"deletedRegion"`
    InsertedContent *ArtifactContent `json:"insertedContent,omitempty"`
}
```

Extend `sarif.Result` with an optional `Fixes` field:

```go
type Result struct {
    // ... existing fields ...
    Fixes []Fix `json:"fixes,omitempty"`
}
```

`omitempty` keeps SARIF output unchanged for findings without fixes — backward compatible with all existing consumers.

`Replacement.DeletedRegion` reuses the existing `Region` struct (startLine/endLine). `InsertedContent` reuses `ArtifactContent` (which has a `Text` field). No duplication.

## Analyzer → SARIF Wiring

In `internal/analyzer/analyzer.go`, inside the result-building loop, after constructing `result := sarif.Result{...}`, conditionally attach a fix:

```go
if f.FixReplacementText != "" {
    result.Fixes = []sarif.Fix{{
        Description: sarif.Message{Text: f.Recommendation},
        ArtifactChanges: []sarif.ArtifactChange{{
            ArtifactLocation: sarif.ArtifactLocation{URI: path},
            Replacements: []sarif.Replacement{{
                DeletedRegion: sarif.Region{
                    StartLine: f.StartLine,
                    EndLine:   f.EndLine,
                },
                InsertedContent: &sarif.ArtifactContent{
                    Text: f.FixReplacementText,
                },
            }},
        }},
    }}
}
```

The fix's `description` reuses the existing `recommendation` text (human-readable summary). The replacement carries the machine-applicable payload. `gavel/recommendation` stays in `Properties` for backward compatibility — consumers that don't understand `fixes` still get the text.

### Deduplication

In `internal/sarif/assembler.go`, the existing dedup logic keeps the highest-confidence result among overlapping findings. Because `Fixes` is attached to the `Result` before dedup runs, the surviving result naturally carries its own fix — no extra merging logic needed.

## Testing

1. **`internal/sarif/sarif_test.go`** — JSON marshaling tests for `Fix`/`ArtifactChange`/`Replacement`. Verify a `Result` with `Fixes` serializes to spec-compliant JSON matching the shape in the issue, and a `Result` without `Fixes` omits the field entirely.

2. **`internal/analyzer/analyzer_test.go`** — Use the existing mock `BAMLClient` to return a `Finding` with `FixReplacementText` populated. Assert the resulting `sarif.Result` has `Fixes` with the expected `DeletedRegion` and `InsertedContent`. Also test the empty case: a `Finding` with empty `FixReplacementText` produces a `Result` with no `Fixes`.

3. **`internal/sarif/assembler_test.go`** — Verify deduplication preserves `Fixes` on the surviving result (highest-confidence wins, its fix travels with it).

4. **Integration test** — Existing `TestIntegration` in `cmd/gavel/` should continue passing unchanged (fix is optional). Add one assertion that when the mock returns a fix, the final SARIF output contains a `fixes` array.

No test needed for the BAML `fixReplacementText` field itself — coverage comes via the analyzer tests through the mock, and the BAML generator is trusted.

## Edge Cases

1. **LLM returns fix with wrong line range**: Can't verify — we trust the LLM's stated region. SARIF `fixes` is advisory; downstream tools should apply with user review.

2. **LLM returns fix for a `none`-level finding**: Still include it. Severity governs verdict evaluation, not fix availability.

3. **Fix spans beyond file bounds**: Not validated at SARIF emission. Consumers handle this.

4. **Markdown code fences in replacement text**: The prompt explicitly instructs raw text. If small models ignore the instruction in practice, add a strip pass in `convertFindings()` in `bamlclient.go` (post-implementation check).

## Out of Scope

- LSP code action consumption of the new `Fixes` field (separate follow-up issue).
- Multi-region or multi-file fixes.
- Column-level precision in `DeletedRegion`.
- Validation that replacement text actually fits the region.
