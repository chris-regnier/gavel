# Model Benchmark Spec Sheet

**Date:** 2026-04-02
**Status:** Draft
**Goal:** A `gavel bench` subcommand that produces a spec sheet comparing hosted models across quality, latency, and cost — so users can evaluate tradeoffs when choosing a provider/model for their CI pipeline.

## Motivation

Gavel supports multiple LLM providers and models via OpenRouter. Users need concrete data to decide which model fits their quality/cost/speed requirements. Today there's no standardized way to compare models against Gavel's actual analysis task. This feature closes that gap with a reusable, first-class benchmark command.

## CLI Interface

### `gavel bench`

Runs a multi-model comparison benchmark.

```
gavel bench [flags]
  --runs N          Number of iterations per model per test case (default 3)
  --parallel N      Max concurrent models (default 4)
  --models m1,m2    Comma-separated OpenRouter model IDs (overrides defaults)
  --corpus dir      Path to corpus directory (default: embedded benchmarks/corpus/)
  --output dir      Directory for result artifacts (default: .gavel/bench/)
  --format json|yaml  Structured output format (default: json)
```

### `gavel bench models`

Queries OpenRouter for available programming models.

```
gavel bench models [flags]
  --sort price|name   Sort order (default: price)
  --limit N           Max models to show (default: 20)
```

Output: a table of model ID, input/output pricing, context window, and provider — so users can curate a `--models` list.

### Default Model List

Shipped in code as a constant list, overridable via `--models`. Spans premium through budget tiers:

| Tier | Model ID | Input $/M | Output $/M |
|------|----------|-----------|------------|
| Premium | `anthropic/claude-opus-4.6` | $5.00 | $25.00 |
| Premium | `openai/gpt-5.3-codex` | $1.75 | $14.00 |
| Value | `google/gemini-3.1-pro-preview` | $2.00 | $12.00 |
| Value | `anthropic/claude-sonnet-4.6` | $3.00 | $15.00 |
| Fast | `anthropic/claude-haiku-4.5` | $1.00 | $5.00 |
| Fast | `google/gemini-3-flash-preview` | $0.50 | $3.00 |
| Budget | `deepseek/deepseek-v3.2` | $0.26 | $0.38 |
| Budget | `qwen/qwen3-coder-next` | $0.12 | $0.75 |

Note: Pricing is as of 2026-04-02 and may change. The `gavel bench models` command provides current pricing from OpenRouter.

## Architecture

### Approach: New comparison engine alongside existing harness

A new `internal/bench/compare.go` engine handles multi-model comparison. It reuses the existing corpus loader and scorer from `internal/bench/` but has its own orchestration for concurrency, rate limiting, and aggregation. The existing `RunBenchmark()` in `runner.go` stays untouched for single-model prompt-tuning experiments.

**Rationale:** Model comparison has different concerns than prompt tuning — concurrency across models, rate limiting, cost tracking, latency measurement, spec-sheet aggregation. Clean separation avoids coupling the two workflows.

### Core Types

```go
// CompareResult holds all benchmark data for a single model.
type CompareResult struct {
    ModelID   string
    Runs      []RunResult       // per-run raw data
    Quality   QualityMetrics    // precision, recall, F1 (from existing scorer)
    Latency   LatencyMetrics    // p50, p95, p99, mean per call
    Cost      CostMetrics       // actual tokens in/out, computed cost
}

type QualityMetrics struct {
    Precision         float64
    Recall            float64
    F1                float64
    HallucinationRate float64
    MeanConfidence    float64
    Variance          float64  // finding count variance across runs
}

type LatencyMetrics struct {
    MeanMs int64
    P50Ms  int64
    P95Ms  int64
    P99Ms  int64
}

type CostMetrics struct {
    InputTokensTotal  int64
    OutputTokensTotal int64
    InputPricePerM    float64
    OutputPricePerM   float64
    TotalUSD          float64
    PerFileAvgUSD     float64
}
```

### Orchestration Flow

1. Load corpus via existing `bench.LoadCorpus()`
2. Resolve model list — use defaults or parse `--models` flag, validate each against OpenRouter API
3. For each model (up to `--parallel` concurrently):
   a. Create a temporary provider config with that model's OpenRouter ID
   b. Run all corpus cases x N runs sequentially within that model's goroutine
   c. Capture per-call: wall-clock latency, actual input/output token counts (from OpenRouter response body `usage.prompt_tokens` / `usage.completion_tokens`), raw findings
4. Run real-world file set (latency/cost only, no scoring)
5. Score each model's corpus findings against expected via existing `bench.Score()`
6. Aggregate into `CompareResult` per model
7. Write structured output (JSON/YAML) and top-line summary (Markdown)

### Rate Limiting & Concurrency

- Each model goroutine runs its own sequential loop of corpus cases x runs
- The `--parallel` flag controls how many model goroutines run concurrently (default 4)
- On HTTP 429 responses, exponential backoff (already configured in BAML retry policy: max 2 retries, 300-10000ms delays)
- Model validation happens before any benchmark calls, so typos are caught early

