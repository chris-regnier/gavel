# A/B Evaluation Methodology for Prompt Tuning

## Why Multi-Run Averaging

LLM outputs are nondeterministic. Even with the same model, code, and prompt, consecutive runs produce different findings. During initial applicability filter testing, a single-run comparison showed a 46% reduction in LLM findings — but a 3-run average of the same experiment showed only 12%.

Single-run comparisons are unreliable for measuring prompt changes. Multi-run averaging surfaces the actual signal.

## Experiment Design

Each experiment compares two conditions against the same code using the same model:

1. **Condition A** (`strict_filter: true`): Persona prompt includes the applicability filter
2. **Condition B** (`strict_filter: false`): Persona prompt without the filter

For each condition, the runner executes N iterations (default 3) across a fixed set of packages. This produces N × packages × 2 data points.

The key controls:
- **Same code**: All runs analyze the same source packages
- **Same model**: Provider config is identical between conditions
- **Same policies**: Only the filter toggle changes
- **Multiple runs**: Averages smooth out LLM nondeterminism

## Metrics Captured

Each run records these metrics per package from the SARIF output:

| Metric | Source | What It Measures |
|--------|--------|------------------|
| `total` | SARIF result count | Overall finding volume |
| `llm` | `gavel/tier = comprehensive` | LLM-generated findings (affected by prompt changes) |
| `instant` | `gavel/tier = instant` | Regex/AST findings (control — should not change) |
| `errors` | `level = error` | High-severity findings |
| `warnings` | `level = warning` | Medium-severity findings |
| `notes` | `level = note` | Low-severity findings |
| `errs_hi_conf` | `level = error AND gavel/confidence > 0.8` | High-confidence errors (actionable signals) |
| `avg_conf` | mean of `gavel/confidence` | Overall confidence calibration |
| `decision` | Rego verdict | Gate decision (merge/review/reject) |

## How to Run

### 1. Build Gavel

```bash
task build
```

### 2. Set Up Eval Directory

```bash
mkdir -p /tmp/gavel-eval/.gavel
cp scripts/eval/example-policies.yaml /tmp/gavel-eval/.gavel/policies.yaml
# Edit the provider section to match your LLM backend
```

### 3. Run the Experiment

```bash
# Default: 3 runs across 5 packages
./scripts/eval/run-experiment.sh ./dist/gavel /tmp/gavel-eval

# Custom packages and run count
RUNS=5 ./scripts/eval/run-experiment.sh ./dist/gavel /tmp/gavel-eval \
  internal/analyzer internal/config internal/mcp
```

### 4. Summarize Results

```bash
python3 scripts/eval/summarize-results.py /tmp/gavel-eval/experiment-results.jsonl
```

## Interpreting Results

### What to Look For

**Noise reduction (good):**
- LLM findings decrease → filter is suppressing speculative findings
- Instant findings unchanged → confirms the filter only affects LLM behavior
- Total errors decrease → fewer false positives at error level

**Precision preservation (good):**
- High-confidence errors stable or increasing → real issues still detected
- Average confidence stable or increasing → model is more calibrated

**Warning signs:**
- High-confidence errors increase significantly → filter may be causing the model to over-compensate with inflated confidence on remaining findings
- Average confidence drops → model is less certain about everything
- LLM findings drop to near-zero → filter is too aggressive

### Key Metrics to Watch

1. **LLM finding delta**: The primary signal. Negative delta = filter reduces noise.
2. **High-confidence error delta**: Should be near zero. Positive = concerning.
3. **Per-run variance (std dev)**: High variance means more runs are needed for reliable conclusions.

## Initial Experiment Results

**Setup**: qwen2.5-coder:7b via Ollama, 2-test applicability filter (PRACTICAL IMPACT + CONCRETE EVIDENCE), 3 runs across 5 packages.

```
AGGREGATE METRICS (averaged across 3 runs)
  Total findings         ON=38.3    OFF=42.3    Δ=-4.0    (-9.4%)
  LLM findings           ON=29.3    OFF=33.3    Δ=-4.0    (-12.0%)
  Instant findings       ON=9.0     OFF=9.0     Δ=+0.0    (+0.0%)
  Error-level            ON=7.7     OFF=10.0    Δ=-2.3    (-23.3%)
  Errors w/ conf>0.8     ON=6.3     OFF=5.0     Δ=+1.3    (+26.7%)
  Avg confidence         ON=0.843   OFF=0.832   Δ=+0.010
```

**Interpretation**: The 2-test filter produced a modest 12% reduction in LLM findings with a 23% reduction in error-level findings. However, high-confidence errors *increased* by 27%, suggesting the filter causes the small model to inflate confidence on remaining findings. The per-run standard deviation was high (±6.0 for ON total findings vs ±1.5 for OFF), indicating substantial run-to-run variance that would make single-run comparisons misleading.

**Prior comparison**: An earlier 3-test filter version (including PROPORTIONAL SEVERITY) was evaluated via single run and appeared to show a 46% improvement. The multi-run methodology revealed the actual improvement was only 12%, and the severity calibration test was adding confusion rather than helping — it was removed.
