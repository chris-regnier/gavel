# SARIF Taxonomies for CWE/OWASP References

**Issue:** [#77](https://github.com/chris-regnier/gavel/issues/77)
**Date:** 2026-04-09
**Status:** Design

## Summary

Replace the custom `gavel/cwe` and `gavel/owasp` result properties with the
SARIF 2.1.0-standard `run.taxonomies` array plus
`reportingDescriptor.relationships`. This makes Gavel's CWE/OWASP references
interoperable with security dashboards (GitHub Advanced Security, Semgrep,
Snyk, DefectDojo) that natively consume SARIF taxonomies.

## Motivation

CWE and OWASP references currently live in two places:

1. **Result properties** (`gavel/cwe`, `gavel/owasp`) — set in
   `internal/analyzer/tiered.go` for regex and AST rule findings.
2. **Rule help text** — rendered as `**CWE:** ...` / `**OWASP:** ...` sections
   inside the `ReportingDescriptor.Help.Markdown` field by
   `internal/rules/sarif.go#buildHelp`.

Neither is portable. Consumers that understand SARIF's native taxonomy format
cannot discover Gavel's references without custom parsers. Moving to the
standard format unlocks existing tooling for free.

## Non-Goals

- No changes to Rego policies, verdicts, or evaluation.
- No changes to the `Rule` YAML schema (the `cwe` / `owasp` fields stay).
- No populating CWE/OWASP on LLM-driven findings (the BAML `Finding` struct
  currently has no CWE/OWASP fields — out of scope).
- No canonical CWE/OWASP name lookup. Emitted taxa carry `id` only.

## Design

### Dependency Direction

Current layering is `rules → sarif` (the rules package imports sarif types,
not the reverse). The design preserves that. New taxonomy logic sits either
in `sarif` (as pure functions over descriptors) or in `rules` (as conversion
logic on `Rule`). Nothing in `sarif` learns about the `rules` package.

### 1. New SARIF types (`internal/sarif/sarif.go`)

Add these types to model SARIF §3.19 (`toolComponent`), §3.19.6 (`taxon`),
§3.52 (`reportingDescriptorRelationship`), and §3.53
(`reportingDescriptorReference`):

```go
// ToolComponent represents a SARIF toolComponent (§3.19). Used inside
// Run.Taxonomies to describe a taxonomy (e.g., CWE, OWASP) and its taxa.
type ToolComponent struct {
    Name         string  `json:"name"`
    Organization string  `json:"organization,omitempty"`
    Taxa         []Taxon `json:"taxa,omitempty"`
}

// Taxon represents an entry in a taxonomy (a reportingDescriptor used as
// a taxon, §3.19.6).
type Taxon struct {
    ID   string `json:"id"`
    Name string `json:"name,omitempty"`
}

// Relationship represents a reportingDescriptorRelationship (§3.52) attached
// to a rule, pointing at a taxon in a taxonomy.
type Relationship struct {
    Target RelationshipTarget `json:"target"`
    Kinds  []string           `json:"kinds,omitempty"`
}

// RelationshipTarget represents a reportingDescriptorReference (§3.53)
// identifying a specific taxon within a named toolComponent.
type RelationshipTarget struct {
    ID            string                  `json:"id"`
    ToolComponent *ToolComponentReference `json:"toolComponent,omitempty"`
}

// ToolComponentReference identifies a toolComponent by name. Used by
// RelationshipTarget to point at a taxonomy.
type ToolComponentReference struct {
    Name string `json:"name"`
}
```

Extend existing types:

- `ReportingDescriptor` gains `Relationships []Relationship` with
  `json:"relationships,omitempty"`.
- `Run` gains `Taxonomies []ToolComponent` with `json:"taxonomies,omitempty"`.

`omitempty` on every optional field preserves the current JSON shape for
outputs that have no taxonomies (e.g., LLM-only runs with no rule hits).

### 2. Rule → Relationships conversion (`internal/rules/sarif.go`)

Extend `Rule.ToSARIFDescriptor()` to populate `d.Relationships` from
`r.CWE` and `r.OWASP`:

```go
for _, cwe := range r.CWE {
    id := strings.TrimPrefix(cwe, "CWE-")  // "CWE-798" → "798"
    d.Relationships = append(d.Relationships, sarif.Relationship{
        Target: sarif.RelationshipTarget{
            ID:            id,
            ToolComponent: &sarif.ToolComponentReference{Name: "CWE"},
        },
        Kinds: []string{"relevant"},
    })
}
for _, owasp := range r.OWASP {
    d.Relationships = append(d.Relationships, sarif.Relationship{
        Target: sarif.RelationshipTarget{
            ID:            owasp,  // e.g., "A07:2021"
            ToolComponent: &sarif.ToolComponentReference{Name: "OWASP"},
        },
        Kinds: []string{"relevant"},
    })
}
```

Decisions embedded here:

- **Kind:** `"relevant"` for all relationships. Most generic SARIF value and
  matches how Semgrep / GitHub Code Scanning emit CWE references. Avoids
  claiming stricter semantics like `superset` / `subset`.
- **CWE ID format:** Strip the `CWE-` prefix so the taxon `id` is the bare
  number (`"798"`). This matches the issue example and the MITRE taxonomy's
  native identifier scheme.
- **OWASP ID format:** Keep as-is (`"A07:2021"`). OWASP Top 10 categories are
  already referenced by that exact string.

Additionally, **remove** the CWE/OWASP synthesis from `buildHelp()` (the
`**CWE:** ...` and `**OWASP:** ...` markdown lines). That information is now
first-class on the descriptor via `relationships`. The `buildHelp` guard
`if r.Remediation == "" && len(r.CWE) == 0 && ... { return nil }` becomes
`if r.Remediation == "" && len(r.References) == 0 { return nil }`.

**Keep** `resolveHelpURI()` and its `cweURL()` fallback untouched: `helpUri`
points at primary rule documentation, which is a different concept than
taxonomy relationships, and the fallback is still useful when a rule has
no explicit `references` list.

### 3. Taxonomy aggregation (`internal/sarif/taxonomies.go`, new)

A pure function over rule descriptors:

```go
// BuildTaxonomies walks the Relationships of each rule and returns the
// unique set of SARIF toolComponents referenced, each populated with its
// taxa. Taxa and taxonomies are sorted deterministically so that output
// is stable across runs. Returns nil when no relationships are present.
func BuildTaxonomies(rules []ReportingDescriptor) []ToolComponent
```

Implementation sketch:

1. Group relationship targets by `toolComponent.name`.
2. Within each group, dedupe by `target.id` using a `map[string]struct{}`.
3. Sort taxon IDs lexicographically for stability.
4. Emit one `ToolComponent` per group, setting `Organization`:
   - `"CWE"` → `"MITRE"`
   - `"OWASP"` → `"OWASP Foundation"`
   - anything else → left empty
5. Sort the returned slice by `ToolComponent.Name` so ordering is stable.

Deterministic ordering matters because SARIF output is consumed by cache
keying and snapshot tests. A map-iteration-order-based output would cause
spurious diffs.

### 4. Assembler wiring

**`internal/sarif/builder.go`** — in `Assembler.Build()`, right after
`log.Runs[0].Tool.Driver.Rules = a.rules`:

```go
if taxa := BuildTaxonomies(a.rules); len(taxa) > 0 {
    log.Runs[0].Taxonomies = taxa
}
```

**`internal/sarif/assembler.go`** — same addition in the legacy `Assemble()`
function for parity. The `service.analyze` pipeline calls `Assemble()`, not
`Assembler.Build()`, so this is the code path actually exercised in
production today.

### 5. Remove `gavel/cwe` / `gavel/owasp` result properties

Delete these blocks from `internal/analyzer/tiered.go`:

- Lines 375–380 (regex rule path, inside `runPatternMatching`)
- Lines 464–469 (AST rule path, inside `runASTRules`)

Update test fixture `internal/output/sarif_test.go:38` to remove the
`"gavel/cwe": []string{"CWE-798"}` entry.

### 6. Test plan

Tests written before or alongside implementation (TDD):

- **`internal/sarif/taxonomies_test.go`** (new): table-driven tests for
  `BuildTaxonomies`:
  - Empty rules → `nil` return.
  - Single rule with one CWE → one CWE taxonomy, one taxon.
  - Single rule with two CWEs → one CWE taxonomy, two taxa.
  - Two rules referencing the same CWE → deduped.
  - Mixed CWE + OWASP → two taxonomies.
  - Unknown taxonomy name → organization field empty.
  - Stable ordering: shuffle input, assert sorted output.
- **`internal/sarif/sarif_test.go`** (extend): JSON serialization check
  for `Taxon`, `ToolComponent`, `Relationship`, `RelationshipTarget`, and
  `ToolComponentReference` — verify `omitempty` drops unset fields.
- **`internal/rules/sarif_test.go`** (extend):
  - `ToSARIFDescriptor` for a rule with CWE + OWASP populates
    `Relationships` in the expected shape (CWE ID stripped, OWASP kept).
  - `buildHelp` no longer contains `**CWE:**` / `**OWASP:**` markdown lines
    (but still contains `**Remediation:**` and `**References:**`).
  - `resolveHelpURI` still returns `cwe.mitre.org/definitions/798.html`
    when no `references` list is present.
  - A rule with no remediation, no CWE, no OWASP, and no references still
    produces a nil `Help`.
- **`internal/sarif/assembler_test.go`** (extend): after `Assemble()`,
  assert `Runs[0].Taxonomies` is populated when rule descriptors carry
  relationships.
- **`internal/analyzer/tiered_test.go`**: remove any assertions on
  `gavel/cwe` / `gavel/owasp` result properties and add a negative
  assertion that they are absent.

Run the integration test (`go test -run TestIntegration ./...`) to
confirm end-to-end SARIF output still parses and validates.

### 7. Documentation

Update `docs/reference/sarif.md`:

- Add a "Taxonomies" section describing the new `run.taxonomies` array
  and the rule `relationships` field, with a trimmed JSON example.
- Remove the `gavel/cwe` and `gavel/owasp` entries from the result
  properties table.

## Out of Scope

- **LLM findings with CWE/OWASP.** The BAML `Finding` struct has no
  CWE/OWASP fields today, so LLM-driven results carry no taxonomy refs.
  Adding them would require a BAML schema change (`Finding.cwe` /
  `Finding.owasp`) plus type-builder regeneration — separate issue.
- **Taxon canonical names.** Our rule YAML stores only IDs. Populating
  `Taxon.Name` would require a lookup map; users picked "omit name" as
  the simplest correct option.
- **`isComprehensive` and `version` on CWE/OWASP taxonomies.** We are
  only emitting the subset our rules reference, not the complete CWE or
  OWASP catalog, so these fields are intentionally omitted.

## Risks & Mitigations

- **SARIF validator regressions.** Adding new fields at `run.taxonomies`
  could change JSON shape for snapshot tests. Mitigation: audit existing
  golden files in `internal/sarif/*_test.go` and
  `internal/output/sarif_test.go` — update only where taxonomies are
  actually expected.
- **Cache-key drift.** `cache_key` is computed from file content + model
  + policies + BAML templates (`internal/sarif/builder.go`), not from
  the SARIF log itself. Adding taxonomies to the output does NOT change
  the cache key — existing cached SARIF stays valid.
- **Ordering instability causing spurious diffs.** Mitigated by
  deterministic sorting in `BuildTaxonomies`.

## Implementation Order

1. Add the new SARIF types + extend `ReportingDescriptor` / `Run`.
2. Write `taxonomies_test.go`.
3. Implement `BuildTaxonomies`.
4. Extend `Rule.ToSARIFDescriptor` and its tests.
5. Strip CWE/OWASP from `buildHelp` and update tests.
6. Wire `BuildTaxonomies` into `Assemble` and `Assembler.Build`.
7. Remove `gavel/cwe` / `gavel/owasp` props from `tiered.go` and update
   fixtures.
8. Update `docs/reference/sarif.md`.
9. `task lint && task test && task build`.
