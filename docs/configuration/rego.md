# Custom Rego Policies

The default gate policy maps findings to decisions based on severity and confidence. To customize the gating logic, place `.rego` files in `.gavel/rego/`.

## Default Behavior

| Decision | Trigger |
|----------|---------|
| `merge` | No unsuppressed findings |
| `reject` | Any unsuppressed error-level finding with confidence > 0.8 |
| `review` | All other cases (default) |

Suppressed findings (see [Suppressing Findings](../guides/suppressions.md)) are excluded from decision logic.

## Writing Custom Policies

```rego
package gavel.gate

import rego.v1

default decision := "review"

# Filter out suppressed findings
_suppressed(result) if {
    suppressions := object.get(result, "suppressions", [])
    count(suppressions) > 0
}

unsuppressed_results contains result if {
    some result in input.runs[0].results
    not _suppressed(result)
}

# Reject on any unsuppressed error with high confidence
decision := "reject" if {
    some result in unsuppressed_results
    result.level == "error"
    result.properties["gavel/confidence"] > 0.8
}

# Auto-merge if no unsuppressed findings
decision := "merge" if {
    count(unsuppressed_results) == 0
}
```

The Rego policy receives the full SARIF log as `input`. It never sees source code directly — only the structured findings. Suppressed findings remain in the SARIF results array but have a non-empty `suppressions` field.

Custom `.rego` files in the rego directory override the embedded default policy entirely. If you write custom policies, include the suppression filtering logic above to respect suppressions.

## Input Structure

The Rego policy receives a standard SARIF 2.1.0 log. Key paths:

- `input.runs[0].results` — array of findings (includes suppressed findings)
- `input.runs[0].results[_].level` — `"error"`, `"warning"`, or `"note"`
- `input.runs[0].results[_].ruleId` — the policy that triggered the finding
- `input.runs[0].results[_].properties["gavel/confidence"]` — LLM confidence (0.0-1.0)
- `input.runs[0].results[_].suppressions` — array of suppression entries (empty if not suppressed)

See [SARIF Extensions](reference/sarif.md) for all gavel-specific properties.
