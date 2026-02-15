# OpenTelemetry Plan Review

Review of `docs/opentelemetry-plan.md` against OTel specification best practices, Grafana Mimir compatibility, and Grafana Beyla applicability.

---

## Critical Issues

### 1. Metric Counter Names Will Produce Double Suffixes in Mimir

**Severity: High** — Will cause confusing metric names in any Prometheus-compatible backend.

The plan names counters with terminal words like `total`, `hits`, `misses`:

```
gavel.analyses.total   → Mimir converts → gavel_analyses_total_total  (DOUBLE SUFFIX)
gavel.cache.hits       → Mimir converts → gavel_cache_hits_total
gavel.cache.misses     → Mimir converts → gavel_cache_misses_total
gavel.cache.stale      → Mimir converts → gavel_cache_stale_total
```

Mimir (and any Prometheus-compatible backend) auto-appends `_total` to monotonic counters during OTLP-to-Prometheus translation. The OTel convention is to **not include type suffixes** in instrument names.

**Fix:** Rename all counters to omit implicit type/plural suffixes:

| Current (plan) | Corrected | Mimir result |
|---|---|---|
| `gavel.analyses.total` | `gavel.analysis.count` | `gavel_analysis_count_total` |
| `gavel.analyses.errors` | `gavel.analysis.error.count` | `gavel_analysis_error_count_total` |
| `gavel.findings.total` | `gavel.finding.count` | `gavel_finding_count_total` |
| `gavel.tokens.input` | `gavel.token.input` | `gavel_token_input_total` |
| `gavel.tokens.output` | `gavel.token.output` | `gavel_token_output_total` |
| `gavel.cache.hits` | `gavel.cache.hit` | `gavel_cache_hit_total` |
| `gavel.cache.misses` | `gavel.cache.miss` | `gavel_cache_miss_total` |
| `gavel.cache.stale` | `gavel.cache.stale` | `gavel_cache_stale_total` |

### 2. Histograms Missing OTel `unit` Field — Mimir Won't Append Unit Suffix

**Severity: High** — Histogram names will lack unit context in Prometheus/Mimir.

The plan specifies `gavel.analysis.duration` with "buckets: 10, 50, 100, 500, 1000, 5000, 30000" (milliseconds). But it does not mention setting the OTel instrument `unit` field. Mimir auto-appends the unit as a suffix (e.g., `_seconds`, `_bytes`) from the OTel `unit` metadata. Without it, the Prometheus name will be `gavel_analysis_duration_bucket` with no indication of the unit.

**Fix:** Set the `unit` field on all histogram instruments. OTel semantic conventions prefer seconds for duration:

```go
meter.Float64Histogram("gavel.analysis.duration",
    metric.WithUnit("s"),
    metric.WithDescription("Analysis duration"),
)
// Mimir result: gavel_analysis_duration_seconds_bucket, _sum, _count
```

Adjust bucket boundaries to seconds: `{0.01, 0.05, 0.1, 0.5, 1, 5, 30}` instead of ms.

### 3. GenAI Span Attributes Use Deprecated Names

**Severity: Medium** — `gen_ai.system` was replaced by `gen_ai.provider.name` in OTel semconv.

The plan specifies:
> `gen_ai.system` (provider name)

The current GenAI semantic conventions (semconv 1.29.0+) renamed this:

| Deprecated | Current |
|---|---|
| `gen_ai.system` | `gen_ai.provider.name` |

Additionally, the plan is missing `gen_ai.operation.name` which is **Required** at span creation. The LLM call span name should follow the convention `{gen_ai.operation.name} {gen_ai.request.model}` — e.g., `chat claude-sonnet-4`, not `gavel.llm.call`.

**Fix:** Update the LLM span definition:

| Span Name | Key Attributes |
|---|---|
| `chat {model}` | `gen_ai.operation.name=chat`, `gen_ai.provider.name` (not `system`), `gen_ai.request.model`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens` |

### 4. Sampler Not Wrapped in `ParentBased`

**Severity: Medium** — Will break trace consistency in distributed scenarios.

The plan's `sample_rate: 1.0` config maps to a `TraceIDRatioBased` sampler, but does not specify wrapping it in `ParentBased()`. Without `ParentBased`, the sampler ignores parent sampling decisions. If a parent trace was sampled, a child service using bare `TraceIDRatioBased(0.1)` might drop 90% of its spans from that trace, producing broken/incomplete traces.

**Fix:** The `Init()` implementation must use:
```go
trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(cfg.SampleRate)))
```

This is the OTel-recommended default (`parentbased_traceidratio`). Document this in the config section.

---

## Moderate Issues

### 5. Span Names Use Dot-Delimited Hierarchy — Not Recommended

The plan uses `gavel.analyze`, `gavel.tier.instant`, `gavel.cache.lookup`, etc. The OTel specification recommends span names follow **`{verb} {object}`** patterns (per the [August 2025 naming guidance](https://opentelemetry.io/blog/2025/how-to-name-your-spans/)):

- HTTP: `GET /api/users/:id`
- RPC: `UserService/GetUser`
- DB: `SELECT mydb.users`
- GenAI: `chat claude-sonnet-4`

Dots in span names are only conventional where the domain naturally uses them (e.g., RPC service names). For internal application operations, prefer descriptive verb-object names.

**Suggested revision:**

| Current (plan) | Recommended | Rationale |
|---|---|---|
| `gavel.analyze` | `analyze code` | Verb-object |
| `gavel.config.load` | `load config` | Verb-object |
| `gavel.input.read` | `read input` | Verb-object |
| `gavel.tier.instant` | `run instant tier` | Verb-object |
| `gavel.tier.comprehensive` | `run comprehensive tier` | Verb-object |
| `gavel.analyze_file` | `analyze file` | Verb-object; file path goes in attribute |
| `gavel.llm.call` | `chat {model}` | GenAI convention |
| `gavel.evaluate` | `evaluate rego` | Verb-object |
| `gavel.store.write_sarif` | `write sarif` | Verb-object |
| `gavel.cache.lookup` | `cache lookup` | Acceptable noun-phrase |

Note: dots in *attribute* names (`gavel.tier`, `gavel.file_path`) are correct — that is the OTel attribute naming convention.

### 6. Missing `service.version` Resource Attribute

The plan mentions `service_name: "gavel"` in config but omits `service.version`. OTel best practice is to always include both:

```go
resource.NewWithAttributes(semconv.SchemaURL,
    semconv.ServiceName("gavel"),
    semconv.ServiceVersion(version), // from build info or ldflags
)
```

Also missing: `resource.Merge(resource.Default(), ...)` to include auto-detected SDK attributes (`telemetry.sdk.name`, `telemetry.sdk.language`, `telemetry.sdk.version`).

### 7. Propagator Should Include Baggage

The plan specifies:
> Set `propagation.TraceContext{}` as global propagator.

Best practice is a composite propagator with both TraceContext and Baggage:
```go
otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
    propagation.TraceContext{},
    propagation.Baggage{},
))
```

Even if baggage isn't used initially, including it in the propagator costs nothing and enables future use (e.g., propagating `gavel.run_id` to LLM provider calls).

### 8. Tracer Instrumentation Scope Name Should Be Import Path

The plan uses:
```go
tracer := otel.Tracer("gavel")
```

OTel convention is for the instrumentation scope name to be the **Go import path** of the package performing the instrumentation:
```go
tracer := otel.Tracer("github.com/chris-regnier/gavel/cmd/gavel")
```

Or define per-package tracers:
```go
// internal/analyzer/tiered.go
var tracer = otel.Tracer("github.com/chris-regnier/gavel/internal/analyzer")
```

This helps identify which instrumentation library produced which spans.

### 9. Missing `otelhttp` for Outgoing LLM HTTP Calls

The BAML client makes HTTP calls to LLM providers (Anthropic, OpenRouter, OpenAI, Bedrock). If these use `net/http` under the hood, wrapping the transport with `otelhttp.NewTransport()` from `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` would automatically produce HTTP client spans with standard `http.*` attributes (status code, URL, method), DNS timing, etc. This is complementary to the GenAI-level spans — the HTTP span captures transport-level detail while the GenAI span captures semantic detail.

This depends on whether BAML's generated client exposes a configurable `http.Client`. If it does, this is low-effort, high-value. Worth investigating in Phase 2.

### 10. Attribute Cardinality Warning Missing for Metrics

The plan lists `gavel.file_path` as a span attribute (fine — traces are high cardinality by nature) but the Phase 3 metrics section doesn't explicitly warn against putting unbounded attributes on metrics. Mimir's default cardinality limit is **150,000 active series per tenant**. A histogram with 10 buckets and a `file_path` label with 1,000 unique values = 30,000 series for one metric.

**Fix:** Add a cardinality guidance section to Phase 3:
- **Safe metric attributes** (bounded): `gavel.tier`, `gavel.provider`, `gavel.model`, `gavel.persona`, `gavel.cache.result`, `gavel.decision`
- **Unsafe for metrics** (unbounded): `gavel.file_path`, `gavel.cache.key`, `gavel.error.message`, `gavel.rule.id` (if many custom rules)

Use these unbounded values only on span attributes, never on metric attributes.

---

## Minor Issues

### 11. CLI Shutdown Should Handle OS Signals

For a CLI tool, the plan should mention using `signal.NotifyContext` so that `Ctrl+C` still triggers a clean OTel shutdown (flushing buffered spans/metrics):

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
```

Without this, an interrupted `gavel analyze` run loses all buffered telemetry.

### 12. `PeriodicReader` Interval for CLI vs LSP

The plan notes the 60s `PeriodicReader` interval and that `Shutdown()` flushes. For short CLI runs this is correct. However, the plan should explicitly recommend a **shorter interval for the LSP server** (e.g., 15s) since it's long-running and operators want near-real-time metrics. This could be a separate config value or auto-detected based on the subcommand.

