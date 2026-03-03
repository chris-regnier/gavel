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

## Experiment: Blunt Persona Tone

**Hypothesis**: Rewriting the `code-reviewer` persona to use blunt, terse, emphatic language would reduce low-value findings (noise), since direct language leaves less room for the LLM to hedge or generate speculative findings.

**Setup**: qwen2.5-coder:7b via Ollama, 3 runs across 5 packages. Baseline used the standard `code-reviewer` persona; variant replaced it with a terse rewrite.

**Baseline persona** (excerpt):
```
You are a senior code reviewer with 15+ years of experience...
YOUR TONE: Constructive and educational. When you identify issues, explain *why* they matter...
CONFIDENCE GUIDANCE:
- High (0.8-1.0): Clear violations of established patterns, obvious bugs...
- Low (0.0-0.5): Suggestions for alternative approaches, minor nitpicks
```

**Variant persona** (full):
```
You are a ruthless code reviewer. No hand-holding. No fluff.
Find real bugs. Find real problems. Skip the rest.

REPORT:
- Bugs and crashes
- Broken error handling
- Security holes
- Untestable code
- Dead code
- Unnecessary complexity

DO NOT REPORT:
- Style preferences
- Speculative issues ("might fail", "could be slow")
- Theoretical problems without concrete evidence
- Noise

CONFIDENCE:
- High (0.8-1.0): It IS broken. It WILL fail. It IS a vulnerability.
- Medium (0.5-0.8): Clear code smell. Real maintainability problem.
- Low (0.0-0.5): Don't bother. If it's this weak, skip it entirely.

Be exact. Line numbers. What's wrong. How to fix it. Done.
Never fabricate findings.
```

**Results** (filter ON / strict filter enabled):

```
                        Baseline    Blunt       Delta
Total findings          41.7        47.3        +13.4%
LLM findings            32.7        38.3        +17.1%
Instant findings         9.0         9.0         0.0%  (control stable)
Error-level              7.7        18.7       +143%
Errors w/ conf>0.8       3.7        14.3       +287%
Avg confidence           0.816       0.855      +0.039
Std dev (total)         ±5.5       ±11.8
```

**Results** (filter OFF):

```
                        Baseline    Blunt       Delta
Total findings          38.3        41.3        +7.8%
LLM findings            29.3        32.3       +10.2%
Error-level              6.7        19.3       +188%
Errors w/ conf>0.8       4.7        16.0       +240%
Avg confidence           0.823       0.863      +0.040
```

**Verdict: Do not ship.** The blunt persona backfired on every metric:

1. **More findings, not fewer** — LLM findings increased 10-17%, the opposite of the desired noise reduction.
2. **Massive confidence inflation** — High-confidence errors jumped 3-4x. The emphatic language ("It IS broken. It WILL fail.") caused the model to assign high confidence to everything rather than being more selective.
3. **Higher variance** — Std dev roughly doubled (±11.8 vs ±5.5), making results less predictable.

**Takeaway**: For a 7B model, terse emphatic language doesn't produce selectivity — it produces aggression and overconfidence. The constructive "educational" tone in the baseline produces better-calibrated confidence and fewer overall findings. Persona tone is a lever, but "more emphatic" pushes in the wrong direction for small models.

## Experiment: Measured and Firm Tone

**Hypothesis**: A measured, constructive, and firm tone — without the aggression of the blunt variant — would produce fewer speculative findings while maintaining confidence calibration. The key difference from the blunt experiment: anti-hedging instructions ("do not hedge with 'might' or 'could potentially'") combined with PR-gating confidence framing ("you would block a pull request over this") instead of emphatic absolutes.

**Setup**: qwen2.5-coder:7b via Ollama, 3 runs across 5 packages (`internal/mcp`, `internal/evaluator`, `internal/store`, `internal/sarif`, `internal/input`).

**Baseline persona** (excerpt):
```
You are a senior code reviewer with 15+ years of experience...
YOUR TONE: Constructive and educational. When you identify issues, explain *why* they matter...
CONFIDENCE GUIDANCE:
- High (0.8-1.0): Clear violations of established patterns, obvious bugs...
- Medium (0.5-0.8): Style issues, potential improvements, debatable design choices
- Low (0.0-0.5): Suggestions for alternative approaches, minor nitpicks
```

