# Online Calibration RAG Service Design

**Date:** 2026-03-03
**Status:** Approved
**Branch:** benchmarks

## Overview

A remote server component that uses accumulated user feedback and analysis history to improve Gavel's code review quality over time. The system operates as a RAG (Retrieval-Augmented Generation) backend with three layers: per-team calibration profiles, cross-org aggregated learning, and semantic few-shot example retrieval.

## Goals

- Reduce noise rate by >20% through feedback-driven calibration
- Enable cross-organization learning from anonymized rule effectiveness data
- Provide few-shot examples from similar past reviews to ground LLM analysis
- Maintain graceful degradation — analysis works without the server

## Architecture: Event-Sourced Calibration Service

The system treats every finding and feedback signal as an immutable event. Three materialized views — team profiles, cross-org aggregates, and a vector index — are derived from the event stream and served to the CLI at analysis time.

```
CLI ──upload──▶ Calibration API ──▶ Event Store (append-only)
                    │                       │
CLI ◀──retrieve──   │              ┌────────┼────────┐
                    │              ▼        ▼        ▼
                    │         Team       Cross-org   Vector
                    │        Profiles   Aggregates   Index
                    │              │        │        │
                    ◀──────────────┴────────┴────────┘
                         (materialized views)
```

## Section 1: Event Model & Data Flow

### Events (append-only)

| Event | Trigger | Payload |
|-------|---------|---------|
| `analysis_completed` | CLI finishes `gavel analyze` | SARIF result ID, team ID, rule IDs triggered, file types, finding count, model/provider used |
| `finding_created` | Per finding within an analysis | Rule ID, severity, confidence, code snippet (opt-in), finding message, explanation, file type, line range |
| `feedback_received` | User marks a finding | Finding ref, verdict (`useful`/`noise`/`wrong`), optional reason text |
| `outcome_observed` | Implicit signal | Finding ref, outcome type (`dismissed`, `merged_unchanged`, `merged_after_fix`, `time_to_resolve_ms`) |

### Ingest Flow

```
gavel analyze
    │
    ├── (existing) store SARIF locally
    │
    └── (new) POST /v1/events/batch
              body: { team_id, events: [...] }
              │
              ▼
        Calibration API
              │
              ├── validate & normalize
              ├── append to event store
              └── async: update materialized views
```

The CLI uploads events **asynchronously after analysis completes** — it doesn't block the user's workflow. If the server is unreachable, events are queued locally in `.gavel/pending_events/` and retried on next run.

### Retrieval Flow

```
gavel analyze (start)
    │
    ├── GET /v1/calibration/{team_id}?rules=SEC001,SEC002&file_type=go
    │       → { thresholds: {...}, few_shot_examples: [...] }
    │
    ├── inject few-shot examples into BAML prompt
    ├── run LLM analysis
    ├── apply threshold adjustments to output
    └── proceed with SARIF assembly
```

Retrieval is **synchronous at analysis start**. Latency target: <200ms. If unreachable, analysis proceeds with defaults (graceful degradation).

### Key Decisions

- **Opt-in code context:** `finding_created` only includes code snippets if `calibration.share_code: true`.
- **Local queue for resilience:** Failed uploads stored in `.gavel/pending_events/` as JSON, retried next run.
- **Team isolation by default:** Cross-org aggregation uses anonymized rule-level statistics only.

## Section 2: Materialized Views & Retrieval

### View 1: Team Calibration Profiles

Per-team, per-rule statistical summary updated incrementally as feedback arrives.

```
team_calibration_profile {
  team_id:            string
  rule_id:            string

  total_findings:     int
  useful_count:       int
  noise_count:        int
  wrong_count:        int
  noise_rate:         float   // noise_count / total_findings

  mean_useful_conf:   float
  mean_noise_conf:    float
  conf_calibration:   float   // mean_useful_conf - mean_noise_conf

  suppress_below:     float   // derived: suppress findings below this confidence
  boost_above:        float   // derived: prioritize findings above this confidence

  dismiss_rate:       float
  avg_time_to_resolve_ms: int

  updated_at:         timestamp
}
```

**Threshold derivation:** When `noise_rate > 0.7`, set `suppress_below` to the 75th percentile of noise finding confidences. When `conf_calibration < 0.1`, flag rule as poorly calibrated and increase suppression threshold.

### View 2: Cross-Org Aggregates

Anonymized, rule-level statistics across all teams.

```
cross_org_rule_stats {
  rule_id:            string
  total_teams_using:  int
  global_noise_rate:  float
  global_conf_calibration: float

  by_file_type: map[string]{
    noise_rate:       float
    finding_count:    int
  }

  noise_rate_trend:   []float   // rolling 30-day windows, last 4
  updated_at:         timestamp
}
```

