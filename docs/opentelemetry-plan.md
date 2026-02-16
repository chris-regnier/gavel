# OpenTelemetry Integration Plan for Gavel

## Overview

This document describes a plan for adding OpenTelemetry (OTel) compatible telemetry to Gavel. The goal is to provide distributed tracing, metrics, and structured logging that follow the OTel standard, enabling operators to export telemetry to any OTel-compatible backend (Jaeger, Grafana Tempo, Datadog, Honeycomb, etc.).

## Current State

Gavel already has:
- **Custom metrics system** (`internal/metrics/`): `Collector`, `Recorder`, `AnalysisEvent` structs, JSON/CSV export, context-based recorder injection.
- **OTel SDK in go.mod** (indirect, v1.39.0): Pulled in transitively by BAML/OPA dependencies — `go.opentelemetry.io/otel`, `otel/trace`, `otel/metric`, `otel/sdk`.
- **Context propagation**: `context.Context` is threaded through the entire analysis pipeline (analyze → tiered analyzer → BAML client → evaluator → store).
- **Interface boundaries**: `BAMLClient`, `Store`, `Check` interfaces make it straightforward to add instrumented wrappers or inject spans at boundaries.

What's missing:
- No OTel TracerProvider/MeterProvider initialization.
- No spans anywhere in the codebase.
- No OTLP exporter configuration.
- Custom metrics (`internal/metrics/`) are not bridged to OTel Metrics API.
- No structured logging (only scattered `slog.Warn` and `log.Printf`).

## Design Principles