**Variant persona** (full):
```
You are a senior code reviewer. You read code carefully, identify real problems, and state them plainly.

FOCUS AREAS:
- Bugs and incorrect behavior
- Error handling that silently drops failures
- Code that is hard to test or reason about
- Unnecessary complexity and dead code
- Performance pitfalls with concrete impact

YOUR TONE:
Measured, constructive, and firm. State what the problem is, why it matters, and what to do about it.
Do not hedge with "might" or "could potentially" — if you are not confident enough to state the issue
directly, do not report it. Do not soften findings with excessive praise or apology.

CONFIDENCE CALIBRATION:
- High (0.8-1.0): You would block a pull request over this. The issue is clear and the impact is real.
- Medium (0.5-0.8): Worth raising in review. The issue is genuine but reasonable people could disagree on priority.
- Low (0.0-0.5): A concrete suggestion for improvement, not a defect.
- Do not assign high confidence to style preferences or speculative concerns.

Report precise line numbers and actionable recommendations.
Only report findings you can justify with evidence in the code. Do not fabricate issues.
```

**Key design differences from the blunt experiment:**
- No emphatic absolutes ("It IS broken") — replaced with decision framing ("would you block a PR?")
- Explicit anti-inflation guardrail ("do not assign high confidence to style preferences")
- Anti-hedging instruction targets language ("might", "could potentially") rather than demanding aggression
- Focus areas reframed from abstract qualities ("readability", "SOLID") to concrete problems ("error handling that silently drops failures")

**Results** (filter ON / strict filter enabled):

```
                        Baseline    Variant     Delta
Total findings          38.0        35.0        -7.9%
LLM findings            29.0        26.0       -10.3%
Instant findings         9.0         9.0         0.0%  (control stable)
Error-level              5.7        18.0       +216%
Warning-level           14.0         6.3        -55%
Note-level              17.0         9.7        -43%
Errors w/ conf>0.8       4.7        12.3       +162%
Avg confidence           0.811       0.903      +0.092
Std dev (total)         ±3.5        ±6.2
```

**Results** (filter OFF):

```
                        Baseline    Variant     Delta
Total findings          41.0        36.3       -11.5%
LLM findings            32.0        27.3       -14.6%
Error-level             10.3        20.0        +94%
Warning-level           17.7         6.0        -66%
Note-level              12.7        10.0        -21%
Errors w/ conf>0.8       4.7        15.7       +234%
Avg confidence           0.823       0.896      +0.073
```

**Verdict: Do not ship.** The measured tone achieved the desired noise reduction but still triggered severe confidence inflation:

1. **Noise reduction worked** — LLM findings dropped 10-15%, crossing the -10% threshold. The model produced fewer total findings under the firm tone.
2. **Severity reclassification, not reduction** — Findings didn't disappear; they migrated from warning/note → error. Warning-level dropped 55-66%, but error-level jumped 216%. The model reclassified existing findings upward rather than filtering them out.
3. **Confidence inflation persists** — High-confidence errors jumped 162% (filter ON) to 234% (filter OFF), well beyond the ±15% safety threshold. Average confidence increased from 0.81 → 0.90.
4. **Filter became less effective** — Baseline filter reduced errors by 45%; variant filter only reduced them by 10%. The firm tone overrode the filter's ability to suppress marginal findings.

**Comparison with blunt experiment:**

| Metric (filter ON) | Blunt Δ | Measured Δ | Assessment |
|---------------------|---------|------------|------------|
| LLM findings | +17.1% | -10.3% | Measured wins — actually reduced noise |
| Error-level | +143% | +216% | Both inflate, measured is worse |
| Errors w/ conf>0.8 | +287% | +162% | Both inflate, measured is less bad |
| Avg confidence | +0.039 | +0.092 | Measured inflates more |
| Std dev (total) | ±11.8 | ±6.2 | Measured is more stable |

**Takeaway**: The measured/firm tone is a step forward from the blunt experiment — it actually reduces finding count rather than increasing it, and has lower variance. But it shares the same fundamental problem: any tone instruction that implies "be more decisive" causes the 7B model to express that decisiveness through severity escalation rather than selectivity.

The confidence calibration framing ("would you block a PR?") was intended as a high bar, but the model interpreted it as a directive to rate more things as PR-blocking. The anti-inflation guardrail ("do not assign high confidence to style preferences") was insufficient to counteract this.

**Possible next iterations:**
1. Keep measured tone but add explicit distribution constraints ("most findings should be warning-level; fewer than 20% should be error-level")
2. Separate the tone instruction from confidence calibration — firm tone in the general section, but keep the original baseline confidence scale
3. Test on a larger model (Sonnet, GPT-4) where confidence calibration may respond differently to nuanced instructions
