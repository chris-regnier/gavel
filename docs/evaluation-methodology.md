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

## Experiment: Repeated Instructions

**Hypothesis**: Repeating the code-reviewer persona instructions twice within the prompt — once as the primary instructions and again as a "REMINDER: KEY INSTRUCTIONS" block — would increase accuracy and reduce error rate. This technique has been shown to improve instruction-following in some LLM benchmarks.

**Setup**: qwen2.5-coder:7b via Ollama, 3 runs across 5 packages (`internal/mcp`, `internal/evaluator`, `internal/store`, `internal/sarif`, `internal/input`). Baseline used the standard `code-reviewer` persona.

**Variant change**: Appended a `===== REMINDER: KEY INSTRUCTIONS =====` section to the code-reviewer prompt that restated the focus areas, confidence guidance, and tone instructions verbatim (with "(restated)" labels).

**Results** (filter ON / strict filter enabled):

```
                        Baseline    Variant     Delta
Total findings          38.0        43.3        +14.0%
LLM findings            29.0        34.3        +18.3%
Instant findings         9.0         9.0         0.0%  (control stable)
Error-level              5.7         7.0        +22.8%
Warning-level           14.0        22.0        +57.1%
Note-level              17.0        12.3        -27.6%
Errors w/ conf>0.8       4.7         4.0        -14.9%
Avg confidence           0.811       0.830      +0.019
Std dev (total)         ±3.5        ±4.5
```

**Results** (filter OFF):

```
                        Baseline    Variant     Delta
Total findings          41.0        50.0        +22.0%
LLM findings            32.0        41.0        +28.1%
Error-level             10.3         9.0        -12.6%
Warning-level           17.7        24.0        +35.6%
Note-level              12.7        14.3        +12.6%
Errors w/ conf>0.8       4.7         5.0         +6.4%
Avg confidence           0.823       0.799      -0.024
```

**Verdict: Do not ship.** Repeating instructions increased noise across the board:

1. **More findings, not fewer** — LLM findings increased 18-28%. The repeated focus areas were interpreted as encouragement to report more issues, not as reinforcement of quality thresholds.
2. **Warning inflation** — Warning-level findings jumped 36-57%. The model treated the repeated emphasis on "what to look for" as a stronger mandate to generate findings in each category.
3. **High-confidence errors stable** — Errors with conf>0.8 stayed within ±15% (4.7→4.0 ON, 4.7→5.0 OFF), so the extra findings are predominantly noise rather than signal.
4. **No confidence inflation** — Unlike the blunt and measured tone experiments, average confidence stayed roughly stable. The repeated instructions didn't cause severity reclassification — just volume increase.

**Comparison with previous experiments:**

| Metric (filter ON) | Blunt Δ | Measured Δ | Repeated Δ | Assessment |
|---------------------|---------|------------|------------|------------|
| LLM findings | +17.1% | -10.3% | +18.3% | Repeated is as bad as blunt |
| Error-level | +143% | +216% | +22.8% | Repeated avoids inflation |
| Errors w/ conf>0.8 | +287% | +162% | -14.9% | Repeated is safe here |
| Avg confidence | +0.039 | +0.092 | +0.019 | Repeated is stable |
| Warnings | — | -55% | +57.1% | Repeated inflates warnings |

**Takeaway**: Instruction repetition is a fundamentally different failure mode from tone changes. Tone experiments caused severity reclassification (findings migrate from warning → error). Repetition caused volume inflation (more findings across all levels, especially warnings). For a 7B model, restating "what to look for" is read as "look harder" rather than "look more carefully." The baseline prompt's single statement of focus areas is already sufficient — doubling it adds noise without improving precision.

## Batch Experiment: Four Variants (Parallel Run)

Four experiments were run in parallel to explore different levers for improving analysis quality with the 7B model. All used qwen2.5-coder:7b via Ollama, 3 runs across 5 packages.

### Experiment: Few-Shot Examples

**Hypothesis**: Adding concrete good/bad example findings to the BAML template would improve precision — 7B models benefit more from demonstrations than abstract instructions.

**Change**: Added to `baml_src/analyze.baml` after the INSTRUCTIONS section:
- A "GOOD FINDING" example: error with silently ignored database query error, confidence 0.92
- A "BAD FINDING" example: speculative "could benefit from more comments", confidence 0.6, with explanation of why it's bad

**Results** (filter ON):

```
                        Baseline    Variant     Delta
Total findings          38.7        35.7        -7.8%
LLM findings            29.7        26.7       -10.1%
Instant findings         9.0         9.0         0.0%  (control stable)
Error-level              9.7        15.0        +54.6%
Errors w/ conf>0.8       8.0        13.0        +62.5%
Avg confidence           0.833       0.868      +0.035
Std dev (total)         ±7.2       ±6.4
```