### Token Counting

OpenRouter returns actual token counts in the response body (`usage.prompt_tokens` / `usage.completion_tokens`). We capture these directly rather than estimating from character counts. This gives accurate cost computation.

**Implementation note:** The current `BAMLClient` interface returns only `[]Finding`. The comparison engine needs token usage alongside findings. Rather than changing the shared interface, the bench package will wrap the BAML call with its own HTTP-level instrumentation or extend the return type within the bench package only. The exact mechanism is an implementation detail — the key constraint is that we get real token counts, not estimates.

## Model Discovery

`gavel bench models` calls `GET https://openrouter.ai/api/v1/models?category=programming` with the user's `OPENROUTER_API_KEY`.

Lives in `internal/bench/models.go`. Fetches, filters to text-output models, sorts by price or name, prints a table. The same validation function is reused by the comparison engine to reject invalid `--models` input before spending money.

No caching of the model list — it's a quick API call and models change frequently.

## Output Artifacts

### Structured Results

Written to `.gavel/bench/<timestamp>/results.json` (or `.yaml`).

```json
{
  "metadata": {
    "timestamp": "2026-04-02T...",
    "runs_per_model": 5,
    "corpus_size": 10,
    "corpus_hash": "sha256:...",
    "gavel_version": "0.3.0"
  },
  "models": [
    {
      "model_id": "anthropic/claude-opus-4.6",
      "tier": "premium",
      "quality": {
        "precision": 0.92,
        "recall": 0.88,
        "f1": 0.90,
        "hallucination_rate": 0.04,
        "mean_confidence": 0.87,
        "variance": 1.8
      },
      "latency": {
        "mean_ms": 3420,
        "p50_ms": 3100,
        "p95_ms": 5800,
        "p99_ms": 7200
      },
      "cost": {
        "input_tokens_total": 44000,
        "output_tokens_total": 24000,
        "input_price_per_m": 5.00,
        "output_price_per_m": 25.00,
        "total_usd": 0.82,
        "per_file_avg_usd": 0.016
      },
      "runs": []
    }
  ],
  "realworld": [
    {
      "model_id": "anthropic/claude-opus-4.6",
      "files": [
        {
          "path": "benchmarks/realworld/gin-handler.go",
          "latency_ms": 4200,
          "input_tokens": 2100,
          "output_tokens": 1400
        }
      ]
    }
  ]
}
```

### Top-Line Summary

Written to `docs/model-benchmarks.md`. A concise comparison table with one row per model showing F1, mean latency, cost per file, and a recommended-use-case column. Short intro paragraph explaining what the numbers mean and how to re-run. Updated each time the benchmark is run.

This file contains only top-line findings — users who want deeper analysis use the structured JSON.

## Real-World File Set

A curated set of real-world files for latency/cost measurement under realistic conditions. These are larger, more representative files — not scored for quality, just measured for performance.

**Location:** `benchmarks/realworld/` with a `manifest.yaml` listing each file, its source, and why it's included.

**Selection criteria:**
- 5-8 files across Go, Python, TypeScript
- Mix of sizes: ~50 LOC, ~200 LOC, ~500 LOC
- Sourced from popular open-source repos (with attribution)
- Chosen to exercise different analysis patterns (web handlers, data processing, CLI code)

**Usage:** `gavel bench` runs both corpus (scored) and real-world (latency/cost only) by default. The results JSON includes both sections clearly separated.

Documented in `docs/model-benchmarks.md` so users can substitute their own files for local experimentation.

## Cost Estimate

For the initial experiment (5 runs, 8 models, 10 corpus cases + 5 real-world files):

- Quality benchmark (corpus): ~$23
- Real-world files (latency/cost): ~$35-55
- **Estimated total: ~$60-80**

Default 3-run configuration would cost ~$35-50.

## New Files

| File | Purpose |
|------|---------|
| `cmd/gavel/bench.go` | CLI subcommand wiring |
| `internal/bench/compare.go` | Multi-model comparison engine |
| `internal/bench/models.go` | OpenRouter model discovery |
| `internal/bench/compare_test.go` | Tests for comparison engine |
| `benchmarks/realworld/` | Curated real-world file set |
| `benchmarks/realworld/manifest.yaml` | File metadata and attribution |
| `docs/model-benchmarks.md` | Top-line summary (generated) |

## Modified Files

| File | Change |
|------|--------|
| `cmd/gavel/root.go` | Register `bench` subcommand |

## Out of Scope

- Comparing personas across models (existing harness handles this)
- Comparing Rego policies (verdicts are independent of model choice)
- Non-OpenRouter providers (Ollama, direct Anthropic, Bedrock) — could be added later but this experiment focuses on the unified OpenRouter API
- Automated model selection / recommendation engine — the spec sheet informs human decisions
