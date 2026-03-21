# Finding Suppression

Gavel lacks a mechanism for users or agents to dismiss findings. Once a rule produces a result, it appears in every subsequent analysis until the rule is disabled project-wide. This design adds project-local suppressions that filter findings at evaluation time without losing the underlying SARIF data.

## Suppression File

A new file `.gavel/suppressions.yaml` stores all active suppressions. It sits alongside the existing `policies.yaml` and `rules/` directory.

```yaml
suppressions:
  - rule_id: S1001
    reason: "Intentional pattern in this codebase"
    created: "2026-03-20T14:30:00Z"
    source: "cli:user:chris"

  - rule_id: G101
    file: internal/auth/tokens.go
    reason: "False positive — variable name matches credential pattern but holds a config key"
    created: "2026-03-20T14:31:00Z"
    source: "mcp:agent:claude-code"
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `rule_id` | yes | Rule ID to suppress (e.g., `S1001`, `G101`) |
| `reason` | yes | Human-readable justification |
| `created` | yes | ISO 8601 timestamp, auto-set on creation |
| `source` | yes | Namespaced provenance string (see Source Convention) |
| `file` | no | Restrict suppression to this file path. Omit for global. |

### Source Convention

The `source` field uses a namespaced format: `<channel>:<type>:<identity>`.

| Channel | Format | Example |
|---------|--------|---------|
| CLI | `cli:user:<$USER>` | `cli:user:chris` |
| MCP | `mcp:agent:<name>` | `mcp:agent:claude-code` |
| CI (future) | `ci:job:<name>` | `ci:job:pr-review` |

The channel and identity are determined by the tool context, not user input. CLI commands auto-populate from the OS username. The MCP server uses a configured agent name or a default.

### Granularity

Two levels of suppression:

- **Global**: `rule_id` only. Suppresses the rule across the entire project.
- **Per-file**: `rule_id` + `file`. Suppresses the rule in a specific file only.

Per-region (line-based) suppression is explicitly out of scope. Code moves frequently, making line-based suppressions fragile without a content-hashing mechanism to detect drift.

### File Path Normalization

All file paths in suppressions are stored as relative paths from the project root, with forward slashes and no leading `./`. Both the `suppress` CLI command and the `Match` function normalize paths to this canonical form before storing or comparing. SARIF location URIs are normalized the same way before matching.

### Nonexistent Rules

Suppressing a rule ID that does not exist in the currently loaded rules is allowed. The CLI prints a warning ("rule S9999 is not currently defined; suppression will apply if the rule is added later") but succeeds. This supports pre-configuring suppressions before enabling rules from a different tier.

## CLI Commands

### `gavel suppress`

Add a suppression entry.

```bash
# Global suppression
gavel suppress S1001 --reason "too noisy for this project"

# Per-file suppression
gavel suppress S1001 --file internal/auth/tokens.go --reason "false positive"
```

Writes to `.gavel/suppressions.yaml`. The `--reason` flag is required. `source` is auto-set to `cli:user:$USER`. `created` is auto-set to the current time. Duplicate detection: if an identical `rule_id` + `file` pair already exists, the command updates the existing entry (new reason, timestamp, source) rather than creating a duplicate.

### `gavel suppressions`

List all active suppressions.

```
RULE     FILE                        SOURCE              REASON
S1001    (all)                       cli:user:chris      too noisy for this project
G101     internal/auth/tokens.go     mcp:agent:claude    false positive
```

Optional filter: `gavel suppressions --source "mcp:"` to show only agent-created suppressions (prefix match on source field).

### `gavel unsuppress`

Remove a suppression entry.

```bash
# Remove global suppression
gavel unsuppress S1001

# Remove file-specific suppression
gavel unsuppress S1001 --file internal/auth/tokens.go
```

Exits with an error if the specified suppression does not exist.

## MCP Tools

Three new tools registered on the MCP server, following the existing pattern in `internal/mcp/server.go`.

### `suppress_finding`

```
Parameters:
  rule_id  (string, required): Rule ID to suppress
  file     (string, optional): Restrict to this file path
  reason   (string, required): Justification for suppression
```

Source is auto-set to `mcp:agent:<configured-name>`. Returns confirmation JSON with the created entry.

### `list_suppressions`

```
Parameters: none
```

Returns JSON array of all entries from `.gavel/suppressions.yaml`.

### `unsuppress_finding`

```
Parameters:
  rule_id  (string, required): Rule ID to unsuppress
  file     (string, optional): Remove file-specific suppression only
```

Returns confirmation or error if the suppression does not exist.

## SARIF Integration

### Structural Changes

The `Result` struct in `internal/sarif/sarif.go` needs a new `Suppressions` field. A new `Suppression` SARIF type is added:

```go
type SARIFSuppression struct {
    Kind          string                 `json:"kind"`
    Justification string                 `json:"justification,omitempty"`
    Properties    map[string]interface{} `json:"properties,omitempty"`
}
```

The `Result` struct gains: `Suppressions []SARIFSuppression \`json:"suppressions,omitempty"\``

### Application Timing

Suppressions are applied at two points, ensuring both fresh analysis and re-evaluation work correctly:

1. **During `analyze`**: After SARIF assembly and deduplication, the analyze command loads `suppressions.yaml` and stamps matching results with the `suppressions` array. The SARIF is stored with these annotations.

