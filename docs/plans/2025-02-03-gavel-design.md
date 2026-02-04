# Gavel — Design Document

## Overview

Gavel is a stateless CLI tool that performs LLM-powered code analysis against declarative policies, produces extended SARIF output, and evaluates that output with Rego policies to produce actionable verdicts for gating CI workflows.

Humans and agents alike can use gavel's structured output to gate processes. For example, a CI pipeline might auto-merge high-quality CRs, kick back low-quality ones to the author, and route medium-quality reviews to a human.

## Pipeline

```
Code Artifacts → Input Handler → BAML Analyzer → SARIF Assembler → Rego Evaluator → Verdict
                                                       ↓                   ↓
                                                 Storage Backend ←─────────┘
```

## Components

### Input Handler

Accepts code artifacts in multiple forms:

- **Diffs** — patches from PRs or commits
- **Files** — specific files to analyze
- **Directories** — a subtree of the repo
- **Whole repos** — full codebase scan

The input handler prepares artifacts for analysis, including metadata like file paths, language, and diff context. If input exceeds the effective context size of the LLM (less policy content), the handler splits it into chunks internally using simple algorithms. This is an implementation detail — not a first-class architectural concern.

Semantic chunking (AST, tree-sitter) is explicitly deferred. It will be introduced only if testing proves simple splitting insufficient for preserving meaningful context.

### BAML Analyzer

The BAML analyzer templates declarative policies into LLM prompts and returns typed, structured findings.

**Responsibilities:**

- Accept prepared code artifacts from the input handler
- Accept the merged policy set from the configuration system
- Template policies into BAML function calls
- Parse LLM responses into typed findings using BAML's structured output guarantees

**Finding schema:**

| Field | Type | Description |
|-------|------|-------------|
| `ruleId` | string | Maps to the policy name (e.g., `error-handling`) |
| `level` | enum | `error`, `warning`, `note`, `none` |
| `message` | string | Concise description of the issue |
| `location` | Location | File path and line range |
| `recommendation` | string | Suggested fix or action (gavel SARIF extension) |
| `explanation` | string | Longer reasoning (gavel SARIF extension) |
| `confidence` | float | 0.0–1.0, LLM confidence (gavel SARIF extension) |

**What BAML provides:**

- Type-safe structured output with retry on schema violation
- Prompt templating with policy instructions
- Provider agnosticism — swap LLM providers without changing analysis logic

**What the analyzer does NOT do:**

- Merging across chunks (SARIF assembler's job)
- Decision-making (Rego's job)

### SARIF Assembler

Collects findings from one or more analyzer invocations and produces a valid SARIF 2.1.0 document with gavel-specific extensions.

**SARIF structure:**

```
sarifLog
└── runs[0]
    ├── tool
    │   ├── driver: "gavel"
    │   └── rules[]: one entry per enabled policy (ruleId, description, severity)
    ├── results[]: one entry per finding
    │   ├── ruleId, level, message, locations[]
    │   └── properties:
    │       ├── gavel/recommendation
    │       ├── gavel/explanation
    │       └── gavel/confidence
    └── properties:
        ├── gavel/configTiers: which config tiers were active
        └── gavel/inputScope: "diff" | "files" | "directory" | "repo"
```

**Deduplication:** When findings overlap across chunk boundaries, the assembler deduplicates using a simple heuristic — same `ruleId` + overlapping location = dedupe, keeping the finding with the highest confidence.

**Extension namespace:** All gavel-specific properties live under the `gavel/` prefix in SARIF `properties` bags, clearly separated from the base schema and other tooling.

### Rego Evaluator

Receives the assembled SARIF document and produces a structured verdict.

**How it works:**

- The full SARIF document is provided to OPA as the `input` document
- Rego policies are loaded with the same tiered configuration model as analysis policies
- Policies query SARIF findings — levels, confidence scores, gavel extensions — to produce a verdict

**Verdict schema:**

| Field | Type | Description |
|-------|------|-------------|
| `decision` | enum | `merge`, `reject`, `review` |
| `reason` | string | Human-readable summary |
| `relevant_findings` | array | Subset of findings that drove the decision |
| `metadata` | map | Arbitrary key-value pairs for downstream consumers |

**Example Rego policy:**

```rego
package gavel.gate

default decision = "review"

decision = "reject" {
    some result in input.runs[0].results
    result.level == "error"
    result.properties["gavel/confidence"] > 0.8
}

decision = "merge" {
    count([r | r := input.runs[0].results[_]; r.level != "none"]) == 0
}
```

**Key constraints:**

- Rego never sees source code — only SARIF
- All findings are present; Rego decides which are relevant for a given gate
- Users can write arbitrarily sophisticated decision logic

### Storage Backend

A pluggable interface for persisting SARIF documents and verdicts.

**Interface:**

```go
type Store interface {
    WriteSARIF(ctx context.Context, doc *sarif.Log) (string, error)
    WriteVerdict(ctx context.Context, sarifID string, verdict *Verdict) error
    ReadSARIF(ctx context.Context, id string) (*sarif.Log, error)
    ReadVerdict(ctx context.Context, sarifID string) (*Verdict, error)
    List(ctx context.Context, opts ListOpts) ([]string, error)
}
```

**Filesystem implementation (default):**

```
.gavel/results/
├── 2025-01-15T10-30-00Z-a1b2c3/
│   ├── sarif.json
│   └── verdict.json
├── 2025-01-15T11-00-00Z-d4e5f6/
│   ├── sarif.json
│   └── verdict.json
```

Directory names use `<timestamp>-<short-hash>` for natural ordering and uniqueness. Each result is self-contained and easy to commit as CI artifacts.

## Configuration System

Gavel uses tiered, declarative configuration with four levels. Higher tiers override lower ones.

### Precedence (highest to lowest)

1. **Human** — CLI flags and environment variables (`--policy severity.min=warning`, `GAVEL_POLICY_*`)
2. **Project** — checked into the repo (`.gavel/policies.yaml`)
3. **Machine** — per-machine config (`~/.config/gavel/policies.yaml`)
4. **System** — built-in defaults shipped with gavel

### Policy Format

```yaml
policies:
  error-handling:
    description: "Public functions must handle errors explicitly"
    severity: warning
    instruction: >
      Check that all public functions either return an error
      or handle errors from called functions. Flag functions
      that silently discard errors.
    enabled: true

  function-length:
    description: "Functions should not exceed a reasonable length"
    severity: note
    instruction: >
      Flag functions longer than 50 lines. Consider whether
      the function could be decomposed.
    enabled: true
```

### Merging Behavior

- Tiers merge top-down: system defaults load first, then machine, project, and human overrides apply in order
- A higher tier can override any field of a policy (e.g., change `severity`, set `enabled: false`)
- A higher tier can define entirely new policies
- Policies are identified by name: same name across tiers = override, new name = addition

## Technology

- **Go** — CLI and core pipeline
- **BAML** — LLM prompt templating and structured output
- **OPA/Rego** — verdict evaluation
- **SARIF 2.1.0** — interchange format with gavel extensions

## Explicitly Deferred

- Parameterized/variable policies
- Semantic chunking (AST/tree-sitter)
- SQLite or embedded database storage
- Vector storage for AST embeddings
- Remote storage backends (S3, GCS)