Alternatively, mention `ManualReader` as an option for CLI mode — it gives explicit control over when collection happens and avoids the background goroutine entirely.

### 13. `evaluator.NewEvaluator` Context Propagation Gap

The plan's risk table mentions auditing `context.Background()` call sites, but doesn't call out the specific issue: `evaluator.NewEvaluator()` creates `context.Background()` internally for `rego.PrepareForEval(ctx)`. This should be changed to accept an external `ctx` parameter so the Rego compilation step appears in the trace. Currently:

```go
func NewEvaluator(policyDir string) (*Evaluator, error) {
    ctx := context.Background()  // <-- breaks trace propagation
```

Should become:
```go
func NewEvaluator(ctx context.Context, policyDir string) (*Evaluator, error) {
```

### 14. Missing `OTEL_EXPORTER_OTLP_HEADERS` Env Var

The plan lists several `OTEL_*` env vars but omits `OTEL_EXPORTER_OTLP_HEADERS`, which is the standard way to pass auth tokens (e.g., for Grafana Cloud). This is particularly important because the plan's YAML `headers` field serves the same purpose. Should document that `OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic ...` overrides the config file headers.

Note: most of the `OTEL_*` variables are actually handled natively by the OTel SDK's exporter packages — the `Init()` function doesn't need to manually parse them. This should be documented to avoid reimplementing what the SDK already provides.

---

## Beyla Assessment

### Not Applicable for `gavel analyze` (CLI Mode)

Grafana Beyla (now OpenTelemetry eBPF Instrumentation / OBI) is architecturally unsuitable for Gavel's primary CLI mode:

- **Process lifetime**: Beyla needs to discover the process, inspect the binary, and attach eBPF uprobes. A `gavel analyze` run lasting 2-30 seconds may complete before probes are fully attached.
- **No listening port**: Beyla discovers services by listening port. CLI tools don't listen.
- **Network-centric model**: Beyla captures HTTP/gRPC request-response patterns. Gavel's workload (read files, call LLM API, write SARIF, evaluate Rego) is mostly local I/O + a few outgoing HTTP calls. Manual SDK instrumentation captures this far more richly.
- **eBPF overhead**: Kernel-level probe setup overhead dominates for a short-lived process.

**Verdict**: The plan is correct to not mention Beyla for the `analyze` command. Direct OTel SDK instrumentation is the right approach.

### Marginal Value for LSP Server (Phase 4)

For the long-running LSP server, Beyla's applicability is limited:

- **LSP protocol uses stdio/pipes**, not HTTP — Beyla cannot instrument editor-to-server communication.
- Beyla could capture outgoing HTTP calls to LLM APIs (automatic RED metrics), but this is the same data the manual GenAI spans already capture with richer attributes.
- The high-value LSP metrics (per-file analysis latency, cache hit rates, diagnostic publish counts) are business logic that Beyla cannot observe.

**Verdict**: Beyla provides negligible incremental value over the planned manual instrumentation. Not worth adding as a dependency or recommendation.

### Recommendation

Add a brief note to the plan acknowledging that Beyla was evaluated and deemed unsuitable:

> **Beyla/eBPF auto-instrumentation**: Evaluated and not recommended. Gavel's CLI mode is too short-lived for eBPF probe attachment, and the LSP server's stdio-based protocol is outside Beyla's HTTP/gRPC instrumentation scope. Manual OTel SDK instrumentation provides superior coverage for all Gavel workloads.

---

## Mimir Compatibility Summary

| Aspect | Plan Status | Action Needed |
|---|---|---|
| Metric name format (dots) | Correct | None |
| Counter `_total` suffix avoidance | **Wrong** | Rename counters (Issue #1) |
| Histogram unit field | **Missing** | Add `unit` to all histograms (Issue #2) |
| Explicit bucket histograms | Correct | None (safe default) |
| Exponential histograms | Not mentioned | Note: requires Mimir config flag if used |
| Label cardinality | Not addressed | Add guidance (Issue #10) |
| Resource → `target_info` | Not mentioned | Document for users querying in Grafana |
| `service.name` / `service.version` | Partial | Add `service.version` (Issue #6) |

---

## Overall Assessment

The plan's architecture is sound — phased rollout, zero-cost-when-disabled, additive to existing metrics, and correct identification of instrumentation boundaries. The main gaps are:

1. **Naming conventions** need revision to avoid Mimir double-suffixes and follow OTel span naming guidance.
2. **GenAI semantic conventions** need updating to the current spec (deprecated attributes, missing required fields).
3. **Sampler wrapping**, **propagator composition**, and **resource attributes** need minor corrections per OTel best practices.
4. **Cardinality guidance** is missing — important for anyone targeting Prometheus/Mimir.
5. **Beyla** is correctly absent from the plan but should be explicitly noted as evaluated-and-rejected for documentation completeness.

None of these issues require architectural changes — they are naming, configuration, and documentation corrections that can be addressed before implementation begins.