**Results** (filter OFF):

```
                        Baseline    Variant     Delta
Total findings          41.7        44.7        +7.2%
LLM findings            32.7        35.7        +9.2%
Error-level              5.0        15.7       +214%
Errors w/ conf>0.8       3.0        15.0       +400%
Avg confidence           0.824       0.848      +0.024
```

**Verdict: Do not ship.** Modest noise reduction with filter ON (-10% LLM) but severe confidence inflation. The example finding used `confidence: 0.92`, and the model anchored on this value — assigning high confidence to nearly everything. Without the filter, hi-conf errors jumped 400%.

**Takeaway**: Few-shot examples are powerful for 7B models, but the example values become anchors. If using few-shot in the future, the example confidence should be deliberately low (0.6-0.7) to avoid upward anchoring, and multiple examples at varying confidence levels should be included.

### Experiment: Hard Output Cap

**Hypothesis**: "Report at most 5 issues per file, keep only the most impactful" would force the model to prioritize rather than report everything.

**Change**: Added an `OUTPUT LIMIT` section to the code-reviewer persona instructing at most 5 issues per file, keeping only the most impactful.

**Results** (filter ON):

```
                        Baseline    Variant     Delta
Total findings          38.7        69.7        +80.1%
LLM findings            29.7        60.7       +104.4%
Instant findings         9.0         9.0         0.0%  (control stable)
Error-level              9.7        17.7        +82.5%
Errors w/ conf>0.8       8.0        11.3        +41.3%
Avg confidence           0.833       0.805      -0.028
Std dev (total)         ±7.2       ±4.2
```

**Verdict: Do not ship.** Catastrophic volume explosion — LLM findings more than doubled. The 7B model interpreted "at most 5 issues per file" as a quota to fill rather than a ceiling, generating far more findings than baseline. This is the worst-performing experiment across all tested variants.

**Takeaway**: Explicit numerical constraints are misinterpreted by 7B models as targets. "At most 5" becomes "find at least 5." Avoid quantitative output constraints with small models.

### Experiment: Minimal Persona

**Hypothesis**: Shorter prompt = less confusion for 7B models. Strip the persona to 3 sentences, removing elaborate focus areas, tone instructions, and detailed confidence guidance.

**Change**: Replaced the entire `codeReviewerPrompt` with:
```
You are a code reviewer. Find bugs, broken error handling, and security issues.
Use high confidence (0.8+) only for clear bugs. Use medium (0.5-0.8) for code smells. Skip anything below 0.5.
Be precise with line numbers. Only report real issues with evidence in the code.
```

**Results** (filter ON):

```
                        Baseline    Variant     Delta
Total findings          38.7        31.3       -19.1%
LLM findings            29.7        22.3       -24.9%
Instant findings         9.0         9.0         0.0%  (control stable)
Error-level              9.7        11.0        +13.4%
Warning-level           14.3         9.7        -32.2%
Note-level              14.0        10.0        -28.6%
Errors w/ conf>0.8       8.0         4.7        -41.3%
Avg confidence           0.833       0.853      +0.020
Std dev (total)         ±7.2       ±2.5
```

**Results** (filter OFF):

```
                        Baseline    Variant     Delta
Total findings          41.7        38.7        -7.2%
LLM findings            32.7        29.7        -9.2%
Error-level              5.0        14.3       +186%
Errors w/ conf>0.8       3.0         4.7        +56.7%
Avg confidence           0.824       0.828      +0.004
```

**Verdict: Most promising.** The minimal persona is the best-performing variant across all experiments:

1. **Strong noise reduction** — 25% fewer LLM findings with filter ON, crossing the -10% threshold.
2. **Hi-conf errors decreased** — Down 41%, unprecedented across all experiments. Every prior variant inflated this metric.
3. **Lowest variance** — ±2.5 vs ±7.2 baseline, making results far more predictable.
4. **Filter synergy** — Filter ON reduced LLM findings by 25% vs filter OFF, with zero change in hi-conf errors — the ideal filter behavior (removes noise, preserves signal).

**Caveat**: Hi-conf error std dev is ±5.5 (larger than the 4.7 mean), so the -41% improvement may be partially noise. Additional runs (RUNS=5+) would increase confidence.

**Takeaway**: For 7B models, prompt brevity is a feature, not a limitation. The elaborate focus areas ("Design patterns and SOLID principles"), tone instructions ("Constructive and educational"), and multi-level confidence guidance in the baseline likely overwhelm the model's instruction-following capacity. Three focused sentences produce better calibration than three detailed paragraphs.

### Experiment: Hybrid (Measured Focus + Baseline Confidence)

