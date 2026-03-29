# Prose Personas Design

**Date:** 2026-03-29
**Status:** Approved

## Goal

Add two non-code personas to Gavel — `research-assistant` and `sharp-editor` — to prove out generalizability beyond code analysis. These personas analyze prose (technical writing, persuasive writing, documentation) using the same pipeline, output format, and policy system as code personas.

## Design Decisions

- **Approach 2 (persona-aware template language + conditional filter)** was chosen over persona-only changes (Approach 1) and category-based template variants (Approach 3). This neutralizes code-specific language in the BAML template and makes the applicability filter persona-aware, without introducing new abstractions.
- **Future direction:** If more persona categories emerge, `IsProsePersona()` should evolve into a persona category/type system (Approach 3) with explicit `code` vs `prose` categories that select template phrasings and filter variants.
- **No new default policies.** Prose personas work with user-configured policies. Canned configs per use-case may be added in the future.
- **No changes to input handling or output format.** The Finding structure (line numbers, confidence, filePath, etc.) works for prose files as-is.

## Changes

### 1. New Persona Prompts (`internal/analyzer/personas.go`)

Two new prompt constants (~100 words each):

**`research-assistant`** — Research depth advisor for technical and persuasive writing.
- Focus: claims lacking evidence or citation, thin arguments needing depth, specific research directions (data, counterarguments, expert perspectives), logical gaps or unsupported leaps.
- Tone: curious and constructive, like a peer reviewer pushing for rigor.
- Confidence: high (0.8+) for clear logical gaps or unsupported claims; medium (0.5-0.8) for areas that would benefit from deeper treatment; low (<0.5) for optional enrichment suggestions.

**`sharp-editor`** — Prose clarity and effectiveness editor.
- Focus: unnecessary words/jargon/passive voice, weak verbs and vague language, sentence structure and paragraph flow, places where the reader might get lost or disengaged.
- Tone: direct and opinionated, like a newspaper editor with a red pen.
- Confidence: high (0.8+) for clear structural problems (incoherent flow, contradictions); medium (0.5-0.8) for style improvements (wordiness, passive voice); low (<0.5) for subjective stylistic preferences.

### 2. BAML Template Neutralization (`baml_src/analyze.baml`)

Word swaps only — no structural changes:

| Before | After |
|--------|-------|
| `CODE TO ANALYZE` | `CONTENT TO ANALYZE` |
| `Analyze the code against these specific policies` | `Analyze the content against these specific policies` |
| `If a policy doesn't apply to this code` | `If a policy doesn't apply to this content` |
| `For each policy violation or issue you find` | `For each issue you find` |

All existing code personas work identically after these changes.

### 3. Conditional Applicability Filter (`internal/analyzer/personas.go`)

**New helper:** `IsProsePersona(persona string) bool` — returns true for `research-assistant` and `sharp-editor`. Include a comment noting this should evolve into a category/type system if more persona categories emerge.

**New constant:** `ProseApplicabilityFilterPrompt` with two gates:
- **ACTIONABLE:** Is this feedback specific enough that the writer can act on it? Skip vague impressions.
- **EVIDENCED:** Can you point to the specific sentence, paragraph, or passage? Don't report general feelings.

**Prompt assembly:** In `cmd/gavel/analyze.go` (~line 118), where `ApplicabilityFilterPrompt` is appended when `StrictFilter` is true, check `IsProsePersona(cfg.Persona)` and append `ProseApplicabilityFilterPrompt` instead for prose personas.

### 4. Registration & Validation

- `personas.go` `GetPersonaPrompt()`: Add switch cases for `"research-assistant"` and `"sharp-editor"`.
- `config.go` `Validate()`: Add both to the `validPersonas` map.
- Update error message strings listing valid personas.
- No change to `SystemDefaults()` — default remains `"code-reviewer"`.

## Files Modified

| File | Change |
|------|--------|
| `internal/analyzer/personas.go` | New prompts, `IsProsePersona()`, `ProseApplicabilityFilterPrompt`, switch cases |
| `internal/config/config.go` | Add to `validPersonas` map |
| `baml_src/analyze.baml` | Neutralize code-specific wording |
| `cmd/gavel/analyze.go` | Conditional filter selection based on persona type |
| `internal/mcp/server.go` | Conditional filter selection (same logic as analyze.go) |

## Out of Scope

- New default policies for prose personas (future: canned configs)
- Separate BAML functions or input handling changes
- Changes to the Finding struct or SARIF output format
- Rego evaluator changes
