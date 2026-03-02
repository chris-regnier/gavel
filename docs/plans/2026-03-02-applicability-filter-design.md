# Applicability Filter for BAML Prompt

## Problem

Gavel's LLM analysis produces findings that are technically correct but practically irrelevant. From a review of `internal/mcp/`, three noise patterns emerged:

1. **Theoretical edge cases** - flagging a path validation bug when `RootDir` is `/`, a configuration that would never occur in practice (error, confidence 0.95)
2. **Speculative concerns** - flagging potential thread-safety issues on a stateless HTTP client with no evidence of actual races (warning, confidence 0.6)
3. **Severity mismatch** - flagging a missing test assertion as error-level when it's a minor test hygiene issue (error, confidence 0.95)

These three findings triggered a **reject** verdict on fundamentally sound code.

## Solution

Add an **applicability filter** to the BAML prompt that instructs the LLM to suppress impractical findings before reporting them. The filter is injected by appending it to the persona prompt string on the Go side, avoiding any changes to the BAML template, interface, or code generation.

### Filter Text

The filter applies three tests to each potential finding:

1. **PRACTICAL IMPACT** (hard gate): Would this issue cause a real problem in a realistic production scenario? If it requires an unrealistic or adversarial configuration to trigger, suppress it.
2. **CONCRETE EVIDENCE** (hard gate): Is there concrete evidence in the code that this is an actual problem? If purely speculative, suppress it.
3. **PROPORTIONAL SEVERITY** (calibration): Assign severity proportional to actual impact. Test hygiene = note. Theoretical concerns surviving tests 1-2 = warning at most. Reserve error for clear, demonstrable defects.

### Opt-out

A `strict_filter` boolean in `Config` (default `true`) controls whether the filter is appended. Users set `strict_filter: false` in `policies.yaml` to get raw unfiltered LLM output.

## Architecture

```
Config.StrictFilter (default true)
  -> cmd/gavel/analyze.go: append filter to personaPrompt if true
  -> internal/mcp/server.go: same for MCP path
    -> personaPrompt flows into BAML via {{ personaPrompt }}
      -> LLM applies/skips filter per finding
```

## Files Changed

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `StrictFilter bool` field to `Config` |
| `internal/config/defaults.go` | Set `StrictFilter: true` in `SystemDefaults()` |
| `internal/config/config.go` | Handle `StrictFilter` in `MergeConfigs()` |
| `internal/analyzer/personas.go` | Add `ApplicabilityFilterPrompt` constant |
| `cmd/gavel/analyze.go` | Append filter to persona prompt when enabled |
| `internal/mcp/server.go` | Same for MCP code path |

## What This Does NOT Change

- No BAML template changes (`baml_src/analyze.baml` untouched)
- No `BAMLClient` interface changes
- No `task generate` needed
- No Rego threshold changes
- No persona confidence calibration changes
- No deduplication logic changes

## Expected Impact

From the MCP review example:
- Finding 1 (root `/` edge case): Fails test 1 -> suppressed
- Finding 2 (thread safety): Fails test 2 -> suppressed
- Finding 3 (missing assertion): Survives tests 1-2 but test 3 downgrades from error to note -> verdict changes from reject to review/merge

## Testing

- Unit test: verify filter constant is appended when `StrictFilter` is true, absent when false
- Unit test: verify `MergeConfigs` handles `StrictFilter` correctly
- Integration: re-run `gavel analyze` on `internal/mcp/` with filter enabled, verify noise reduction
