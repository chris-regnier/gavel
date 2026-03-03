---
name: tune-review
description: Use when tuning Gavel's LLM review prompts - modifying persona text, applicability filters, or BAML templates and measuring the effect with A/B evaluation. Triggers on prompt tuning, review quality improvement, persona changes, filter adjustments, or finding noise reduction.
---

# Tune Review

Run A/B evaluations to measure the effect of prompt changes on Gavel's LLM analysis quality.

**Core principle:** Never ship a prompt change based on a single run. LLM nondeterminism makes single-run comparisons unreliable. Always use multi-run averaging.

## When to Use

- Modifying persona prompts in `internal/analyzer/personas.go`
- Changing the applicability filter (`ApplicabilityFilterPrompt`)
- Adjusting BAML templates in `baml_src/analyze.baml`
- Adding/removing/editing policies for evaluation
- Comparing models or providers for quality

## What Can Vary

| Knob | Location | How to Vary |
|------|----------|-------------|
| Persona text | `internal/analyzer/personas.go` | Edit prompt constants, rebuild |
| Applicability filter | `internal/analyzer/personas.go` | Edit `ApplicabilityFilterPrompt`, rebuild |
| Filter on/off | `.gavel/policies.yaml` | `strict_filter: true/false` (script toggles this) |
| Persona selection | `.gavel/policies.yaml` | `persona: code-reviewer\|architect\|security` |
| Policy instructions | `.gavel/policies.yaml` | Edit `policies:` section |
| Model | `.gavel/policies.yaml` | Change provider/model |
| BAML template | `baml_src/analyze.baml` | Edit template, run `task generate`, rebuild |

## Process

### 1. Define the Variation

Decide what you're changing and what you're holding constant. Write down your hypothesis:
> "Adding X to the prompt should reduce Y by Z% without increasing high-confidence errors."

### 2. Set Up Eval Environment

```bash
mkdir -p /tmp/gavel-eval/.gavel
cp scripts/eval/example-policies.yaml /tmp/gavel-eval/.gavel/policies.yaml
# Configure provider section for your LLM backend
```

### 3. Build Baseline Binary

```bash
task build
cp dist/gavel /tmp/gavel-eval/gavel-baseline
```

### 4. Run Baseline Experiment

```bash
./scripts/eval/run-experiment.sh /tmp/gavel-eval/gavel-baseline /tmp/gavel-eval
mv /tmp/gavel-eval/experiment-results.jsonl /tmp/gavel-eval/baseline.jsonl
```

### 5. Make Your Change

Edit the prompt/template, then rebuild:

```bash
# If changing Go code (personas, filter):
task build
cp dist/gavel /tmp/gavel-eval/gavel-variant

# If changing BAML templates:
task generate && task build
cp dist/gavel /tmp/gavel-eval/gavel-variant
```

### 6. Run Variant Experiment

```bash
./scripts/eval/run-experiment.sh /tmp/gavel-eval/gavel-variant /tmp/gavel-eval
mv /tmp/gavel-eval/experiment-results.jsonl /tmp/gavel-eval/variant.jsonl
```

### 7. Compare Results

```bash
echo "=== BASELINE ===" && python3 scripts/eval/summarize-results.py /tmp/gavel-eval/baseline.jsonl
echo "=== VARIANT ===" && python3 scripts/eval/summarize-results.py /tmp/gavel-eval/variant.jsonl
```

### 8. Decide

Apply the decision framework below, then commit or discard.

## Shortcut: Filter Toggle Only

When only toggling `strict_filter` (no code changes needed), a single experiment run captures both conditions:

```bash
task build
./scripts/eval/run-experiment.sh ./dist/gavel /tmp/gavel-eval
python3 scripts/eval/summarize-results.py /tmp/gavel-eval/experiment-results.jsonl
```

The script toggles `strict_filter` between `true`/`false` automatically.

## Decision Framework

| Signal | Meaning | Action |
|--------|---------|--------|
| LLM findings Δ < -10% | Noise reduction | Good — proceed if other metrics stable |
| LLM findings Δ > +10% | More noise | Bad — reconsider change |
| High-conf errors Δ > +15% | Confidence inflation | Bad — model is compensating, prompt is confusing |
| High-conf errors Δ < -15% | Real issues suppressed | Bad — filter too aggressive |
| Instant findings Δ ≠ 0 | Control metric shifted | Bug — instant tier shouldn't change |
| Std dev > mean Δ | High variance | Inconclusive — increase RUNS |

**Ship when:** LLM delta is negative, high-conf errors are stable (±15%), and variance is low enough that the delta exceeds 2× std dev.

## Increasing Confidence

- **More runs:** `RUNS=5` or `RUNS=10` reduces variance
- **More packages:** Add packages to increase sample size per run
- **Different models:** Test on multiple models to ensure prompt isn't model-specific

## Key Files

- `scripts/eval/run-experiment.sh` — Experiment runner
- `scripts/eval/summarize-results.py` — Results summarizer
- `scripts/eval/example-policies.yaml` — Config template
- `internal/analyzer/personas.go` — Persona prompts + applicability filter
- `baml_src/analyze.baml` — BAML analysis template
- `docs/evaluation-methodology.md` — Full methodology documentation

## Common Mistakes

| Mistake | Fix |
|---------|-----|
| Single-run comparison | Always use RUNS≥3; initial tests showed 46% apparent improvement was actually 12% |
| Changing multiple knobs at once | Vary one thing at a time to isolate effects |
| Forgetting to rebuild after code changes | `task build` (or `task generate && task build` for BAML) |
| Using the same eval dir without clearing results | Move/rename old JSONL before re-running |
| Evaluating on trivial code | Use real packages with meaningful logic |
