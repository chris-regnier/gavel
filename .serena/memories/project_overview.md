# Gavel Project Overview

## Purpose
Gavel is an AI-powered code analysis CLI that gates CI workflows by analyzing code against configurable policies via an LLM, producing SARIF (Static Analysis Results Interchange Format) output, and evaluating it with Rego to reach a verdict: merge, reject, or review.

## Tech Stack
- **Language**: Go 1.25+
- **LLM Integration**: BAML (Boundary ML) with OpenRouter API
- **Policy Evaluation**: OPA (Open Policy Agent) with Rego
- **CLI Framework**: Cobra
- **Build Tool**: Task (taskfile.dev)
- **Default LLM Model**: anthropic/claude-sonnet-4 via OpenRouter

## Architecture
```
Input Handler → BAML Analyzer → SARIF Assembler → Rego Evaluator → Verdict
                                       ↓                ↓
                                 FileStore ←─────────────┘
```

Key components:
- `cmd/gavel/` - CLI entry point (Cobra)
- `internal/input/` - Reads files, diffs, directories
- `internal/config/` - Tiered YAML policy configuration
- `internal/analyzer/` - Orchestrates LLM analysis via BAML
- `internal/sarif/` - SARIF 2.1.0 assembly and deduplication
- `internal/evaluator/` - Rego policy evaluation
- `internal/store/` - Filesystem persistence
- `baml_src/` - BAML prompt templates (source of truth)
- `baml_client/` - Generated Go client (do not edit)

## Key Design Patterns
- **Interface-based LLM client**: `BAMLClient` interface allows for mocking in tests
- **Tiered configuration**: System defaults → machine config → project config
- **SARIF extensions**: Custom properties under `gavel/` namespace
- **Generated code**: BAML client is code-generated, never edit directly