**Hypothesis**: Combine the measured-tone experiment's concrete focus areas (which reduced noise -10%) with the baseline's confidence guidance (which didn't inflate), getting the best of both.

**Change**: Replaced focus areas with concrete problem descriptions ("Error handling that silently drops failures" instead of "Error handling and edge cases"), added anti-hedging instruction ("Do not hedge with 'might' or 'could potentially'"), but kept the baseline's exact confidence guidance and constructive tone.

**Results** (filter ON):

```
                        Baseline    Variant     Delta
Total findings          38.7        27.7       -28.4%
LLM findings            29.7        18.7       -37.0%
Instant findings         9.0         9.0         0.0%  (control stable)
Error-level              9.7        13.0        +34.0%
Warning-level           14.3         4.0        -72.0%
Note-level              14.0        10.3        -26.4%
Errors w/ conf>0.8       8.0        11.7        +46.3%
Avg confidence           0.833       0.913      +0.080
Std dev (total)         ±7.2       ±2.5
```

**Results** (filter OFF):

```
                        Baseline    Variant     Delta
Total findings          41.7        42.7        +2.4%
LLM findings            32.7        33.7        +3.1%
Error-level              5.0        18.7       +274%
Errors w/ conf>0.8       3.0        15.0       +400%
Avg confidence           0.824       0.884      +0.060
```

**Verdict: Do not ship.** The hybrid achieved the largest raw noise reduction (-37% LLM with filter ON) but with severe confidence inflation — the same pattern as the measured-tone experiment it was derived from:

1. **Noise reduction is real** — With filter ON, LLM findings dropped from 29.7 to 18.7, the biggest reduction of any experiment.
2. **Confidence inflation persists** — Avg confidence 0.913, hi-conf errors +46%. The anti-hedging instruction ("do not hedge") still drives the model toward over-certainty.
3. **Filter dependency** — Without the filter, total findings barely changed (+2.4%) while errors exploded (+274%). The variant only works when the filter suppresses the inflated findings.

**Takeaway**: The anti-hedging instruction is the culprit, not the confidence scale. Even with the baseline's conservative confidence guidance, telling the model "do not hedge" causes it to express certainty through high confidence scores. For 7B models, removing hedging language and improving selectivity are separate goals that require separate mechanisms.

### Cross-Experiment Summary

| Metric (filter ON) | Few-shot | Hard-cap | Minimal | Hybrid |
|---------------------|----------|----------|---------|--------|
| LLM findings Δ | -10.1% | +104.4% | **-24.9%** | -37.0% |
| Error-level Δ | +54.6% | +82.5% | +13.4% | +34.0% |
| Hi-conf errors Δ | +62.5% | +41.3% | **-41.3%** | +46.3% |
| Avg confidence Δ | +0.035 | -0.028 | +0.020 | +0.080 |
| Std dev (total) | ±6.4 | ±4.2 | **±2.5** | ±2.5 |
| Verdict | No ship | No ship | **Promising** | No ship |

**Overall takeaway**: The minimal persona is the only variant that reduced noise without inflating confidence. The pattern across all six experiments (blunt, measured, repeated, few-shot, hard-cap, hybrid) is clear: **any instruction that adds complexity to the 7B model's decision-making — tone, examples, constraints, repetition — causes a compensating distortion in either volume or confidence.** The only approach that worked was *removing* instructions, not adding them.

## Finding Quality Analysis: Minimal vs Baseline

The quantitative metrics above measure *volume* and *confidence distribution*, but not whether findings are actually useful. This section examines the content quality of individual findings to determine whether the minimal persona improves, degrades, or preserves analysis quality.

### Methodology

All LLM findings from the filter_on condition across 3 runs (89 baseline, 67 minimal) were classified into categories using automated heuristics validated by manual review:

- **Plausible**: References an existing file, line is within bounds, message describes a concrete code issue without noise phrases or speculative language
- **Noise**: Contains style preferences ("add comments", "naming convention", "hardcoded"), speculative language in explanation ("might", "could potentially"), or is a non-finding ("code appears working")
- **Wrong path**: References a file that does not exist in the package (e.g., `handlers.go` instead of `server.go`)
- **Wrong line**: References a valid file but the line number exceeds the file's length

### Results

```
BASELINE — 89 LLM findings (filter_on, 3 runs)
  Plausible:      33  (37%)   avg conf 0.791
  Noise:          15  (17%)   avg conf 0.665
  Wrong path:     29  (33%)   avg conf 0.862
  Wrong line:     12  (13%)   avg conf 0.875

MINIMAL — 67 LLM findings (filter_on, 3 runs)
  Plausible:      29  (43%)   avg conf 0.833
  Noise:           3   (4%)   avg conf 0.733
  Wrong path:     26  (39%)   avg conf 0.821
  Wrong line:      9  (13%)   avg conf 0.789
```

### Interpretation

**Signal preservation**: Both conditions produce approximately the same number of plausible findings (33 vs 29). The minimal persona does not suppress real issues — it preserves the signal while cutting noise.

**Noise elimination**: The most dramatic improvement. Baseline produces 15 noise findings (style preferences, speculative concerns, "add comments" suggestions); minimal produces only 3. This is an 80% reduction in noise findings, confirming that the elaborate focus areas in the baseline ("Design patterns and SOLID principles", "Testability and test coverage gaps") actively generate low-value findings that the minimal prompt avoids entirely.

**Path hallucination is a model-level problem, not a prompt-level problem**: Both conditions hallucinate file paths at roughly the same rate (~46-52%). The model invents conventional Go filenames (`handlers.go`, `input.go`, `gate.go`) because it never sees the actual filename — the input pipeline passes only raw file content to the BAML template, not file paths. This is a finding about the input format, not the persona prompt, and should be addressed separately (see "Root Cause: Missing Filename Context" below).

**Confidence calibration improved**: The most encouraging quality signal is in the confidence distributions:
- Plausible findings: minimal assigns slightly higher confidence (0.833 vs 0.791) — reasonable, as fewer findings means less dilution
- Hallucinated findings: minimal assigns *lower* confidence to wrong-path (0.821 vs 0.862) and wrong-line (0.789 vs 0.875) findings — the model is less confident about its hallucinations, which makes confidence a better discriminator between signal and noise
- Baseline has an *inverted* calibration problem: hallucinated findings (0.862-0.875) have *higher* average confidence than plausible findings (0.791)

### Qualitative Observations

Manual review of run 1 findings revealed additional quality differences:

1. **Baseline generates more duplicate/redundant findings across files**: e.g., "add comments explaining the purpose" appears in multiple packages for different functions. The minimal persona rarely produces this pattern.

2. **Baseline fabricates nonexistent code**: e.g., claims `Message struct is not used` (it is used extensively), claims `ReadVerdict is missing a return statement` (it handles errors correctly). The minimal persona produces fewer factually incorrect claims.

3. **Minimal has a repetition problem in specific packages**: For `internal/mcp`, minimal generated 5 near-identical findings about test error messages ("lacks a detailed error message" at different line numbers). This suggests the minimal prompt's brevity leaves the model without enough guidance to vary its focus across a file.

4. **Neither condition produces true positive bug findings**: Across 156 LLM findings in both conditions combined, zero identified an actual bug that would cause a runtime failure. All plausible findings are "reasonable concerns" — valid observations about code quality, error handling patterns, or potential issues, but none that a developer would classify as a "must fix." This is likely a limitation of the 7B model's analytical capability rather than a prompt issue.

### Root Cause: Missing Filename Context

Investigation of the input pipeline (`internal/analyzer/analyzer.go` → `baml_src/analyze.baml`) revealed that **the model never sees the filename of the code it's analyzing**. The `Artifact.Path` field is available but not passed to the BAML `AnalyzeCode` function — only `Artifact.Content` is sent as the `code` parameter. The path is used only *after* analysis to populate SARIF results when the model doesn't return a `FilePath` in findings.

This explains why ~50% of findings reference nonexistent files: the model must infer filenames from package declarations, function signatures, and Go naming conventions. It defaults to conventional names (`handlers.go` for handler code, `input.go` for input package code) rather than the actual filenames (`server.go`, `handler.go`).

**Recommendation**: Include the file path as a header in the code sent to the model. This is a one-line change in `analyzer.go`:
```go
// Before: a.client.AnalyzeCode(ctx, art.Content, ...)
// After:  a.client.AnalyzeCode(ctx, fmt.Sprintf("// File: %s\n%s", art.Path, art.Content), ...)
```

This would likely reduce path hallucination from ~50% to near zero and is independent of persona prompt changes. It should be tested as its own experiment.

### Summary

| Quality Metric | Baseline | Minimal | Assessment |
|----------------|----------|---------|------------|
| Plausible findings | 33 (37%) | 29 (43%) | Signal preserved, better ratio |
| Noise findings | 15 (17%) | 3 (4%) | **80% noise reduction** |
| Path hallucinations | 41 (46%) | 35 (52%) | Same rate — model-level issue |
| Confidence discriminates signal? | No (inverted) | Partially | **Improved calibration** |
| True positive bugs found | 0 | 0 | Model capability limit |

The minimal persona produces approximately the same useful findings with dramatically less noise, slightly better confidence calibration, and lower variance. It does not degrade analysis quality. The remaining quality problems (path hallucination, no true-positive bugs) are model-level and input-format issues that prompt changes cannot address.
