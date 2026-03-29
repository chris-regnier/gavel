# Prose Personas Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `research-assistant` and `sharp-editor` personas to prove Gavel works beyond code analysis, with content-neutral BAML template language and persona-aware applicability filters.

**Architecture:** Two new persona prompt constants, a `ProseApplicabilityFilterPrompt` with prose-specific gates, and an `IsProsePersona()` helper that selects the right filter. The BAML template replaces code-specific wording with content-neutral equivalents. Both `cmd/gavel/analyze.go` and `internal/mcp/server.go` use the conditional filter logic.

**Tech Stack:** Go, BAML

**Spec:** `docs/superpowers/specs/2026-03-29-prose-personas-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/analyzer/personas.go` | Persona prompt constants, `GetPersonaPrompt()`, `IsProsePersona()`, `ProseApplicabilityFilterPrompt` |
| `internal/analyzer/personas_test.go` | Tests for new personas, prose filter, `IsProsePersona()` |
| `internal/config/config.go` | `validPersonas` map in `Validate()` |
| `baml_src/analyze.baml` | Content-neutral template wording |
| `cmd/gavel/analyze.go` | Conditional filter selection |
| `internal/mcp/server.go` | Conditional filter selection (same logic) |

---

### Task 1: Add `IsProsePersona()` helper and `ProseApplicabilityFilterPrompt`

**Files:**
- Modify: `internal/analyzer/personas.go`
- Test: `internal/analyzer/personas_test.go`

- [ ] **Step 1: Write failing tests for `IsProsePersona` and `ProseApplicabilityFilterPrompt`**

Add to `internal/analyzer/personas_test.go`:

```go
func TestIsProsePersona(t *testing.T) {
	tests := []struct {
		persona string
		want    bool
	}{
		{"research-assistant", true},
		{"sharp-editor", true},
		{"code-reviewer", false},
		{"code-reviewer-verbose", false},
		{"architect", false},
		{"security", false},
		{"unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.persona, func(t *testing.T) {
			if got := IsProsePersona(tt.persona); got != tt.want {
				t.Errorf("IsProsePersona(%q) = %v, want %v", tt.persona, got, tt.want)
			}
		})
	}
}

func TestProseApplicabilityFilterPrompt_NotEmpty(t *testing.T) {
	if ProseApplicabilityFilterPrompt == "" {
		t.Error("ProseApplicabilityFilterPrompt should not be empty")
	}
}

func TestProseApplicabilityFilterPrompt_ContainsKeyPhrases(t *testing.T) {
	phrases := []string{
		"ACTIONABLE",
		"EVIDENCED",
		"Do not report",
	}
	for _, phrase := range phrases {
		if !strings.Contains(ProseApplicabilityFilterPrompt, phrase) {
			t.Errorf("ProseApplicabilityFilterPrompt missing phrase: %q", phrase)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/analyzer/ -run "TestIsProsePersona|TestProseApplicabilityFilterPrompt" -v`
Expected: FAIL — `IsProsePersona` undefined, `ProseApplicabilityFilterPrompt` undefined

- [ ] **Step 3: Implement `IsProsePersona` and `ProseApplicabilityFilterPrompt`**

Add to `internal/analyzer/personas.go`, after the `ApplicabilityFilterPrompt` constant:

```go
// ProseApplicabilityFilterPrompt is the applicability filter for prose-focused
// personas (research-assistant, sharp-editor). It replaces the code-oriented
// filter with gates appropriate for writing analysis.
const ProseApplicabilityFilterPrompt = `

===== APPLICABILITY FILTER =====
Before reporting any finding, apply this applicability test:

