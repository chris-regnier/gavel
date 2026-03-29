# Custom Rego Policies

The default gate policy maps findings to decisions based on severity and confidence. To customize the gating logic, place `.rego` files in `.gavel/rego/`.

## Default Behavior

| Decision | Trigger |
|----------|---------|
| `merge` | No findings |
| `reject` | Any error-level finding with confidence > 0.8 |
| `review` | All other cases (default) |

## Writing Custom Policies

```rego
package gavel.gate

import rego.v1

default decision := "review"

# Reject on any error with high confidence
decision := "reject" if {
    some result in input.runs[0].results
    result.level == "error"
    result.properties["gavel/confidence"] > 0.8
}

# Auto-merge if clean
decision := "merge" if {
    count(input.runs[0].results) == 0
}
```

The Rego policy receives the full SARIF log as `input`. It never sees source code directly — only the structured findings.

Custom `.rego` files in the rego directory override the embedded default policy entirely.

## Input Structure

The Rego policy receives a standard SARIF 2.1.0 log. Key paths:

- `input.runs[0].results` — array of findings
- `input.runs[0].results[_].level` — `"error"`, `"warning"`, or `"note"`
- `input.runs[0].results[_].ruleId` — the policy that triggered the finding
- `input.runs[0].results[_].properties["gavel/confidence"]` — LLM confidence (0.0–1.0)

See [SARIF Extensions](reference/sarif.md) for all gavel-specific properties.
