# Benchmarking, Evaluation & Monitoring Design

## Overview

A layered system for evaluating Gavel's analysis quality and operational performance, with three independent layers that compose together:

1. **Benchmark harness** — labeled corpus, deterministic scoring, LLM-as-judge
2. **OTel integration** — operational metrics and tracing via OpenTelemetry
3. **Feedback collection** — CLI feedback, GitHub SARIF integration, corpus refinement loop

Each layer works independently. Benchmark runs emit OTel signals, feedback data enriches the corpus, and OTel dashboards visualize both operational and quality metrics.

```
┌──────────────────────────────────────────────────────────┐
│                    Grafana Dashboard                     │
│  [Quality Over Time] [Model Comparison] [Ops Health]     │
│  [Feedback Loop]     [Alerts]                            │
└──────────────────────┬───────────────────────────────────┘
                       │ OTLP
┌──────────────────────▼───────────────────────────────────┐
│              OTel Collector / Backend                     │
│         (Prometheus + Tempo + Grafana Cloud)              │
└──────┬───────────────┬───────────────────┬───────────────┘
       │               │                   │
┌──────▼──────┐ ┌──────▼──────┐ ┌──────────▼──────────┐
│  Benchmark  │ │   Gavel     │ │  Feedback Collector  │
│  Harness    │ │  (prod use) │ │  (CLI + GitHub sync) │
│             │ │             │ │                      │
│ corpus/ ────┤ │ OTel spans  │ │ feedback.json ──────┤
│ gen-corpus  │ │ OTel metrics│ │ sync-feedback       │
│ LLM judge   │ │             │ │ import-feedback ────►│
│ scoring     │ │             │ │     (→ corpus)       │
└─────────────┘ └─────────────┘ └──────────────────────┘
```

## Layer 1: Benchmark Harness & Labeled Corpus

### Corpus Structure

Directory-based corpus under `benchmarks/corpus/` where each test case is a directory containing source code and ground truth:

```
benchmarks/
├── corpus/
│   ├── go/
│   │   ├── sql-injection/
│   │   │   ├── source.go          # Code with known issue
│   │   │   ├── expected.yaml      # Ground truth findings
│   │   │   └── metadata.yaml      # Category, difficulty, notes
│   │   ├── error-handling-good/
│   │   │   ├── source.go          # Clean code (no findings expected)
│   │   │   └── expected.yaml      # Empty — validates false positive rate
│   │   └── ...
│   ├── python/
│   └── typescript/
├── cmd/gavel-bench/               # Benchmark runner CLI
├── internal/bench/                 # Scoring, corpus loading, LLM judge
└── results/                        # Historical benchmark results (gitignored)
```

Initial corpus covers Go, Python, and TypeScript (top 3 most used languages).

### Expected Findings Manifest (`expected.yaml`)

```yaml
findings:
  - rule_id: "SEC001"           # Or "any" for LLM findings without specific rule
    severity: error
    line_range: [15, 20]        # Approximate — allows ±5 line tolerance
    category: sql-injection
    must_find: true             # false = acceptable but not required
  - rule_id: "any"
    severity: warning
    line_range: [30, 35]
    category: error-handling
    must_find: false

false_positives: 0              # Expected noise count (0 for clean corpus entries)
```

### Scoring Metrics

Per-case and aggregate metrics computed from multi-run averaging (3+ runs):

| Metric | Formula | Purpose |
|---|---|---|
| True Positives (TP) | Expected findings found (within line tolerance) | Core accuracy |
| False Negatives (FN) | Expected `must_find` findings missed | Recall signal |
| False Positives (FP) | Findings not matching any expected finding | Noise signal |
| Precision | TP / (TP + FP) | How many findings are real |
| Recall | TP / (TP + FN) | How many real issues are found |
| F1 Score | 2 × (P × R) / (P + R) | Balanced quality metric |
| Confidence calibration | Mean confidence(TP) vs mean confidence(FP) | Model calibration |
| Hallucination rate | Findings with wrong paths or nonexistent lines | Path/line accuracy |

### Synthetic Corpus Generation