1. ACTIONABLE: Is this feedback specific enough that the writer can act on
   it? If it is a vague impression ("this could be better", "consider
   revising"), do not report it.

2. EVIDENCED: Can you point to the specific sentence, paragraph, or passage
   that has the issue? If you cannot identify a concrete location, do not
   report it.

Do not report findings that fail either of these tests.
===== END FILTER =====`

// IsProsePersona returns true if the given persona is designed for prose/writing
// analysis rather than code analysis. This determines which applicability filter
// to use.
//
// Future direction: if more persona categories emerge, this should evolve into
// a persona category/type system with explicit "code" vs "prose" categories
// that select template phrasings and filter variants.
func IsProsePersona(persona string) bool {
	switch persona {
	case "research-assistant", "sharp-editor":
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/analyzer/ -run "TestIsProsePersona|TestProseApplicabilityFilterPrompt" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/analyzer/personas.go internal/analyzer/personas_test.go
git commit -m "feat: add IsProsePersona helper and ProseApplicabilityFilterPrompt"
```

---

### Task 2: Add `research-assistant` persona prompt

**Files:**
- Modify: `internal/analyzer/personas.go`
- Test: `internal/analyzer/personas_test.go`

- [ ] **Step 1: Write failing test for `research-assistant` persona**

Add a test case to the `tests` slice in `TestGetPersonaPrompt` in `internal/analyzer/personas_test.go`:

```go
{
	name:    "research-assistant persona",
	persona: "research-assistant",
	wantErr: false,
	wantContains: []string{
		"research",
		"evidence",
		"claims",
		"CONFIDENCE GUIDANCE",
	},
},
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/analyzer/ -run "TestGetPersonaPrompt/research-assistant" -v`
Expected: FAIL — `unknown persona: research-assistant`

- [ ] **Step 3: Add the `research-assistant` prompt constant and switch case**

Add the constant to `internal/analyzer/personas.go` after `securityPrompt`:

```go
	researchAssistantPrompt = `You are a research advisor reviewing technical and persuasive writing.
Your job is to find where arguments are thin, claims lack evidence, and ideas deserve deeper exploration.

FOCUS AREAS:
- Claims stated without evidence or citation
- Arguments that need stronger support or counterargument consideration
- Logical gaps or unsupported leaps in reasoning
- Opportunities for data, examples, or expert perspectives
- Areas where the reader would reasonably ask "says who?" or "how do you know?"

YOUR TONE:
Curious and constructive, like a peer reviewer pushing for rigor. You want the writing to be
more convincing, not less ambitious.

CONFIDENCE GUIDANCE:
- High (0.8-1.0): Clear logical gaps, factual claims with no evidence, contradictions
- Medium (0.5-0.8): Areas that would benefit from deeper treatment, weak arguments
- Low (0.0-0.5): Optional enrichment suggestions, additional angles to explore

Be precise about which passage needs attention. Only report genuine weaknesses.`
```

Add the switch case in `GetPersonaPrompt`:

```go
	case "research-assistant":
		return researchAssistantPrompt, nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/analyzer/ -run "TestGetPersonaPrompt/research-assistant" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/analyzer/personas.go internal/analyzer/personas_test.go
git commit -m "feat: add research-assistant persona prompt"
```

---

### Task 3: Add `sharp-editor` persona prompt

**Files:**
- Modify: `internal/analyzer/personas.go`
- Test: `internal/analyzer/personas_test.go`

- [ ] **Step 1: Write failing test for `sharp-editor` persona**

Add a test case to the `tests` slice in `TestGetPersonaPrompt` in `internal/analyzer/personas_test.go`:

```go
{
	name:    "sharp-editor persona",
	persona: "sharp-editor",
	wantErr: false,
	wantContains: []string{
		"editor",
		"clarity",
		"passive voice",
		"CONFIDENCE GUIDANCE",
	},
},
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/analyzer/ -run "TestGetPersonaPrompt/sharp-editor" -v`
Expected: FAIL — `unknown persona: sharp-editor`

- [ ] **Step 3: Add the `sharp-editor` prompt constant and switch case**

Add the constant to `internal/analyzer/personas.go` after `researchAssistantPrompt`:

```go
	sharpEditorPrompt = `You are a sharp prose editor focused on making writing clearer and more effective.
You cut waste, strengthen verbs, and fix structure so every sentence earns its place.

FOCUS AREAS:
- Unnecessary words, filler, and redundancy
- Passive voice where active would be stronger
- Weak verbs and vague language ("utilize", "leverage", "various", "aspects")
- Jargon that obscures rather than clarifies
- Sentence structure and paragraph flow problems
- Places where the reader might get lost or disengaged

YOUR TONE:
Direct and opinionated, like a newspaper editor with a red pen. You respect the writer's intent
but not their darlings.

CONFIDENCE GUIDANCE:
- High (0.8-1.0): Clear structural problems — incoherent flow, contradictions, passages that obscure meaning
- Medium (0.5-0.8): Style improvements — wordiness, passive voice, weak verbs
- Low (0.0-0.5): Subjective stylistic preferences, alternative phrasings

Be precise about which sentence or passage needs work. Only report genuine problems.`
```

Add the switch case in `GetPersonaPrompt`:

```go
	case "sharp-editor":
		return sharpEditorPrompt, nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/analyzer/ -run "TestGetPersonaPrompt/sharp-editor" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/analyzer/personas.go internal/analyzer/personas_test.go
git commit -m "feat: add sharp-editor persona prompt"
```

---

### Task 4: Add prose filter test for new personas

**Files:**
- Modify: `internal/analyzer/personas_test.go`

- [ ] **Step 1: Write test verifying prose personas get the prose filter**

Add to `internal/analyzer/personas_test.go`:

```go
func TestGetPersonaPrompt_WithProseFilter(t *testing.T) {
	prosePersonas := []string{"research-assistant", "sharp-editor"}
	for _, persona := range prosePersonas {
		t.Run(persona, func(t *testing.T) {
			prompt, err := GetPersonaPrompt(context.Background(), persona)
			if err != nil {
				t.Fatalf("GetPersonaPrompt(%s): %v", persona, err)
			}

			if !IsProsePersona(persona) {
				t.Errorf("IsProsePersona(%s) should be true", persona)
			}

			// Simulate what the caller does when StrictFilter is true for prose
			filtered := prompt + ProseApplicabilityFilterPrompt

			if !strings.Contains(filtered, "APPLICABILITY FILTER") {
				t.Errorf("filtered %s prompt missing filter block", persona)
			}
			if !strings.Contains(filtered, "ACTIONABLE") {
				t.Errorf("filtered %s prompt missing ACTIONABLE gate", persona)
			}
			if !strings.Contains(filtered, "EVIDENCED") {
				t.Errorf("filtered %s prompt missing EVIDENCED gate", persona)
			}
			// Prose filter should NOT contain code-specific gates
			if strings.Contains(filtered, "LANGUAGE SAFETY") {
				t.Errorf("filtered %s prompt should not contain LANGUAGE SAFETY gate", persona)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./internal/analyzer/ -run "TestGetPersonaPrompt_WithProseFilter" -v`
Expected: PASS (all implementations from prior tasks are in place)

- [ ] **Step 3: Commit**

```bash
git add internal/analyzer/personas_test.go
git commit -m "test: add prose filter verification for new personas"
```

---

### Task 5: Register new personas in config validation

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add new personas to `validPersonas` map**

In `internal/config/config.go`, in the `Validate()` method, change the `validPersonas` map (around line 215):

```go
	validPersonas := map[string]bool{
		"code-reviewer":         true,
		"code-reviewer-verbose": true,
		"architect":             true,
		"security":              true,
		"research-assistant":    true,
		"sharp-editor":          true,
	}
```

- [ ] **Step 2: Update the error message to include new personas**

In the same function, update the error format string (around line 222):

```go
	if c.Persona != "" && !validPersonas[c.Persona] {
		return fmt.Errorf("unknown persona: %s (valid: code-reviewer, code-reviewer-verbose, architect, security, research-assistant, sharp-editor)", c.Persona)
	}
```

- [ ] **Step 3: Run existing tests to verify nothing breaks**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: register research-assistant and sharp-editor in config validation"
```

---

### Task 6: Neutralize BAML template wording

**Files:**
- Modify: `baml_src/analyze.baml`

- [ ] **Step 1: Replace code-specific wording with content-neutral equivalents**

In `baml_src/analyze.baml`, make four word swaps:

1. Line 34: `Analyze the code against these specific policies. Only report genuine violations.` → `Analyze the content against these specific policies. Only report genuine violations.`
2. Line 35: `If a policy doesn't apply to this code, don't force a finding.` → `If a policy doesn't apply to this content, don't force a finding.`
3. Line 39: `===== CODE TO ANALYZE =====` → `===== CONTENT TO ANALYZE =====`
4. Line 44: `For each policy violation or issue you find:` → `For each issue you find:`

- [ ] **Step 2: Regenerate the BAML client**

Run: `task generate`
Expected: Clean generation, no errors

- [ ] **Step 3: Run all tests to verify nothing breaks**

Run: `task test`
Expected: PASS — template wording changes don't affect test behavior

- [ ] **Step 4: Commit**

```bash
git add baml_src/analyze.baml baml_client/
git commit -m "feat: neutralize BAML template wording for content-type agnosticism"
```

---

### Task 7: Conditional filter selection in `cmd/gavel/analyze.go`

**Files:**
- Modify: `cmd/gavel/analyze.go`

- [ ] **Step 1: Update filter append logic**

In `cmd/gavel/analyze.go`, replace the block at ~lines 117-120:

```go
	// Append applicability filter if enabled (default)
	if cfg.StrictFilter {
		personaPrompt += analyzer.ApplicabilityFilterPrompt
	}
```

with:

```go
	// Append applicability filter if enabled (default).
	// Prose personas get a writing-appropriate filter; code personas get the original.
	if cfg.StrictFilter {
		if analyzer.IsProsePersona(cfg.Persona) {
			personaPrompt += analyzer.ProseApplicabilityFilterPrompt
		} else {
			personaPrompt += analyzer.ApplicabilityFilterPrompt
		}
	}
```

- [ ] **Step 2: Run tests to verify nothing breaks**

Run: `task test`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/gavel/analyze.go
git commit -m "feat: select applicability filter based on persona type in analyze command"
```

---

### Task 8: Conditional filter selection in `internal/mcp/server.go`

**Files:**
- Modify: `internal/mcp/server.go`

- [ ] **Step 1: Update filter append logic**

In `internal/mcp/server.go`, replace the block at ~lines 692-695:

```go
	// Append applicability filter if enabled (default)
	if h.cfg.Config.StrictFilter {
		personaPrompt += analyzer.ApplicabilityFilterPrompt
	}
```

with:

```go
	// Append applicability filter if enabled (default).
	// Prose personas get a writing-appropriate filter; code personas get the original.
	if h.cfg.Config.StrictFilter {
		if analyzer.IsProsePersona(persona) {
			personaPrompt += analyzer.ProseApplicabilityFilterPrompt
		} else {
			personaPrompt += analyzer.ApplicabilityFilterPrompt
		}
	}
```

Note: the MCP handler uses the `persona` parameter (from `runAnalysis(ctx, artifacts, persona)`), not `cfg.Persona`.

- [ ] **Step 2: Run tests to verify nothing breaks**

Run: `task test`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/mcp/server.go
git commit -m "feat: select applicability filter based on persona type in MCP server"
```

---

### Task 9: Update `GetPersonaPrompt` error message

**Files:**
- Modify: `internal/analyzer/personas.go`

- [ ] **Step 1: Update the error message in the default case**

In `internal/analyzer/personas.go`, update the default case in `GetPersonaPrompt` (~line 135):

```go
	default:
		return "", fmt.Errorf("unknown persona: %s (valid options: code-reviewer, code-reviewer-verbose, architect, security, research-assistant, sharp-editor)", persona)
```

- [ ] **Step 2: Update the invalid persona test to check for new names in error**

In `internal/analyzer/personas_test.go`, the existing `"invalid persona"` test case checks `wantErr: true` — it will still pass. No change needed.

- [ ] **Step 3: Run all tests**

Run: `task test`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/analyzer/personas.go
git commit -m "chore: update persona error message to include new personas"
```
