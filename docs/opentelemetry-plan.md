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
5. **Semantic conventions**: Follow [OTel semantic conventions](https://opentelemetry.io/docs/specs/semconv/) where applicable (e.g., `rpc.*` for LLM calls, `code.*` for file analysis).

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│  cmd/gavel/analyze.go                                   │
│  ┌─────────────────────────────────────────────────────┐│
│  │ initTelemetry(cfg) → TracerProvider, MeterProvider  ││
│  │ defer shutdown()                                    ││
│  └─────────────────────────────────────────────────────┘│
│                         │                               │
│              Root span: "gavel.analyze"                  │
│                         │                               │
│  ┌──────────┬───────────┼───────────┬──────────────┐    │
│  ▼          ▼           ▼           ▼              ▼    │
│ config    input    tiered-analyze  evaluate      store   │
│  load     read     ┌────┴────┐     (rego)      (write)  │
│                    ▼         ▼                           │
│              tier.instant  tier.comprehensive            │
│              (regex+AST)   ┌─────┴─────┐                │
│                           ▼            ▼                │
│                     baml.analyze   baml.analyze          │
│                     (per file)     (per file)            │
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
  sample_rate: 1.0                        # 0.0-1.0, 1.0 = always sample
  headers: {}                             # custom headers (auth tokens, etc.)
```

Also support environment variable overrides per the OTel spec:
- `OTEL_EXPORTER_OTLP_ENDPOINT` → overrides `endpoint`
- `OTEL_EXPORTER_OTLP_PROTOCOL` → overrides `protocol`
- `OTEL_SERVICE_NAME` → overrides `service_name`
- `OTEL_TRACES_SAMPLER` → overrides `sample_rate`
- `GAVEL_TELEMETRY_ENABLED=true` → overrides `enabled`

Environment variables take precedence over config file values. The standard `OTEL_*` variables are the canonical way to configure OTel in production environments; the config file is a convenience for project-level defaults.

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
2. Create OTLP exporter (gRPC or HTTP based on `protocol`).
3. Create `TracerProvider` with `BatchSpanProcessor` and configured sampler.
4. Create `MeterProvider` with `PeriodicReader` (60s default interval).
5. Set as global providers via `otel.SetTracerProvider()` / `otel.SetMeterProvider()`.
6. Set `propagation.TraceContext{}` as global propagator.
7. Return a `shutdown` function that flushes and shuts down both providers.

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

```go
tracer := otel.Tracer("gavel")
ctx, span := tracer.Start(ctx, "gavel.analyze",
    trace.WithAttributes(
        attribute.String("gavel.input_scope", inputScope),
        attribute.Int("gavel.artifact_count", len(artifacts)),
        attribute.String("gavel.persona", cfg.Persona),
        attribute.String("gavel.provider", cfg.Provider.Name),
    ),
)
defer span.End()
```

On error, call `span.RecordError(err)` and `span.SetStatus(codes.Error, err.Error())`.

#### 2b. Tiered Analyzer Spans in `internal/analyzer/tiered.go`

Each tier gets a child span:

| Span Name | Key Attributes |
|---|---|
| `gavel.tier.instant` | `gavel.tier=instant`, `gavel.rule_count`, `gavel.finding_count` |
| `gavel.tier.fast` | `gavel.tier=fast`, `gavel.model`, `gavel.provider` |
| `gavel.tier.comprehensive` | `gavel.tier=comprehensive`, `gavel.model`, `gavel.provider` |

Per-file analysis within each tier creates a child span:

| Span Name | Key Attributes |
|---|---|
| `gavel.analyze_file` | `gavel.file_path`, `gavel.file_size`, `gavel.line_count`, `gavel.tier` |

#### 2c. LLM Call Spans in `internal/analyzer/bamlclient.go`

Each provider call gets a span following [OTel GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/):

| Span Name | Key Attributes |
|---|---|
| `gavel.llm.call` | `gen_ai.system` (provider name), `gen_ai.request.model`, `gen_ai.response.finish_reason`, `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens` |

Errors from provider calls are recorded on the span.

#### 2d. Evaluator Span in `internal/evaluator/evaluator.go`

| Span Name | Key Attributes |
|---|---|
| `gavel.evaluate` | `gavel.decision`, `gavel.finding_count`, `gavel.relevant_count` |

#### 2e. Store Spans in `internal/store/filestore.go`

| Span Name | Key Attributes |
|---|---|
| `gavel.store.write_sarif` | `gavel.store.id`, `gavel.store.result_count` |
| `gavel.store.write_verdict` | `gavel.store.id`, `gavel.decision` |

#### 2f. Cache Spans in `internal/cache/`

| Span Name | Key Attributes |
|---|---|
| `gavel.cache.lookup` | `gavel.cache.hit` (bool), `gavel.cache.key` |
| `gavel.cache.store` | `gavel.cache.key` |

**Files to modify:**
- `cmd/gavel/analyze.go` — root span
- `internal/analyzer/tiered.go` — tier and per-file spans
- `internal/analyzer/bamlclient.go` — LLM call spans
- `internal/evaluator/evaluator.go` — evaluation span
- `internal/store/filestore.go` — storage spans
- `internal/cache/cache.go` — cache spans

---

### Phase 3: OTel Metrics

Bridge the existing custom metrics system to the OTel Metrics API so that the same data is available in OTel-compatible backends.

#### 3a. OTel Instruments

Create OTel metric instruments in `internal/telemetry/metrics.go`:

| OTel Instrument | Type | Description |
|---|---|---|
| `gavel.analyses.total` | Counter | Total analyses performed |
| `gavel.analyses.errors` | Counter | Total analysis errors |
| `gavel.findings.total` | Counter | Total findings produced |
| `gavel.analysis.duration` | Histogram | Analysis duration (ms), buckets: 10, 50, 100, 500, 1000, 5000, 30000 |
| `gavel.queue.duration` | Histogram | Queue wait time (ms) |
| `gavel.tokens.input` | Counter | Total input tokens consumed |
| `gavel.tokens.output` | Counter | Total output tokens produced |
| `gavel.cache.hits` | Counter | Cache hits |
| `gavel.cache.misses` | Counter | Cache misses |
| `gavel.cache.stale` | Counter | Stale cache entries served |

Common attributes on all metrics: `gavel.tier`, `gavel.provider`, `gavel.model`, `gavel.persona`.

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

Add spans for each LSP request:

| Span Name | Key Attributes |
|---|---|
| `gavel.lsp.request` | `lsp.method` (e.g., `textDocument/didSave`), `lsp.uri` |
| `gavel.lsp.analyze` | `lsp.uri`, `gavel.finding_count`, `gavel.tier` |
| `gavel.lsp.diagnostic.publish` | `lsp.uri`, `lsp.diagnostic_count` |

#### 4b. LSP Metrics

| OTel Instrument | Type | Description |
|---|---|---|
| `gavel.lsp.requests.total` | Counter | Total LSP requests handled |
| `gavel.lsp.request.duration` | Histogram | Per-request latency |
| `gavel.lsp.diagnostics.published` | Counter | Total diagnostics published |
| `gavel.lsp.files.watched` | UpDownCounter | Current number of watched files |

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
gavel.analyze [root]                                    2.3s
├── gavel.config.load                                   12ms
├── gavel.input.read                                    45ms
├── gavel.tier.instant                                  67ms
│   ├── gavel.analyze_file [main.go]                    8ms
│   ├── gavel.analyze_file [handler.go]                 12ms
│   └── gavel.analyze_file [service.go]                 9ms
├── gavel.tier.comprehensive                            2.1s
│   ├── gavel.analyze_file [main.go]                    680ms
│   │   └── gavel.llm.call [anthropic/claude-sonnet-4]  650ms
│   ├── gavel.analyze_file [handler.go]                 720ms
│   │   └── gavel.llm.call [anthropic/claude-sonnet-4]  690ms
│   └── gavel.analyze_file [service.go]                 700ms
│       └── gavel.llm.call [anthropic/claude-sonnet-4]  670ms
├── gavel.sarif.assemble                                3ms
├── gavel.store.write_sarif                             8ms
├── gavel.evaluate                                      15ms
└── gavel.store.write_verdict                           2ms
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

## Open Questions

1. **Should the `review` and `create` subcommands also be traced?** The plan focuses on `analyze` as the primary pipeline. Other commands could be added later with minimal effort since the telemetry init will be shared.

2. **Baggage propagation**: Should Gavel propagate OTel baggage (e.g., `gavel.run_id`) so downstream services (if any) can correlate? Not needed for v1, but the infrastructure supports it if `propagation.Baggage{}` is added to the composite propagator.

3. **Metrics export interval**: The default 60s periodic reader interval is fine for long-running LSP, but for short CLI runs the `shutdown()` function must flush remaining metrics. OTel SDK handles this via `MeterProvider.Shutdown()`.
