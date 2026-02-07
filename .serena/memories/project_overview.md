# Gavel Project Overview

## Purpose
Gavel is an AI-powered code analysis CLI that gates CI workflows by analyzing code against configurable policies via an LLM, producing SARIF (Static Analysis Results Interchange Format) output, and evaluating it with Rego policies to reach a verdict: **merge**, **reject**, or **review**.

## Tech Stack
- **Language**: Go 1.25.6
- **Task Runner**: Task (taskfile.dev)
- **LLM Integration**: BAML (BoundaryML) - templated LLM prompts with generated Go client
- **Policy Engine**: Open Policy Agent (OPA) with Rego v1
- **CLI Framework**: Cobra
- **LLM Providers**: OpenRouter (default, Claude Sonnet 4) or Ollama (local)
- **Output Format**: SARIF 2.1.0 with custom extensions

## Architecture Pipeline
```
Input Handler → BAML Analyzer → SARIF Assembler → Rego Evaluator → Verdict
                                       ↓                ↓
                                 FileStore ←─────────────┘
```

## Codebase Structure
```
cmd/gavel/           CLI entry point (Cobra commands)
internal/
  input/             Reads files, diffs, directories into artifacts
  config/            Tiered YAML policy configuration
  analyzer/          Orchestrates LLM analysis via BAML client
  sarif/             SARIF 2.1.0 assembly and deduplication
  evaluator/         Rego policy evaluation (OPA)
  store/             Filesystem persistence for results
baml_src/            BAML prompt templates (source of truth)
baml_client/         Generated Go client (DO NOT EDIT MANUALLY)
```

## Key Components
- **SARIF Extensions**: Custom properties under `gavel/` namespace (confidence, explanation, recommendation)
- **Tiered Config**: System defaults → machine config → project config
- **Storage**: Results stored in `.gavel/results/<timestamp-hex>/` with `sarif.json` and `verdict.json`