Rules with `global_noise_rate > 0.8` across 50+ teams signal the rule itself needs tuning.

### View 3: Vector Index (RAG)

Embeddings for semantic similarity retrieval. Only populated for `share_code: true` teams.

```
finding_embedding {
  id:               string
  team_id:          string
  rule_id:          string
  file_type:        string

  code_snippet:     string
  finding_message:  string
  explanation:      string
  verdict:          string   // useful/noise/wrong/pending

  embedding:        float[768]
  created_at:       timestamp
}
```

### Composite Retrieval Response

```json
{
  "team_thresholds": {
    "SEC001": { "suppress_below": 0.45 },
    "QUAL003": { "suppress_below": 0.6 }
  },
  "cross_org_signals": {
    "SEC001": { "global_noise_rate": 0.23 },
    "QUAL003": { "global_noise_rate": 0.81, "warning": "high_noise_globally" }
  },
  "few_shot_examples": [
    {
      "rule_id": "SEC001",
      "code_snippet": "func authenticate(...",
      "finding_message": "SQL injection risk via string concatenation",
      "verdict": "useful",
      "similarity": 0.92
    }
  ]
}
```

## Section 3: Server Architecture & API

### Tech Stack

| Component | Initial | Future Option | Interface |
|-----------|---------|---------------|-----------|
| Event store & profiles | SQLite (WAL mode) | PostgreSQL | `EventStore` interface |
| Vector index | Qdrant | pgvector | `VectorStore` interface |
| API server | Go (net/http + chi) | — | — |
| Embedding model | OpenAI text-embedding-3-small (768d) | — | `Embedder` interface |
| Auth | API keys per team | — | — |

### Storage Interfaces

```go
type EventStore interface {
    AppendEvents(ctx context.Context, teamID string, events []Event) error
    GetTeamProfile(ctx context.Context, teamID string, ruleIDs []string) (*TeamProfile, error)
    GetGlobalStats(ctx context.Context, ruleIDs []string) (*GlobalStats, error)
    UpdateProfileFromFeedback(ctx context.Context, teamID string, feedback FeedbackEvent) error
    DeleteTeamData(ctx context.Context, teamID string) error
}

type VectorStore interface {
    Store(ctx context.Context, findings []FindingWithEmbedding) error
    Search(ctx context.Context, query SearchQuery) ([]SimilarFinding, error)
    UpdateVerdict(ctx context.Context, findingID string, verdict string) error
    DeleteByTeam(ctx context.Context, teamID string) error
}
```

### API Surface

```
Authorization: Bearer <team-api-key>

POST   /v1/events/batch                  # Upload events (202 Accepted)
GET    /v1/calibration/{team_id}         # Composite calibration data
GET    /v1/teams/{team_id}/profile       # Team calibration profile
GET    /v1/teams/{team_id}/rules/{id}    # Rule-specific feedback stats
DELETE /v1/teams/{team_id}/data          # GDPR: purge all team data
GET    /v1/health                        # Healthcheck
```

### Database Schema

```sql
CREATE TABLE events (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  team_id     TEXT NOT NULL,
  event_type  TEXT NOT NULL,
  payload     TEXT NOT NULL,  -- JSON
  created_at  TEXT DEFAULT (datetime('now'))
);
CREATE INDEX idx_events_team ON events(team_id, created_at);

CREATE TABLE team_rule_profiles (
  team_id         TEXT NOT NULL,
  rule_id         TEXT NOT NULL,
  total_findings  INTEGER DEFAULT 0,
  useful_count    INTEGER DEFAULT 0,
  noise_count     INTEGER DEFAULT 0,
  wrong_count     INTEGER DEFAULT 0,
  mean_useful_conf REAL,
  mean_noise_conf  REAL,
  dismiss_rate    REAL,
  suppress_below  REAL,
  updated_at      TEXT,
  PRIMARY KEY (team_id, rule_id)
);

CREATE TABLE global_rule_stats (
  rule_id             TEXT PRIMARY KEY,
  total_teams         INTEGER DEFAULT 0,
  global_noise_rate   REAL,
  global_conf_cal     REAL,
  by_file_type        TEXT,  -- JSON
  updated_at          TEXT
);
```

Qdrant collection schema:
- Collection: `finding_embeddings`
- Vector size: 768, distance: Cosine
- Payload fields: `team_id`, `rule_id`, `file_type`, `code_snippet`, `finding_message`, `explanation`, `verdict`, `created_at`
- Filter: always scoped by `team_id`

### View Materialization

Events trigger incremental updates via background workers using SQLite triggers or polling. Updates to team profiles, global stats, and embedding verdicts run asynchronously — not in the request path.

