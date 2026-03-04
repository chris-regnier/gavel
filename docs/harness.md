# Harness: Systematic Variant Comparison

The harness provides a systematic way to compare analysis variants - different configurations of personas, prompts, policies, providers, and other parameters. It runs multiple iterations to smooth out LLM variance and produces aggregate metrics for comparison.

## Quick Start

```bash
# Create a variants.yaml configuration
cp scripts/eval/example-variants.yaml variants.yaml

# Run the harness
gavel harness run variants.yaml --packages internal/mcp,internal/store

# Summarize results with baseline comparison
gavel harness summarize experiment-results.jsonl --baseline baseline
```

## Configuration

Create a `variants.yaml` file that defines the variants to compare:

```yaml
runs: 3  # Iterations per variant (default: 3)

packages:  # Directories/packages to analyze
  - internal/mcp
  - internal/store

baseline: baseline  # Optional: name of baseline for delta calculations

variants:
  - name: baseline
    description: "Standard configuration"
    persona: code-reviewer
    strict_filter: true

  - name: filter_off
    description: "Without strict filter"
    persona: code-reviewer
    strict_filter: false

  - name: minimal
    description: "Minimal prompt"
    prompt_replace: |
      You are a code reviewer. Find bugs, broken error handling, and security issues.
      Use high confidence (0.8+) only for clear bugs.
    strict_filter: true
```

## Variant Options

Each variant can configure:

| Option | Description |
|--------|-------------|
| `name` | Unique identifier for the variant |
| `description` | Human-readable description |
| `persona` | Override persona (`code-reviewer`, `architect`, `security`) |
| `strict_filter` | Enable/disable applicability filter |
| `prompt_add` | Text to append to the persona prompt |
| `prompt_replace` | Complete replacement for the persona prompt |
| `policies` | Override specific policies |
| `provider` | Override provider/model settings |

## Metrics Captured

For each run, the harness captures:

- `total` - Total findings count
- `llm` - LLM-generated findings (tier=comprehensive)
- `instant` - Pattern/AST findings (tier=instant)
- `errors` - Error-level findings
- `warnings` - Warning-level findings
- `notes` - Note-level findings
- `errs_hi_conf` - Error-level findings with confidence > 0.8
- `avg_conf` - Average confidence across all findings
- `decision` - Rego verdict (merge/review/reject)

## Output

The harness produces:

1. **JSONL results file** (`experiment-results.jsonl`) - Per-run metrics
2. **Summary output** - Aggregate metrics with mean and standard deviation

### Example Summary

```
AGGREGATE METRICS (averaged across 3 runs)
======================================================================

[baseline]
  Total findings            38.0 ±5.5
  LLM findings              29.0 ±4.2
  Instant findings           9.0 ±0.0
  Error-level                7.7 ±2.5
  Warning-level             14.0 ±3.0
  Note-level                17.0 ±2.1
  Errors w/ conf>0.8         4.7 ±1.5
  Avg confidence           0.811 ±0.015

[variant]
  Total findings            35.0 ±6.2
  LLM findings              26.0 ±5.1
  ...

  Delta from baseline:
  Total findings            -7.9%
  LLM findings             -10.3%
  Error-level               +13.4%
  Errors w/ conf>0.8       -41.3%
  Avg confidence           +0.020
```

## Interpreting Results

### Good Signals

- **LLM findings decrease** → Variant reduces noise
- **High-confidence errors stable or decrease** → Precision preserved
- **Low variance (small std dev)** → Predictable behavior

### Warning Signs

- **High-confidence errors increase significantly** → Confidence inflation
- **Large variance** → More runs needed for reliable conclusions
- **Total findings explode** → Variant causes model to over-report

## Use Cases

### 1. Filter Tuning

Compare `strict_filter: true` vs `false` to measure the impact of the applicability filter.

### 2. Persona Evaluation

Compare `code-reviewer`, `architect`, and `security` personas on the same codebase.

### 3. Prompt Experiments

Test prompt modifications using `prompt_add` or `prompt_replace`:

```yaml
variants:
  - name: baseline
    persona: code-reviewer
    strict_filter: true

  - name: minimal
    prompt_replace: |
      You are a code reviewer. Find bugs, broken error handling, and security issues.
      Use high confidence (0.8+) only for clear bugs.
    strict_filter: true
```

### 4. Model Comparison

Compare different models:

```yaml
variants:
  - name: baseline
    persona: code-reviewer

  - name: bigger_model
    persona: code-reviewer
    provider:
      ollama:
        model: qwen2.5-coder:14b
```

### 5. Policy Testing

Test policy changes:

```yaml
variants:
  - name: baseline

  - name: strict_errors
    policies:
      check-error-handling:
        enabled: true
        severity: error
        instruction: "Flag all error handling that silently drops failures."
```

## Programmatic API

For internal calibration and CI integration:

```go
package main

import (
    "context"
    "github.com/chris-regnier/gavel/internal/harness"
)

func main() {
    h := harness.New("gavel", "/path/to/project")
    
    // Load base config
    if err := h.LoadConfig(); err != nil {
        panic(err)
    }
    
    // Define variants
    cfg := &harness.HarnessConfig{
        Runs:     5,
        Packages: []string{"internal/mcp", "internal/store"},
        Variants: []harness.VariantConfig{
            {Name: "baseline", Persona: "code-reviewer"},
            {Name: "variant", Persona: "architect"},
        },
    }
    
    // Run
    results, err := h.Run(context.Background(), cfg, "results.jsonl")
    if err != nil {
        panic(err)
    }
    
    // Summarize
    summary, _ := harness.SummarizeWithBaseline("results.jsonl", "baseline")
    harness.PrintSummary(summary)
}
```
