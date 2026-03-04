---
name: tune-review
description: Use when tuning Gavel's LLM review prompts - modifying persona text, applicability filters, or BAML templates and measuring the effect with A/B evaluation. Triggers on prompt tuning, review quality improvement, persona changes, filter adjustments, or finding noise reduction.
---

# Tune Review

Run A/B evaluations to measure the effect of prompt changes on Gavel's LLM analysis quality using `gavel harness`.

**Core principle:** Never ship a prompt change based on a single run. LLM nondeterminism makes single-run comparisons unreliable. Always use multi-run averaging.

## When to Use

- Modifying persona prompts in `internal/analyzer/personas.go`
- Changing the applicability filter (`ApplicabilityFilterPrompt`)
- Adjusting BAML templates in `baml_src/analyze.baml`
- Adding/removing/editing policies for evaluation
- Comparing models or providers for quality

## What Can Vary

Each variant in the harness config can override any combination of these knobs:

| Knob | Variant Field | Notes |
|------|---------------|-------|
| Persona selection | `persona: code-reviewer\|architect\|security` | Switches persona prompt |
| Filter on/off | `strict_filter: true/false` | Toggles applicability filter |
| Extra prompt text | `prompt_add: "..."` | Appended to persona prompt |
| Replace prompt | `prompt_replace: "..."` | Completely replaces persona prompt |
| Policy instructions | `policies:` section | Merged with base config policies |
| Model/provider | `provider:` section | Override provider name and/or model |

For changes that require code edits (persona text, filter logic, BAML templates), rebuild between variants — see "Code Change Workflow" below.

## Process

### 1. Define the Variation

Decide what you're changing and what you're holding constant. Write down your hypothesis:
> "Adding X to the prompt should reduce Y by Z% without increasing high-confidence errors."

### 2. Create a Variants Config

Create a YAML file defining your experiment. See `scripts/eval/example-variants.yaml` for a full reference.

```yaml
runs: 3
baseline: baseline

# Local packages
targets:
  - path: internal/mcp
  - path: internal/evaluator

# Or external repos
# repos:
#   - name: juice-shop
#     url: https://github.com/juice-shop/juice-shop
#     branch: master
# targets:
#   - repo: juice-shop
#     paths: [server, frontend/src/app]

variants:
  - name: baseline
    description: "Current configuration"
    persona: security
    strict_filter: true

  - name: variant
    description: "With proposed change"
    persona: security
    strict_filter: true
    prompt_add: "Pay special attention to injection vulnerabilities."
```

### 3. Build and Run

```bash
task build
gavel harness run variants.yaml --output results.jsonl
```

### 4. Summarize and Compare

```bash
gavel harness summarize results.jsonl --baseline baseline
```

For machine-readable output:
```bash
gavel harness summarize results.jsonl --baseline baseline --format json
gavel harness summarize results.jsonl --baseline baseline --format yaml
```

### 5. Decide

Apply the decision framework below, then commit or discard.

## Code Change Workflow

When the change requires editing Go code or BAML templates (not just config knobs), you need separate binaries:

```bash
# 1. Build baseline binary
task build
cp dist/gavel /tmp/gavel-baseline

# 2. Make your code change
# Edit personas.go, analyze.baml, etc.
# If BAML: task generate && task build
# If Go only: task build
cp dist/gavel /tmp/gavel-variant

# 3. Run baseline experiment
GAVEL_BINARY=/tmp/gavel-baseline gavel harness run variants.yaml -o baseline.jsonl

# 4. Run variant experiment
GAVEL_BINARY=/tmp/gavel-variant gavel harness run variants.yaml -o variant.jsonl

# 5. Compare
gavel harness summarize baseline.jsonl
gavel harness summarize variant.jsonl
```

## CLI Reference

### `gavel harness run <variants.yaml>`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--runs` | `-n` | From config or 3 | Number of runs per variant |
| `--output` | `-o` | `experiment-results-<timestamp>.jsonl` | Output JSONL file path |
| `--packages` | | From config | Override target packages |
| `--config` | | `.gavel/policies.yaml` | Base config file path |

### `gavel harness summarize <results.jsonl>`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--baseline` | | (none) | Baseline variant name for delta calculations |
| `--format` | `-f` | `text` | Output format: `text`, `json`, or `yaml` |

## Decision Framework

| Signal | Meaning | Action |
|--------|---------|--------|
| LLM findings Δ < -10% | Noise reduction | Good — proceed if other metrics stable |
| LLM findings Δ > +10% | More noise | Bad — reconsider change |
| High-conf errors Δ > +15% | Confidence inflation | Bad — model is compensating, prompt is confusing |
| High-conf errors Δ < -15% | Real issues suppressed | Bad — filter too aggressive |
| Instant findings Δ ≠ 0 | Control metric shifted | Bug — instant tier shouldn't change |
| Std dev > mean Δ | High variance | Inconclusive — increase runs |

**Ship when:** LLM delta is negative, high-conf errors are stable (±15%), and variance is low enough that the delta exceeds 2× std dev.

## Increasing Confidence

- **More runs:** `--runs 5` or `--runs 10` reduces variance
- **More targets:** Add targets to increase sample size per run
- **External repos:** Use `repos:` to test against real-world codebases (e.g., OWASP Juice Shop)
- **Different models:** Add model variants to ensure prompt isn't model-specific

## Key Files

- `scripts/eval/example-variants.yaml` — Full harness config example
- `scripts/eval/juice-shop-security.yaml` — Ready-to-use security calibration config
- `internal/harness/` — Harness implementation
- `internal/analyzer/personas.go` — Persona prompts + applicability filter
- `baml_src/analyze.baml` — BAML analysis template

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Single-run comparison | Always use runs≥3; initial tests showed 46% apparent improvement was actually 12% |
| Changing multiple knobs at once | Vary one thing at a time to isolate effects |
| Forgetting to rebuild after code changes | `task build` (or `task generate && task build` for BAML) |
| Using same output file without renaming | Use distinct `-o` paths or rename old JSONL before re-running |
| Evaluating on trivial code | Use real packages or external repos with meaningful logic |
| Not setting `GAVEL_BINARY` for code changes | Without it, harness uses `gavel` from PATH — both variants run the same binary |