### Deployment

```
┌──────────────┐     ┌──────────────┐
│   API Pod    │     │   Worker     │
│   (Go)       │     │   Pod (Go)   │
│              │     │              │
│ SQLite (WAL) │     │ Embedding    │
│   (mounted   │     │   gen +      │
│    volume)   │     │   Qdrant     │
└──────┬───────┘     │   writes     │
       │             └──────┬───────┘
       │                    │
       │              ┌─────▼─────┐
       └──────────────▶  Qdrant   │
                      └───────────┘
```

## Section 4: CLI Integration

### Configuration

New section in `.gavel/policies.yaml`:

```yaml
calibration:
  enabled: true
  server_url: "https://calibration.gavel.dev"
  api_key_env: "GAVEL_CALIBRATION_KEY"
  share_code: false

  retrieve:
    enabled: true
    include_examples: true
    top_k: 3
    timeout_ms: 500

  upload:
    enabled: true
    include_implicit: true
    batch_size: 100
```

### Changes to `gavel analyze`

**Before LLM call:** Retrieve calibration data, inject few-shot examples into persona prompt, store thresholds for post-processing.

**After LLM call:** Apply threshold overrides to suppress low-confidence findings based on team feedback history. Annotate suppressed findings with `"gavel/suppressed_by": "calibration"`.

**After analysis:** Non-blocking upload of events. Failed uploads queued locally for retry.

### New CLI Commands

```bash
gavel feedback --result <id> --finding 3 --verdict useful
gavel feedback --result <id> --finding 7 --verdict noise --reason "false positive"

gavel calibration sync          # Retry failed uploads
gavel calibration profile       # View team calibration profile
gavel calibration profile --rule SEC001
gavel calibration share-code --enable
gavel calibration share-code --disable
```

### Prompt Augmentation Format

```
--- Calibration Context ---
Based on historical review feedback on similar code:

[USEFUL PATTERN] Rule SEC001 on Go auth code:
Finding: "SQL injection via string concatenation in query builder"
Context: Team confirmed this was a real vulnerability that was fixed.

[NOISE PATTERN] Rule SEC001 on Go database code:
Finding: "Potential SQL injection in user lookup"
Context: Marked as noise — code uses parameterized queries throughout.

Use these patterns to calibrate your confidence. Avoid raising findings
similar to NOISE patterns unless you have strong evidence.
---
```

### Graceful Degradation

| Failure | Behavior |
|---------|----------|
| Server unreachable on retrieval | Warn, proceed without calibration |
| Server unreachable on upload | Queue locally, retry next run |
| Server returns partial data | Use available data, defaults for rest |
| API key missing/invalid | Disable calibration, warn once |
| Timeout exceeded | Fail open, log latency |

## Section 5: Testing, Security & Rollout

### Security Model

| Data | Leaves client? | Condition |
|------|----------------|-----------|
| Rule IDs, severities, confidence | Always (when enabled) | `calibration.enabled: true` |
| File types, line counts | Always (when enabled) | `calibration.enabled: true` |
| Finding messages & explanations | Always (when enabled) | Part of metadata |
| Code snippets | Only if opted in | `calibration.share_code: true` |
| Feedback verdicts & reasons | Always (when enabled) | `calibration.upload.enabled: true` |

- API keys bound to `team_id`, can only access own data
- Vector search always scoped by `team_id` (no cross-team code leakage)
- Cross-org learning uses rule-level statistics only, never code or embeddings
- GDPR deletion via `DELETE /v1/teams/{team_id}/data`
- Data retention: 1 year default, configurable per team

### Testing Strategy

- **Server:** Unit tests for materialization, integration tests with SQLite + Qdrant, API contract tests, load tests (<200ms p95)
- **CLI:** Mock calibration server, graceful degradation tests, prompt augmentation formatting, threshold override application
- **E2E:** Corpus benchmark with/without calibration, target >10% noise reduction without recall loss

### Rollout Plan

| Phase | Scope | Measures |
|-------|-------|----------|
| 1: Event collection | Upload only, accumulate data | Event ingest reliability, data volume |
| 2: Threshold calibration | Retrieval with thresholds, no RAG | Noise rate reduction, false negative rate |
| 3: RAG retrieval | Few-shot examples for share_code teams | Precision, recall, conf calibration delta |
| 4: Cross-org learning | Anonymized aggregates, noisy rule warnings | Global noise rate trends, rule deprecation signals |

### Success Metrics

| Metric | Target |
|--------|--------|
| Noise rate reduction | >20% after 30 days |
| Recall preservation | <5% recall drop |
| Retrieval latency | <200ms p95 |
| Upload reliability | >99% delivery within 24h |
| Adoption | >50% active teams enable calibration |