1. **Zero-cost when disabled**: If no OTel endpoint is configured, all instrumentation should be no-ops (OTel SDK's default behavior with noop providers).
2. **Configuration-driven**: OTel endpoint, sampling, and export format controlled via config/env vars, not code changes.
3. **Preserve existing metrics**: The custom `internal/metrics/` system continues to work. OTel integration is additive — it does not replace the existing Collector/Recorder.
4. **Minimal API surface**: Use `go.opentelemetry.io/otel` global tracer/meter getters rather than passing providers through every function.
5. **Semantic conventions**: Follow [OTel semantic conventions](https://opentelemetry.io/docs/specs/semconv/) where applicable (e.g., `gen_ai.*` for LLM calls, `code.*` for file analysis).
6. **Mimir/Prometheus compatibility**: Metric instrument names must avoid type suffixes (`total`, `count`) that Prometheus auto-appends, and histograms must set the OTel `unit` field so backends can append unit suffixes correctly.
7. **Beyla not applicable**: Grafana Beyla (eBPF auto-instrumentation) was evaluated and is unsuitable — CLI mode is too short-lived for probe attachment, and the LSP server's stdio protocol is outside Beyla's HTTP/gRPC scope. Manual OTel SDK instrumentation provides superior coverage for all Gavel workloads.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  cmd/gavel/analyze.go                                   │
│  ┌─────────────────────────────────────────────────────┐│
│  │ initTelemetry(cfg) → TracerProvider, MeterProvider  ││
│  │ defer shutdown()                                    ││
│  └─────────────────────────────────────────────────────┘│
│                         │                               │
│              Root span: "analyze code"                    │
│                         │                               │
│  ┌──────────┬───────────┼───────────┬──────────────┐    │
│  ▼          ▼           ▼           ▼              ▼    │
│ load      read    tiered-analyze  evaluate      write   │
│ config    input    ┌────┴────┐     rego        sarif    │
│                    ▼         ▼                           │
│          run instant   run comprehensive                │
│            tier          tier                            │
│          (regex+AST)   ┌─────┴─────┐                    │
│                       ▼            ▼                    │
│                 analyze file  analyze file               │
│                 + chat {model}  + chat {model}           │
└─────────────────────────────────────────────────────────┘
                         │
              OTLP gRPC/HTTP export
                         │
              ┌──────────▼──────────┐
              │  Collector Backend  │
              │  (Jaeger/Tempo/etc) │
              └─────────────────────┘
```

## Implementation Phases

---

### Phase 1: Telemetry Initialization & Configuration

**New package**: `internal/telemetry/telemetry.go`

This package owns OTel SDK lifecycle: provider creation, exporter wiring, and shutdown.

#### 1a. Configuration

Add a `Telemetry` section to the config:

```yaml
# .gavel/policies.yaml
telemetry:
  enabled: false                          # master switch
  endpoint: "localhost:4317"              # OTLP gRPC endpoint
  protocol: "grpc"                        # "grpc" or "http"
  insecure: true                          # TLS disabled (for local dev)
  service_name: "gavel"                   # OTel service name
  service_version: ""                     # defaults to build version via ldflags
  sample_rate: 1.0                        # 0.0-1.0, 1.0 = always sample
  headers: {}                             # custom headers (auth tokens, etc.)
```

Also support environment variable overrides per the OTel spec:
- `OTEL_EXPORTER_OTLP_ENDPOINT` → overrides `endpoint`
- `OTEL_EXPORTER_OTLP_PROTOCOL` → overrides `protocol`
- `OTEL_EXPORTER_OTLP_HEADERS` → overrides `headers` (e.g., `Authorization=Basic ...`)
- `OTEL_SERVICE_NAME` → overrides `service_name`
- `OTEL_TRACES_SAMPLER` → overrides sampler (use `parentbased_traceidratio`)
- `OTEL_TRACES_SAMPLER_ARG` → overrides `sample_rate` (e.g., `0.1`)
- `GAVEL_TELEMETRY_ENABLED=true` → overrides `enabled`

Environment variables take precedence over config file values. The standard `OTEL_*` variables are the canonical way to configure OTel in production environments; the config file is a convenience for project-level defaults.

**Important:** Most `OTEL_*` variables are handled natively by the OTel SDK exporter packages. The `Init()` function should not reimplement parsing for variables the SDK already consumes (e.g., `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_HEADERS`). Only `GAVEL_TELEMETRY_ENABLED` and the config-file values require custom handling.

#### 1b. Provider Setup

```go
// internal/telemetry/telemetry.go

package telemetry

// Init initializes OTel providers and returns a shutdown function.
// If telemetry is disabled, returns noop providers and a no-op shutdown.
func Init(ctx context.Context, cfg TelemetryConfig) (shutdown func(context.Context) error, err error)
```

Implementation:
1. If disabled, return immediately (noop providers are the OTel default).
2. Create an OTel `Resource` using `resource.Merge(resource.Default(), ...)` to combine SDK auto-detected attributes (`telemetry.sdk.name`, `telemetry.sdk.language`, `telemetry.sdk.version`) with application attributes:
   ```go
   res, err := resource.Merge(resource.Default(),
       resource.NewWithAttributes(semconv.SchemaURL,
           semconv.ServiceName(cfg.ServiceName),
           semconv.ServiceVersion(cfg.ServiceVersion), // from build info or ldflags
       ))
   ```
3. Create OTLP exporter (gRPC or HTTP based on `protocol`).
4. Create `TracerProvider` with `BatchSpanProcessor` and a `ParentBased` sampler wrapping `TraceIDRatioBased`:
   ```go
   trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(cfg.SampleRate)))
   ```
   **Critical:** Never use bare `TraceIDRatioBased` without `ParentBased` — it would ignore parent sampling decisions and produce broken distributed traces.
5. Create `MeterProvider` with `PeriodicReader` (60s default interval for CLI; 15s recommended for LSP).
6. Set as global providers via `otel.SetTracerProvider()` / `otel.SetMeterProvider()`.
7. Set a composite propagator with both TraceContext and Baggage:
   ```go
   otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
       propagation.TraceContext{},
       propagation.Baggage{},
   ))
   ```
   Including Baggage from the start costs nothing and enables future use (e.g., propagating `gavel.run_id` to LLM provider calls).
8. Return a combined `shutdown` function (using `errors.Join`) that flushes and shuts down both providers. Callers should use `signal.NotifyContext` to ensure clean shutdown on `SIGINT`/`SIGTERM`.

**New dependencies** (promote from indirect to direct):
```
go.opentelemetry.io/otel
go.opentelemetry.io/otel/trace
go.opentelemetry.io/otel/metric
go.opentelemetry.io/otel/sdk
go.opentelemetry.io/otel/sdk/metric
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp
```

**Files to create/modify:**
- Create `internal/telemetry/telemetry.go`
- Create `internal/telemetry/telemetry_test.go`
- Modify `internal/config/config.go` — add `TelemetryConfig` struct and `Telemetry` field to `Config`
- Modify `cmd/gavel/analyze.go` — call `telemetry.Init()` early, defer `shutdown()`

---

### Phase 2: Distributed Tracing (Spans)

Add spans at the pipeline boundaries to produce a trace for every `gavel analyze` invocation.

#### 2a. Root Span in `cmd/gavel/analyze.go`

Each package defines its own tracer using the Go import path as the instrumentation scope name (per OTel convention):

```go
var tracer = otel.Tracer("github.com/chris-regnier/gavel/cmd/gavel")
```

The root span uses verb-object naming:

```go
ctx, span := tracer.Start(ctx, "analyze code",
    trace.WithAttributes(
        attribute.String("gavel.input_scope", inputScope),
        attribute.Int("gavel.artifact_count", len(artifacts)),
        attribute.String("gavel.persona", cfg.Persona),
        attribute.String("gavel.provider", cfg.Provider.Name),
    ),
)
defer span.End()
```

On error, **always pair both calls** — `RecordError` alone does NOT mark the span as failed:
```go
span.RecordError(err)
span.SetStatus(codes.Error, err.Error())
```

Do not set `codes.Ok` on success — leave the status unset (OTel convention).

#### 2b. Tiered Analyzer Spans in `internal/analyzer/tiered.go`

Package-level tracer:
```go
var tracer = otel.Tracer("github.com/chris-regnier/gavel/internal/analyzer")
```

Each tier gets a child span with verb-object naming:

| Span Name | Key Attributes |
|---|---|
| `run instant tier` | `gavel.tier=instant`, `gavel.rule_count`, `gavel.finding_count` |
| `run fast tier` | `gavel.tier=fast`, `gavel.model`, `gavel.provider` |
| `run comprehensive tier` | `gavel.tier=comprehensive`, `gavel.model`, `gavel.provider` |

Per-file analysis within each tier creates a child span:

| Span Name | Key Attributes |
|---|---|
| `analyze file` | `gavel.file_path`, `gavel.file_size`, `gavel.line_count`, `gavel.tier` |

Note: `gavel.file_path` is safe on span attributes (traces are inherently high-cardinality). See Phase 3 cardinality guidance for why this must NOT be used on metric attributes.

#### 2c. LLM Call Spans in `internal/analyzer/bamlclient.go`

Each provider call gets a span following the [OTel GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-spans/). The span name follows the convention `{gen_ai.operation.name} {gen_ai.request.model}`:

| Span Name | Span Kind | Key Attributes |
|---|---|---|
| `chat {model}` (e.g., `chat claude-sonnet-4`) | CLIENT | See table below |

**Required attributes** (must be set at span creation):

| Attribute | Value |
|---|---|
| `gen_ai.operation.name` | `"chat"` |
| `gen_ai.request.model` | Model name from provider config (e.g., `claude-sonnet-4`) |
| `gen_ai.provider.name` | Provider identifier: `anthropic`, `openrouter`, `ollama`, `openai`, `bedrock` |

**Recommended attributes** (set after response):

| Attribute | Value |
|---|---|
| `gen_ai.response.model` | Actual model used in response (may differ from request) |
| `gen_ai.usage.input_tokens` | Number of input tokens |
| `gen_ai.usage.output_tokens` | Number of output tokens |
| `gen_ai.response.finish_reasons` | Why generation stopped (e.g., `["stop"]`) |

**Note:** `gen_ai.system` is **deprecated** (semconv 1.29.0+); use `gen_ai.provider.name` instead. Content recording (`gen_ai.input.messages`, `gen_ai.output.messages`) is opt-in by default due to privacy/storage concerns — not enabled in this plan.

Errors from provider calls are recorded on the span with both `span.RecordError(err)` and `span.SetStatus(codes.Error, ...)`.

#### 2d. Evaluator Span in `internal/evaluator/evaluator.go`

| Span Name | Key Attributes |
|---|---|
| `evaluate rego` | `gavel.decision`, `gavel.finding_count`, `gavel.relevant_count` |

**Context propagation fix:** `evaluator.NewEvaluator()` currently creates `context.Background()` internally for `rego.PrepareForEval(ctx)`. Change the signature to accept an external `ctx` so Rego compilation appears in the trace:
```go
func NewEvaluator(ctx context.Context, policyDir string) (*Evaluator, error)
```

#### 2e. Store Spans in `internal/store/filestore.go`

| Span Name | Key Attributes |
|---|---|
| `write sarif` | `gavel.store.id`, `gavel.store.result_count` |
| `write verdict` | `gavel.store.id`, `gavel.decision` |

#### 2f. Cache Spans in `internal/cache/`

| Span Name | Key Attributes |
|---|---|
| `cache lookup` | `gavel.cache.hit` (bool), `gavel.cache.key` |
| `cache store` | `gavel.cache.key` |

#### 2g. Automatic HTTP Client Spans via `otelhttp`

The BAML client makes outgoing HTTP calls to LLM providers. If BAML's generated client exposes a configurable `http.Client` or `http.Transport`, wrap it with `otelhttp.NewTransport()` from `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`. This automatically produces HTTP client spans with standard `http.*` attributes (status code, URL, method) complementary to the GenAI-level spans — HTTP spans capture transport detail, GenAI spans capture semantic detail.

**Action:** Investigate BAML's HTTP client configurability during Phase 2 implementation. If customizable, add `otelhttp` as a dependency:
```
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
```

**Files to modify:**
- `cmd/gavel/analyze.go` — root span, signal handling
- `internal/analyzer/tiered.go` — tier and per-file spans
- `internal/analyzer/bamlclient.go` — LLM call spans, optionally `otelhttp` transport
- `internal/evaluator/evaluator.go` — evaluation span, accept `ctx` parameter
- `internal/store/filestore.go` — storage spans
- `internal/cache/cache.go` — cache spans

---

### Phase 3: OTel Metrics

Bridge the existing custom metrics system to the OTel Metrics API so that the same data is available in OTel-compatible backends.

#### 3a. OTel Instruments

Create OTel metric instruments in `internal/telemetry/metrics.go`.

**Naming rules for Mimir/Prometheus compatibility:**
- Do NOT include type suffixes in instrument names — Prometheus auto-appends `_total` for counters and `_bucket`/`_sum`/`_count` for histograms.
- Set the `unit` field on all instruments — Mimir auto-appends unit suffixes (e.g., `_seconds`) from OTel `unit` metadata.
- OTel convention uses seconds (not milliseconds) for duration metrics.

| OTel Instrument | Type | Unit | Prometheus Name | Description |
|---|---|---|---|---|
| `gavel.analysis.count` | Counter | `{analysis}` | `gavel_analysis_count_total` | Total analyses performed |
| `gavel.analysis.error.count` | Counter | `{error}` | `gavel_analysis_error_count_total` | Total analysis errors |
| `gavel.finding.count` | Counter | `{finding}` | `gavel_finding_count_total` | Total findings produced |
| `gavel.analysis.duration` | Histogram | `s` | `gavel_analysis_duration_seconds_*` | Analysis duration |
| `gavel.queue.duration` | Histogram | `s` | `gavel_queue_duration_seconds_*` | Queue wait time |
| `gavel.token.input` | Counter | `{token}` | `gavel_token_input_total` | Total input tokens consumed |
| `gavel.token.output` | Counter | `{token}` | `gavel_token_output_total` | Total output tokens produced |
| `gavel.cache.hit` | Counter | `{hit}` | `gavel_cache_hit_total` | Cache hits |
| `gavel.cache.miss` | Counter | `{miss}` | `gavel_cache_miss_total` | Cache misses |
| `gavel.cache.stale` | Counter | `{entry}` | `gavel_cache_stale_total` | Stale cache entries served |

**Histogram bucket boundaries** (in seconds): `{0.01, 0.05, 0.1, 0.5, 1, 5, 30}`

Example instrument creation:
```go
meter.Float64Histogram("gavel.analysis.duration",
    metric.WithUnit("s"),
    metric.WithDescription("End-to-end analysis duration"),
)
```

**Attribute cardinality guidance:**

Mimir's default cardinality limit is 150,000 active series per tenant. A histogram with 7 buckets and an unbounded label easily explodes series count. Only use bounded attributes on metrics:

| Attribute | Safe for Metrics? | Rationale |
|---|---|---|
| `gavel.tier` | Yes | 3 values (instant, fast, comprehensive) |
| `gavel.provider` | Yes | ~5 values (ollama, openrouter, anthropic, bedrock, openai) |
| `gavel.model` | Yes (with care) | Bounded by provider config, but monitor if users configure many models |
| `gavel.persona` | Yes | 3 values (code-reviewer, architect, security) |
| `gavel.cache.result` | Yes | 3 values (hit, miss, stale) |
| `gavel.decision` | Yes | 3 values (merge, reject, review) |
| `gavel.file_path` | **No** | Unbounded — use only on span attributes |
| `gavel.cache.key` | **No** | Unbounded — use only on span attributes |
| `gavel.error.message` | **No** | Unbounded — use only on span attributes |

#### 3b. Integration Approach

Option A (recommended): **Emit OTel metrics alongside existing metrics** at the call sites. The existing `metrics.Recorder` calls remain; add parallel `instrument.Add()` / `instrument.Record()` calls in the same locations.

Option B: Create an `OTelCollector` that implements a similar interface to `metrics.Collector` and register it as a listener. This adds abstraction complexity without much benefit since the call sites are already known.

With Option A, each place that calls `builder.Complete(...)` or `builder.CompleteWithError(...)` also records the corresponding OTel metric. This can be done inside the `Recorder`/`AnalysisBuilder` itself, keeping the call sites unchanged.

#### 3c. Implementation

Add OTel instruments as fields on `metrics.Recorder`. Initialize them from the global `MeterProvider` in `NewRecorder()`. When `Complete()` or `CompleteWithError()` is called, record to both the existing Collector and the OTel instruments.

**Files to create/modify:**
- Create `internal/telemetry/metrics.go` — OTel instrument definitions and initialization
- Modify `internal/metrics/recorder.go` — add OTel recording alongside existing Collector recording

---

### Phase 4: LSP Server Telemetry

The LSP server (`internal/lsp/`) is a long-running process that benefits from per-request tracing and operational metrics.

#### 4a. LSP Request Tracing

Add spans for each LSP request using verb-object naming:

| Span Name | Key Attributes |
|---|---|
| `handle lsp request` | `lsp.method` (e.g., `textDocument/didSave`), `lsp.uri` |
| `analyze lsp file` | `lsp.uri`, `gavel.finding_count`, `gavel.tier` |
| `publish diagnostics` | `lsp.uri`, `lsp.diagnostic_count` |

#### 4b. LSP Metrics

Counter/histogram names follow the same Mimir-compatible conventions as Phase 3 (no type suffixes, explicit units):

| OTel Instrument | Type | Unit | Prometheus Name | Description |
|---|---|---|---|---|
| `gavel.lsp.request.count` | Counter | `{request}` | `gavel_lsp_request_count_total` | Total LSP requests handled |
| `gavel.lsp.request.duration` | Histogram | `s` | `gavel_lsp_request_duration_seconds_*` | Per-request latency |
| `gavel.lsp.diagnostic.count` | Counter | `{diagnostic}` | `gavel_lsp_diagnostic_count_total` | Total diagnostics published |
| `gavel.lsp.file.watched` | UpDownCounter | `{file}` | `gavel_lsp_file_watched` | Current number of watched files |

**Files to modify:**
- `internal/lsp/server.go` — request-level spans
- `internal/lsp/analyzer.go` — analysis spans
- `internal/lsp/diagnostic.go` — diagnostic publishing spans

---

### Phase 5: Structured Logging (Optional Enhancement)

Replace scattered `log.Printf` / `slog.Warn` with `slog` backed by an OTel-aware handler so that logs are correlated with traces.

#### 5a. Log-Trace Correlation

Use `go.opentelemetry.io/contrib/bridges/otelslog` (or a custom `slog.Handler`) that injects `trace_id` and `span_id` into log records when a span is active in the context. This lets log backends (e.g., Grafana Loki) link logs to traces.

#### 5b. Standardized Logger

Create a project-wide `slog.Logger` in `internal/telemetry/logging.go` and use it consistently:

```go
// internal/telemetry/logging.go
func NewLogger(level slog.Level) *slog.Logger
```

**Files to create/modify:**
- Create `internal/telemetry/logging.go`
- Modify `internal/lsp/server.go` — replace `log.Printf` with `slog`
- Modify `internal/input/handler.go` — standardize `slog.Warn` usage

---

## Span Hierarchy (Example Trace)

```
analyze code [root]                                     2.3s
├── load config                                         12ms
├── read input                                          45ms
├── run instant tier                                    67ms
│   ├── analyze file {gavel.file_path=main.go}          8ms
│   ├── analyze file {gavel.file_path=handler.go}       12ms
│   └── analyze file {gavel.file_path=service.go}       9ms
├── run comprehensive tier                              2.1s
│   ├── analyze file {gavel.file_path=main.go}          680ms
│   │   └── chat claude-sonnet-4                        650ms
│   ├── analyze file {gavel.file_path=handler.go}       720ms
│   │   └── chat claude-sonnet-4                        690ms
│   └── analyze file {gavel.file_path=service.go}       700ms
│       └── chat claude-sonnet-4                        670ms
├── assemble sarif                                      3ms
├── write sarif                                         8ms
├── evaluate rego                                       15ms
└── write verdict                                       2ms
```

## Configuration Examples

### Local Development (Jaeger)

```yaml
telemetry:
  enabled: true
  endpoint: "localhost:4317"
  protocol: "grpc"
  insecure: true
  sample_rate: 1.0
