# Gavel as a Service — Design Spec

**Date:** 2026-03-29
**Status:** Draft
**Approach:** Hand-written HTTP API with AsyncAPI documentation (Approach C)

## Overview

Expose Gavel as an HTTP service with SSE streaming, enabling platform teams and editor clients (Gavel Writer) to consume code and prose analysis as a backend. The server is a new `gavel serve` subcommand — single binary, modular interfaces for swapping storage/cache backends later.

## Goals

1. HTTP API for analysis and evaluation with SSE streaming of progressive tier results
2. Tenant-provided config (stateless server) with a path to server-side policy profiles
3. Support both code analysis and prose critique (Gavel Writer) through the same API
4. CLI as a client — `gavel analyze --server <url>` delegates to the service
5. AsyncAPI spec as authoritative documentation of the streaming protocol
6. Design for future async job model without implementing it now

## Non-Goals

- Editor/desktop UI (separate project, consumes this API)
- WebSocket transport (future, SSE is sufficient for v1)
- Async job polling/webhook endpoints (future, but data model supports it)
- Multi-region deployment or database-backed storage (future swap-in)
- Client SDK code generation from AsyncAPI (future, when tooling matures)

## Architecture

### Package Structure

```
cmd/gavel/serve.go              — Cobra command, wires server
internal/server/
  server.go                      — HTTP server lifecycle (start, graceful shutdown)
  routes.go                      — chi router setup, middleware stack
  handlers.go                    — request handlers (thin — delegate to service layer)
  sse.go                         — SSE stream writer utilities
  middleware/
    auth.go                      — API key extraction & tenant identification
    requestid.go                 — X-Request-ID propagation
internal/service/
  analyze.go                     — orchestrates analysis (wraps TieredAnalyzer)
  judge.go                       — orchestrates evaluation (wraps Evaluator)
  types.go                       — request/response types, SSE event schemas
api/
  asyncapi.yaml                  — AsyncAPI spec documenting SSE protocol
```

### Separation of Concerns

- **`internal/server/`** owns HTTP: routing, SSE framing, middleware, request/response serialization.
- **`internal/service/`** owns business logic: transport-agnostic, operates on Go types and channels. Same service layer backs both the HTTP server and the CLI commands.
- **`internal/analyzer/`**, **`internal/evaluator/`**, **`internal/store/`** are unchanged — the service layer composes them.

This means gRPC, WebSocket, or async job transports can be added by writing a new transport layer that calls the same service.

### LLM Client Per Request

The LLM client (`BAMLClient`) is constructed per-request from the tenant's `config.Provider` field. Different tenants can use different providers (Anthropic, OpenAI, Ollama, etc.) in the same server instance. The `TieredAnalyzer` is already stateless — it takes a `BAMLClient` at construction time.

## API Design

### Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/v1/analyze` | Submit analysis, return result synchronously |
| `POST` | `/v1/analyze/stream` | Submit analysis, stream tier results via SSE |
| `POST` | `/v1/judge` | Evaluate a stored result, return verdict |
| `GET` | `/v1/results` | List stored result IDs |
| `GET` | `/v1/results/{id}` | Get full SARIF for a result |
| `GET` | `/v1/results/{id}/verdict` | Get verdict for a result |
| `GET` | `/v1/health` | Liveness probe |
| `GET` | `/v1/ready` | Readiness probe (LLM provider reachable) |

### Authentication

`Authorization: Bearer <api-key>` header. Middleware extracts the key, validates against a keys file (`key:tenant-id` format), and attaches tenant ID to request context. Initially simple key validation; later maps to tenant profiles for server-side policy lookup.

### Analyze Request

```json
{
  "artifacts": [
    {"path": "main.go", "content": "package main...", "kind": "file"}
  ],
  "config": {
    "provider": {"name": "anthropic", "anthropic": {"model": "claude-haiku-4-5"}},
    "persona": "code-reviewer",
    "policies": {"bug-detection": {"enabled": true}},
    "rules": [{"id": "RE001", "pattern": "TODO", "severity": "warning"}]
  }
}
```

Artifact kinds: `"file"`, `"diff"`, `"prose"`. The `"prose"` kind signals the analyzer to skip code-specific instant-tier rules (AST checks, code regex patterns).

### SSE Event Stream

For `/v1/analyze/stream`, the response is `Content-Type: text/event-stream`:

```
event: tier
data: {"tier": "instant", "results": [...], "elapsed_ms": 45}

event: tier
data: {"tier": "fast", "results": [...], "elapsed_ms": 800}

event: tier
data: {"tier": "comprehensive", "results": [...], "elapsed_ms": 12400}

event: complete
data: {"result_id": "2026-03-29T14-00-00Z-a1b2c3", "total_findings": 7}
```

On error:

```
event: error
data: {"message": "LLM provider timeout", "tier": "comprehensive"}
```

Clients receive results from completed tiers even if a later tier fails. The `complete` event always fires (with partial results if a tier failed).

### Judge Request

```json
{
  "result_id": "2026-03-29T14-00-00Z-a1b2c3",
  "rego_policies": "optional inline rego string"
}
```

Returns:

```json
{
  "decision": "review",
  "reason": "3 medium-confidence findings require human review",
  "relevant_findings": [...]
}
```

## Service Layer

Transport-agnostic orchestration types:

```go
// internal/service/analyze.go

type AnalyzeRequest struct {
    Artifacts []input.Artifact
    Config    config.Config
    Rules     []rules.Rule
}

type TierResult struct {
    Tier      string          // "instant", "fast", "comprehensive"
    Results   []sarif.Result
    ElapsedMs int64
}

type AnalyzeResult struct {
    ResultID      string
    TotalFindings int
}

type AnalyzeService struct {
    store store.Store
}

// Synchronous — blocks until all tiers complete
func (s *AnalyzeService) Analyze(ctx context.Context, req AnalyzeRequest) (*AnalyzeResult, error)

// Progressive — emits per-tier results on channel.
// The error channel is for fatal errors only (e.g., invalid config, all providers unreachable).
// Tier-level failures (e.g., LLM timeout on comprehensive) are reported as TierResult with an Error field,
// and the stream continues. The AnalyzeResult channel receives exactly one value when the stream completes.
func (s *AnalyzeService) AnalyzeStream(ctx context.Context, req AnalyzeRequest) (<-chan TierResult, <-chan AnalyzeResult, <-chan error)
```

```go
// internal/service/judge.go

type JudgeRequest struct {
    ResultID string
    RegoDir  string
}

type JudgeService struct {
    store store.Store
}

func (s *JudgeService) Judge(ctx context.Context, req JudgeRequest) (*store.Verdict, error)
```

The `BAMLClient` is constructed per-request from `req.Config.Provider`, passed to a new `TieredAnalyzer` instance. The analyzer is stateless — no shared mutable state between requests.

## Gavel Writer & Prose Analysis

The service supports prose critique through the same API as code analysis. The persona determines the critique lens; the artifact kind determines which instant-tier rules apply.

### How It Works

- **Artifact kind `"prose"`** skips AST checks and code-specific regex rules in the instant tier. Prose-relevant pattern rules (passive voice, sentence length, repeated words) run instead.
- **Prose personas** follow the existing persona pattern — prompt constants in `personas.go` with a `GetPersonaPrompt()` switch case. Initial personas: `editor` (clarity, structure, tone), `copywriter` (persuasion, brevity), `technical-writer` (accuracy, audience-appropriate complexity).
- **Finding semantics** map naturally to SARIF — location (line/column range), rule ID, message (the critique), severity, confidence. Editor clients render these as inline annotations.
- **Progressive feedback** in the editor: instant tier shows pattern-based catches immediately, comprehensive tier adds structural LLM critique seconds later.

### What the Service Does NOT Own

The rich editor UI. Desktop and web editor clients are separate projects that consume the SSE stream and render findings as annotations.

## Storage, Caching & Future Async

### Storage

The existing `store.Store` interface is used as-is. FileStore for single-binary deployment. Tenant ID is embedded in SARIF metadata (`gavel/tenant` property) — no per-tenant storage backends needed initially. When tenant isolation matters (enterprise), swap to a partitioned store implementation.

### Caching

The `TieredAnalyzer`'s in-memory cache (`gavel/cache_key`) lives in-process, shared across requests. When horizontal scaling arrives, the cache interface gets a Redis implementation. No analyzer changes needed.

### Future Async Jobs

The design supports async jobs without implementing them now:
- `AnalyzeStream` returns a channel — an async job runner consumes the same channel
- Result IDs are generated at submission time, not completion
- Results are persisted tier-by-tier (not just at the end)
- Future `GET /v1/jobs/{id}` reads accumulated tier results from the store
- Future webhook callback sends final result to a registered URL

## Configuration & Deployment

### `gavel serve` Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--addr` | `:8080` | Listen address |
| `--auth-keys` | (none) | Path to API keys file (`key:tenant-id` format) |
| `--store-dir` | `.gavel/results` | Result storage directory |
| `--rego-dir` | (none) | Custom Rego policy directory |
| `--max-concurrent` | `10` | Max concurrent analysis jobs |
| `--read-timeout` | `30s` | HTTP read timeout |
| `--write-timeout` | `5m` | HTTP write timeout (long for SSE) |

### Backpressure

A semaphore of `--max-concurrent` slots. When full, new analysis requests receive `503 Service Unavailable` with `Retry-After` header.

### Graceful Shutdown

Same pattern as calibration server: `signal.NotifyContext` for SIGINT/SIGTERM, drain in-flight SSE streams (close channels triggering `complete`/`error` events), flush telemetry, exit.

### Observability

Built on existing OTEL integration:
- HTTP middleware creates root span per request (`http.method`, `http.route`, `http.status_code`)
- Tenant ID as span attribute (`gavel.tenant`)
- Request ID propagation (`X-Request-ID` to trace context)
- `/v1/health` and `/v1/ready` for orchestrators

### AsyncAPI Spec

Lives at `api/asyncapi.yaml`. Documents SSE event schemas (`tier`, `complete`, `error`), request/response types, and channel paths. Integration tests validate actual SSE output against spec schemas.

## CLI as Client

When `--server` is provided, the CLI commands delegate to the service:

- `gavel analyze --server https://gavel.internal --dir ./src` → `POST /v1/analyze` with local files as artifacts
- `gavel analyze --server https://gavel.internal --dir ./src --stream` → `POST /v1/analyze/stream`, prints tier results as they arrive
- `gavel judge --server https://gavel.internal --result <id>` → `POST /v1/judge`

The local execution path remains the default. The `--server` flag is opt-in.

## Testing Strategy

- **Unit tests:** Service layer tested with mock store and mock BAMLClient (existing pattern)
- **Integration tests:** Start server in-process, make real HTTP requests, validate SSE event sequences
- **Contract tests:** Parse `api/asyncapi.yaml`, validate that SSE output from integration tests matches declared schemas
- **Load tests:** Verify backpressure (semaphore) under concurrent requests