A generator (`benchmarks/cmd/gen-corpus/`) that produces test cases programmatically:

- **Vulnerability injection:** Takes clean code templates, injects known vulnerability patterns at specified locations
- **Severity calibration:** Cases at each severity level to test severity assignment accuracy
- **Language parity:** Same logical vulnerability in Go, Python, and TypeScript for cross-language consistency
- **Negative cases:** Clean, well-written code that should produce zero findings

### LLM-as-Judge

Alternative scoring mode where a stronger model (e.g. Claude Opus) evaluates each finding:

- Is the finding describing a real issue?
- Is the severity appropriate?
- Is the recommendation actionable?
- Is the line reference accurate?

Each finding receives a quality score (1-5) and label (valid/noise/hallucination). Runs as a second pass after the harness, so the same run can be scored both deterministically and by LLM judge.

### Benchmark Runner CLI

```bash
# Run full benchmark suite
gavel-bench run --corpus benchmarks/corpus/ --provider anthropic --model claude-sonnet-4

# Run with LLM judge scoring
gavel-bench run --corpus benchmarks/corpus/ --judge --judge-model claude-opus-4-6

# Compare two runs
gavel-bench compare --baseline results/2026-03-01.json --current results/2026-03-03.json

# Generate synthetic corpus
gavel-bench gen-corpus --languages go,python,typescript --output benchmarks/corpus/synthetic/
```

## Layer 2: OTel Integration

Builds on the existing `docs/opentelemetry-plan.md`. Key additions for benchmarking:

### Benchmark-Specific Attributes

When running in benchmark mode, OTel signals include:

| Attribute | Description |
|---|---|
| `gavel.benchmark.run_id` | Unique ID for this benchmark run |
| `gavel.benchmark.corpus_case` | Which corpus test case is being analyzed |
| `gavel.benchmark.iteration` | Run number (for multi-run averaging) |
| `gavel.benchmark.baseline` | Whether this is a baseline or variant run |

### Quality Metrics (OTel Instruments)

Beyond operational metrics from the OTel plan, the benchmark harness emits:

| Instrument | Type | Description |
|---|---|---|
| `gavel.bench.precision` | Gauge | Current precision score |
| `gavel.bench.recall` | Gauge | Current recall score |
| `gavel.bench.f1` | Gauge | Current F1 score |
| `gavel.bench.hallucination_rate` | Gauge | % of findings with wrong paths/lines |
| `gavel.bench.noise_rate` | Gauge | FP / total findings |
| `gavel.bench.confidence_calibration` | Gauge | Correlation between confidence and TP |

Tagged by model/provider/persona/corpus for multi-dimensional analysis.

### Implementation Phasing (Priority for Benchmarking)

1. **Phase 1** (telemetry init) — prerequisite for everything
2. **Phase 3** (metrics) — needed for quality gauges and operational counters
3. **Phase 2** (tracing) — useful but not blocking for benchmarks
4. **Phase 4-5** (LSP, logging) — deferred

## Layer 3: Feedback Collection & Real-World Signal

### CLI Feedback Command

```bash
# Mark a finding as useful
gavel feedback --result <id> --finding 3 --verdict useful

# Mark as noise
gavel feedback --result <id> --finding 7 --verdict noise --reason "style preference"

# Mark as wrong (hallucination)
gavel feedback --result <id> --finding 2 --verdict wrong --reason "file path doesn't exist"

# Bulk feedback via interactive TUI
gavel feedback --result <id> --interactive
```

Feedback stored alongside SARIF in the store:

```
.gavel/results/<id>/
├── analysis.sarif
├── verdict.json
└── feedback.json
```

### `feedback.json` Structure

```json
{
  "result_id": "20260303-abc123",
  "feedback": [
    {
      "finding_index": 3,
      "rule_id": "SEC001",
      "verdict": "useful",
      "reason": "",
      "timestamp": "2026-03-03T10:30:00Z"
    }
  ]
}
```

### GitHub SARIF Integration

A `gavel sync-feedback` command polls GitHub Code Scanning API for passive signals:

- **Dismissed alerts** → `noise` or `wrong` feedback
- **Fixed alerts** → `useful` feedback
- **Open alerts** → ambiguous (duration-open as weak signal)

```bash
gavel sync-feedback --github --repo owner/repo --result <id>
```

Uses `gh api repos/{owner}/{repo}/code-scanning/alerts` to pull states and map via SARIF fingerprints.

### Feedback → Corpus Refinement

Aggregated feedback enriches the benchmark corpus:

1. **High-confidence noise patterns** (repeatedly marked noise) → negative test cases
2. **Consistently useful findings** → positive test cases with `must_find: true`
3. **Hallucination patterns** → inform synthetic corpus generation

```bash
gavel-bench import-feedback --feedback-dir .gavel/results/ --output benchmarks/corpus/proposed/
```

### Feedback Telemetry

| Instrument | Type | Description |
|---|---|---|
| `gavel.feedback.count` | Counter | Feedback submitted, by verdict |
| `gavel.feedback.noise_rate` | Gauge | % findings marked noise (rolling) |
| `gavel.feedback.useful_rate` | Gauge | % findings marked useful (rolling) |

## CI/Nightly Integration

### GitHub Actions Workflow

Nightly and on-release benchmark runs:

```yaml
name: Nightly Benchmark
on:
  schedule:
    - cron: '0 3 * * *'
  workflow_dispatch:
  push:
    tags: ['v*']

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: task build
      - run: |
          gavel-bench run \
            --corpus benchmarks/corpus/ \
            --provider anthropic \
            --model claude-sonnet-4 \
            --runs 3 \
            --output results/$(date +%Y-%m-%d).json
      - run: |
          gavel-bench run \
            --corpus benchmarks/corpus/ \
            --judge \
            --judge-model claude-opus-4-6 \
            --input results/$(date +%Y-%m-%d).json
      - uses: actions/upload-artifact@v4
        with:
          name: benchmark-results
          path: results/
```

### Multi-Model Comparison

```bash
for model in claude-sonnet-4 claude-haiku-4-5 gpt-5.2; do
  gavel-bench run --corpus benchmarks/corpus/ --model $model \
    --output results/$(date +%Y-%m-%d)-$model.json
done
gavel-bench compare results/$(date +%Y-%m-%d)-*.json
```

### Grafana Dashboard

Four panels powered by OTel metrics:

| Panel | Metrics | Purpose |
|---|---|---|
| Quality Over Time | F1, precision, recall (line chart) | Detect prompt regressions |
| Model Comparison | F1 by model, cost per finding, latency | Inform model selection |
| Operational Health | p50/p95 latency, error rate, token usage | Production monitoring |
| Feedback Loop | Useful/noise/wrong rates from user feedback | Real-world quality signal |

Alert rules:
- F1 drops >5% from 7-day average → alert
- Hallucination rate exceeds 10% → alert
- Noise rate (from feedback) exceeds 30% → alert

### Results Format

```json
{
  "run_id": "2026-03-03-nightly",
  "timestamp": "2026-03-03T03:00:00Z",
  "model": "claude-sonnet-4",
  "provider": "anthropic",
  "corpus_version": "v1.2",
  "runs": 3,
  "aggregate": {
    "precision": 0.82,
    "recall": 0.91,
    "f1": 0.86,
    "hallucination_rate": 0.04,
    "noise_rate": 0.12,
    "confidence_calibration": 0.78,
    "mean_latency_ms": 2340,
    "total_tokens": 45000,
    "total_cost_usd": 0.23
  },
  "per_case": []
}
```

## Decisions

- **Corpus languages:** Go, Python, TypeScript (top 3). Expand later.
- **Multi-run averaging:** 3 runs minimum, consistent with existing A/B methodology.
- **LLM-as-judge model:** Claude Opus (strongest available) for quality evaluation.
- **OTel backend:** OTLP export to any compatible backend — user choice.
- **CI cadence:** Nightly + on-release. Not PR-gated.
- **Feedback storage:** Local filesystem (`.gavel/results/`), consistent with existing store pattern.
- **Line tolerance:** ±5 lines for matching findings to expected locations.