2. **During `judge`**: Before passing SARIF to the Rego evaluator, the judge command re-loads `suppressions.yaml` and freshly applies suppressions to the stored SARIF. This is a full reset: existing suppression annotations on results are cleared first, then current suppressions are applied from scratch. This ensures that both newly added suppressions and removed suppressions (`unsuppress`) take effect on re-evaluation without re-running the LLM.

This hybrid model means `analyze` captures the suppression state at analysis time (for the stored SARIF record), while `judge` always uses the current suppression state (for the verdict).

### SARIF Suppression Kind

The SARIF `suppression.kind` is set to `"external"` (not `"inSource"`), because these suppressions come from a configuration file, not from inline source code annotations. This is semantically correct per SARIF 2.1.0 and is interpreted correctly by SARIF consumers like GitHub Code Scanning.

```json
{
  "ruleId": "G101",
  "level": "warning",
  "message": { "text": "Potential hardcoded credential" },
  "locations": ["..."],
  "suppressions": [
    {
      "kind": "external",
      "justification": "False positive — variable name matches credential pattern but holds a config key",
      "properties": {
        "gavel/source": "mcp:agent:claude-code",
        "gavel/created": "2026-03-20T14:31:00Z"
      }
    }
  ]
}
```

Key behaviors:

- **Full SARIF is always stored.** Suppressed findings remain in the log with their `suppressions` metadata. Nothing is discarded.
- **Rego evaluator skips suppressed results.** The default Rego policy is updated to filter suppressed results (see Rego Changes below).
- **Re-evaluation without re-analysis.** Adding or removing suppressions changes the `judge` verdict without re-running the LLM analysis.
- **Cache keys are unaffected.** Suppressions do not influence cache keys, consistent with the existing design where Rego policies also do not affect cache keys. Both only affect evaluation, not analysis.

### Rego Changes

The default Rego policy (`internal/evaluator/default.rego`) is updated to define a helper that filters suppressed results. All decision rules use only unsuppressed results:

```rego
# Helper: a result is suppressed if it has a non-empty suppressions array
_suppressed(result) if {
    suppressions := object.get(result, "suppressions", [])
    count(suppressions) > 0
}

# Unsuppressed results only
unsuppressed_results contains result if {
    some result in input.runs[0].results
    not _suppressed(result)
}

# Reject: high-confidence errors among unsuppressed results
decision := "reject" if {
    some result in unsuppressed_results
    result.level == "error"
    result.properties["gavel/confidence"] > 0.8
}

# Merge: no unsuppressed results at all
decision := "merge" if {
    count(unsuppressed_results) == 0
}
```

### Evaluator Go-Side Filtering

The `RelevantFindings` slice built in `internal/evaluator/evaluator.go` excludes suppressed results (those with non-empty `Suppressions`). The verdict reason string reports unsuppressed finding count (e.g., "3 findings, 2 suppressed"). This ensures the stored verdict only lists findings that actually contributed to the decision.

## Suppression Loading

A new package `internal/suppression/` handles loading, saving, and matching:

- `Load(projectDir string) ([]Suppression, error)` — reads `.gavel/suppressions.yaml`, returns the list. Returns empty list (not error) if file does not exist.
- `Save(projectDir string, suppressions []Suppression) error` — writes the list back.
- `Match(suppressions []Suppression, ruleID string, filePath string) *Suppression` — returns the first matching suppression for a given finding, or nil. Global suppressions (no `file`) match any file. Per-file suppressions match only the specified file. Both the suppression's `file` field and the input `filePath` are normalized before comparison.
- `Apply(suppressions []Suppression, sarifLog *sarif.Log)` — clears all existing suppression annotations on results, then walks all results in the SARIF log, calls `Match` for each, and sets the `Suppressions` field on matching results. The clear-then-apply approach ensures removed suppressions take effect correctly.

## Pipeline Integration

### `analyze` command

After SARIF assembly and deduplication, the analyze command:

1. Loads suppressions from `.gavel/suppressions.yaml`
2. Calls `suppression.Apply()` to stamp matching results
3. Stores the annotated SARIF

The analysis summary output includes a `suppressed` count alongside `findings`.

### `judge` command

Before evaluation, the judge command:

1. Reads the stored SARIF
2. Loads current suppressions from `.gavel/suppressions.yaml`
3. Calls `suppression.Apply()` to re-apply current suppressions (idempotent)
4. Passes the updated SARIF to the Rego evaluator
5. The evaluator filters suppressed results in both the Rego policy and the Go-side `RelevantFindings` builder

This ensures suppressions added after analysis take effect on the next `judge` run.

### MCP handlers

The `handleAnalyzeFile` and `handleAnalyzeDirectory` handlers follow the same pattern as the `analyze` command: load suppressions, apply after assembly, store. The `handleJudge` handler follows the `judge` pattern: re-apply before evaluation.

## Out of Scope

- Inline comment suppression (`// gavel:ignore`)
- Per-region (line-based) suppressions
- Result-referencing (`gavel suppress --result <id> --finding 3`)
- Suppression expiry or TTL
- Wildcard rule IDs (`S10*`)
- Tiered suppression merging (user-level `~/.config/gavel/suppressions.yaml`)
- Concurrent write safety for `suppressions.yaml` (last writer wins; acceptable for v1)