```

```bash
# Start Jaeger all-in-one with OTLP support
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest
```

### CI Pipeline (via env vars)

```bash
export GAVEL_TELEMETRY_ENABLED=true
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otel-collector.internal:4317
export OTEL_SERVICE_NAME=gavel-ci
gavel analyze --dir .
```

### Production (Grafana Cloud)

```yaml
telemetry:
  enabled: true
  endpoint: "tempo-us-central1.grafana.net:443"
  protocol: "grpc"
  sample_rate: 0.1    # 10% sampling in production
  headers:
    Authorization: "Basic <base64-encoded-credentials>"
```

## Rollout & Testing Strategy

### Unit Tests
- `internal/telemetry/telemetry_test.go` — verify provider creation, shutdown, noop when disabled.
- Span assertion tests using `go.opentelemetry.io/otel/sdk/trace/tracetest.NewInMemoryExporter()` to capture spans in-memory and verify names, attributes, and parent-child relationships.
- Metric assertion tests using `go.opentelemetry.io/otel/sdk/metric/metricdata/metricdatatest` for counter/histogram verification.

### Integration Test
- Add a test that runs the full pipeline with an in-memory exporter and asserts the expected span tree structure.
- Verify that disabling telemetry produces zero exported spans.

### Performance Validation
- Benchmark with telemetry enabled vs. disabled to confirm overhead is negligible (<1% for tracing, <0.1% for noop).
- OTel's `BatchSpanProcessor` batches exports asynchronously, so hot-path latency impact should be minimal.

## File Change Summary

| File | Action | Phase |
|---|---|---|
| `internal/telemetry/telemetry.go` | Create | 1 |
| `internal/telemetry/telemetry_test.go` | Create | 1 |
| `internal/telemetry/metrics.go` | Create | 3 |
| `internal/telemetry/logging.go` | Create | 5 |
| `internal/config/config.go` | Modify — add `TelemetryConfig` | 1 |
| `cmd/gavel/analyze.go` | Modify — init telemetry, root span | 1, 2a |
| `internal/analyzer/tiered.go` | Modify — tier spans, per-file spans | 2b |
| `internal/analyzer/bamlclient.go` | Modify — LLM call spans | 2c |
| `internal/evaluator/evaluator.go` | Modify — evaluation span | 2d |
| `internal/store/filestore.go` | Modify — store spans | 2e |
| `internal/cache/cache.go` | Modify — cache spans | 2f |
| `internal/metrics/recorder.go` | Modify — bridge to OTel metrics | 3 |
| `internal/lsp/server.go` | Modify — request spans, replace log.Printf | 4, 5 |
| `internal/lsp/analyzer.go` | Modify — analysis spans | 4 |
| `internal/lsp/diagnostic.go` | Modify — diagnostic spans | 4 |
| `internal/input/handler.go` | Modify — standardize logging | 5 |
| `go.mod` | Modify — promote OTel deps to direct | 1 |

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| OTel SDK adds binary size | Already pulled in as indirect deps; promoting to direct adds ~2-3 MB. Acceptable for a CLI tool. |
| Performance overhead on hot path (instant tier) | OTel noop providers have near-zero cost. With tracing enabled, `BatchSpanProcessor` is async. Benchmark to confirm. |
| Breaking existing metrics consumers | Additive-only: existing JSON/CSV export unchanged. OTel metrics are a parallel output channel. |
| OTLP connection failures blocking analysis | Set short export timeouts (5s). OTel SDK drops spans/metrics silently on export failure — analysis is never blocked. |
| Context propagation gaps | Audit all `context.Background()` call sites in Phase 2 and replace with propagated ctx where applicable (e.g., `evaluator.NewEvaluator`). |

## Rejected Alternatives

### Grafana Beyla / eBPF Auto-Instrumentation

Beyla (now OpenTelemetry eBPF Instrumentation / OBI) was evaluated and is **not recommended** for Gavel:

- **CLI mode:** Too short-lived for eBPF probe attachment (2-30s runs). Beyla discovers services by listening port — CLI tools don't listen. The workload (file I/O, LLM API calls, Rego evaluation) is mostly local processing, outside Beyla's HTTP/gRPC request-response model.
- **LSP mode:** The LSP protocol uses stdio/pipes, not HTTP — Beyla cannot instrument editor-to-server communication. Beyla could capture outgoing HTTP calls to LLM APIs, but this duplicates what the manual GenAI spans already capture with richer semantic attributes. High-value LSP metrics (analysis latency, cache hit rates, diagnostic counts) are business logic that Beyla cannot observe.

Manual OTel SDK instrumentation provides superior coverage for all Gavel workloads.

## Open Questions

1. **Should the `review` and `create` subcommands also be traced?** The plan focuses on `analyze` as the primary pipeline. Other commands could be added later with minimal effort since the telemetry init will be shared.

2. **Baggage values**: The composite propagator now includes `propagation.Baggage{}` from Phase 1, so the infrastructure is ready. The open question is which baggage values to propagate (e.g., `gavel.run_id`, `gavel.ci_job_id`). Not needed for v1 — can be added without code changes once use cases emerge.

3. **Metrics export interval**: The default 60s `PeriodicReader` interval is fine for long-running LSP (or use 15s for near-real-time). For short CLI runs, `MeterProvider.Shutdown()` flushes remaining metrics. An alternative for CLI mode is `ManualReader`, which avoids the background goroutine entirely and gives explicit control over collection timing.

4. **BAML HTTP client configurability**: Phase 2g depends on whether BAML's generated client exposes a configurable `http.Client` or `http.Transport`. If not, `otelhttp` wrapping is not possible and the GenAI-level spans alone provide sufficient coverage.
