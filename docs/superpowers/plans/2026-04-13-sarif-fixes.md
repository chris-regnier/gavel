# SARIF `fixes` for Auto-Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add structured SARIF `fixes` to Gavel's output so machine-applicable code remediations can be consumed by downstream tools (LSP, GitHub Code Scanning, auto-fix bots).

**Architecture:** Extend the BAML `Finding` schema with one optional `fixReplacementText` field. On output, map it to spec-compliant SARIF `Fix` / `ArtifactChange` / `Replacement` structs. The existing `gavel/recommendation` property stays intact for backward compatibility.

**Tech Stack:** Go, BAML (baml-cli codegen), SARIF 2.1.0, existing Gavel test infrastructure.

**Spec:** `docs/superpowers/specs/2026-04-13-sarif-fixes-design.md`

**Related issue:** chris-regnier/gavel#78. LSP consumption is a separate follow-up in chris-regnier/gavel#100.

---

## File Structure

Files to create:
- None (all changes go into existing files)

Files to modify:
- `internal/sarif/sarif.go` — add `Fix`, `ArtifactChange`, `Replacement` types; add `Fixes` to `Result`
- `internal/sarif/sarif_test.go` — JSON marshaling tests for the new types
- `internal/sarif/assembler_test.go` — verify dedup preserves `Fixes`
- `baml_src/analyze.baml` — add `fixReplacementText` to `Finding`, update prompt
- `internal/analyzer/analyzer.go` — add `FixReplacementText` to `Finding`, attach `Fixes` in `Analyze`
- `internal/analyzer/bamlclient.go` — copy new field in `convertFindings`
- `internal/analyzer/analyzer_test.go` — test fix emission (populated + empty)
- `baml_client/**` — regenerated via `task generate` (do NOT hand-edit)

Each task below is self-contained and commits before moving on.

---

## Task 1: Add SARIF `Fix`, `ArtifactChange`, `Replacement` types

**Files:**
- Modify: `internal/sarif/sarif.go` (add types near the other Result-related types, ~line 156)
- Modify: `internal/sarif/sarif_test.go` (add a new test)

- [ ] **Step 1: Write the failing marshaling test**

Add this test at the end of `internal/sarif/sarif_test.go`:

```go
func TestResult_FixMarshaling(t *testing.T) {
	r := Result{
		RuleID:  "hardcoded-credentials",
		Level:   "error",
		Message: Message{Text: "Hard-coded credential detected"},
		Locations: []Location{{
			PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "config.go"},
				Region:           Region{StartLine: 42, EndLine: 42},
			},
		}},
		Fixes: []Fix{{
			Description: Message{Text: "Replace hardcoded credential with env var"},
			ArtifactChanges: []ArtifactChange{{
				ArtifactLocation: ArtifactLocation{URI: "config.go"},
				Replacements: []Replacement{{
					DeletedRegion: Region{StartLine: 42, EndLine: 42},
					InsertedContent: &ArtifactContent{
						Text: `os.Getenv("DB_PASSWORD")`,
					},
				}},
			}},
		}},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}

	// Round-trip through JSON to verify the shape.
	var parsed Result
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if len(parsed.Fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(parsed.Fixes))
	}
	fix := parsed.Fixes[0]
	if fix.Description.Text != "Replace hardcoded credential with env var" {
		t.Errorf("fix description not preserved: got %q", fix.Description.Text)
	}
	if len(fix.ArtifactChanges) != 1 {
		t.Fatalf("expected 1 artifactChange, got %d", len(fix.ArtifactChanges))
	}
	ac := fix.ArtifactChanges[0]
	if ac.ArtifactLocation.URI != "config.go" {
		t.Errorf("artifactLocation URI not preserved: got %q", ac.ArtifactLocation.URI)
	}
	if len(ac.Replacements) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(ac.Replacements))
	}
	rep := ac.Replacements[0]
	if rep.DeletedRegion.StartLine != 42 || rep.DeletedRegion.EndLine != 42 {
		t.Errorf("deletedRegion lines wrong: %+v", rep.DeletedRegion)
	}
	if rep.InsertedContent == nil || rep.InsertedContent.Text != `os.Getenv("DB_PASSWORD")` {
		t.Errorf("insertedContent not preserved: %+v", rep.InsertedContent)
	}

	// Confirm SARIF-standard JSON field names appear in the serialized form.
	s := string(data)
	for _, want := range []string{`"fixes"`, `"artifactChanges"`, `"replacements"`, `"deletedRegion"`, `"insertedContent"`} {
		if !strings.Contains(s, want) {
			t.Errorf("serialized form missing field %s: %s", want, s)
		}
	}
}

func TestResult_OmitsFixesWhenEmpty(t *testing.T) {
	r := Result{
		RuleID:  "no-fix-rule",
		Level:   "warning",
		Message: Message{Text: "Advisory only"},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(data), `"fixes"`) {
		t.Errorf("expected fixes field to be omitted when empty, got: %s", string(data))
	}
}
```

Both tests reference `strings`, so add `"strings"` to the existing import block at the top of `sarif_test.go` if it's not already there.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sarif/ -run 'TestResult_FixMarshaling|TestResult_OmitsFixesWhenEmpty' -v`

Expected: FAIL with a compile error like `undefined: Fix` or `r.Fixes undefined`.

- [ ] **Step 3: Add the new SARIF types**

In `internal/sarif/sarif.go`, add these type definitions after the existing `ArtifactContent` type (after line 156):

```go
// Fix represents a proposed fix for a result, per SARIF 2.1.0 §3.55.
// Downstream tools (LSP, GitHub Code Scanning, auto-fix bots) can apply
// the replacements structurally to remediate the finding.
type Fix struct {
	Description     Message          `json:"description,omitempty"`
	ArtifactChanges []ArtifactChange `json:"artifactChanges"`
}

// ArtifactChange represents a set of replacements applied to a single
// artifact (source file), per SARIF 2.1.0 §3.56.
type ArtifactChange struct {
	ArtifactLocation ArtifactLocation `json:"artifactLocation"`
	Replacements     []Replacement    `json:"replacements"`
}

// Replacement represents a single deletion-plus-insertion within an artifact,
// per SARIF 2.1.0 §3.57. InsertedContent is optional — omitting it expresses
// a pure deletion.
type Replacement struct {
	DeletedRegion   Region           `json:"deletedRegion"`
	InsertedContent *ArtifactContent `json:"insertedContent,omitempty"`
}
```

- [ ] **Step 4: Add `Fixes` field to `Result`**

In `internal/sarif/sarif.go`, modify the existing `Result` struct (currently lines 103-113) by adding a `Fixes` field. The full updated struct:

```go
type Result struct {
	RuleID              string                 `json:"ruleId"`
	Level               string                 `json:"level"`
	Message             Message                `json:"message"`
	Locations           []Location             `json:"locations,omitempty"`
	Fingerprints        map[string]string      `json:"fingerprints,omitempty"`
	PartialFingerprints map[string]string      `json:"partialFingerprints,omitempty"`
	BaselineState       string                 `json:"baselineState,omitempty"`
	Properties          map[string]interface{} `json:"properties,omitempty"`
	Suppressions        []SARIFSuppression     `json:"suppressions,omitempty"`
	Fixes               []Fix                  `json:"fixes,omitempty"`
}
```

The `omitempty` tag ensures backward compatibility — results without fixes serialize identically to before.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/sarif/ -run 'TestResult_FixMarshaling|TestResult_OmitsFixesWhenEmpty' -v`

Expected: PASS both tests.

- [ ] **Step 6: Run full SARIF package tests to confirm no regressions**

Run: `go test ./internal/sarif/ -v`

Expected: All tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/sarif/sarif.go internal/sarif/sarif_test.go
git commit -m "$(cat <<'EOF'
feat(sarif): add Fix/ArtifactChange/Replacement types (#78)

Introduces SARIF 2.1.0 §3.55-3.57 types and an optional Fixes field on
Result. Backward compatible: omitempty keeps existing output unchanged
for findings without fixes.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Verify dedup preserves `Fixes`

**Files:**
- Modify: `internal/sarif/assembler_test.go` (add a new test; no source changes needed — existing dedup carries the whole `Result` through)

- [ ] **Step 1: Write the failing test**

Add this test to `internal/sarif/assembler_test.go`:

```go
func TestAssemble_DedupPreservesFixes(t *testing.T) {
	fix := Fix{
		Description: Message{Text: "Replace magic constant"},
		ArtifactChanges: []ArtifactChange{{
			ArtifactLocation: ArtifactLocation{URI: "foo.go"},
			Replacements: []Replacement{{
				DeletedRegion:   Region{StartLine: 12, EndLine: 12},
				InsertedContent: &ArtifactContent{Text: "MaxRetries"},
			}},
		}},
	}

	results := []Result{
		{
			RuleID:  "magic-number",
			Level:   "warning",
			Message: Message{Text: "issue"},
			Locations: []Location{{PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "foo.go"},
				Region:           Region{StartLine: 10, EndLine: 15},
			}}},
			Properties: map[string]interface{}{"gavel/confidence": 0.7},
		},
		{
			RuleID:  "magic-number",
			Level:   "warning",
			Message: Message{Text: "issue (higher confidence)"},
			Locations: []Location{{PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "foo.go"},
				Region:           Region{StartLine: 12, EndLine: 18},
			}}},
			Properties: map[string]interface{}{"gavel/confidence": 0.95},
			Fixes:      []Fix{fix},
		},
	}

	log := Assemble(results, nil, "files", "code-reviewer")
	if len(log.Runs[0].Results) != 1 {
		t.Fatalf("expected dedup to 1 result, got %d", len(log.Runs[0].Results))
	}
	surviving := log.Runs[0].Results[0]
	if len(surviving.Fixes) != 1 {
		t.Fatalf("expected surviving result to keep its Fixes, got %d", len(surviving.Fixes))
	}
	if surviving.Fixes[0].ArtifactChanges[0].Replacements[0].InsertedContent.Text != "MaxRetries" {
		t.Errorf("fix content not preserved through dedup: %+v", surviving.Fixes[0])
	}
}
```

- [ ] **Step 2: Run test to verify it passes immediately (no source change needed)**

Run: `go test ./internal/sarif/ -run TestAssemble_DedupPreservesFixes -v`

Expected: PASS. The dedup logic in `assembler.go` passes entire `Result` values through the `best` map, so `Fixes` travels with the winning result automatically. If this fails, stop and investigate — the spec assumed this behavior.

- [ ] **Step 3: Run full SARIF package tests to confirm no regressions**

Run: `go test ./internal/sarif/ -v`

Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/sarif/assembler_test.go
git commit -m "$(cat <<'EOF'
test(sarif): verify dedup preserves Fixes field (#78)

Locks in the expectation that Result dedup carries the winning result's
Fixes through to SARIF output.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Extend BAML schema with `fixReplacementText`

**Files:**
- Modify: `baml_src/analyze.baml`
- Modify (via `task generate`): `baml_client/**` — do NOT hand-edit any generated file

- [ ] **Step 1: Update the `Finding` class and prompt**

Replace the entire contents of `baml_src/analyze.baml` with:

```
class Finding {
  ruleId string @description("The policy name this finding relates to")
  level string @description("One of: error, warning, note, none")
  message string @description("Concise description of the issue")
  filePath string @description("Path to the file containing the issue")
  startLine int @description("Line number where the issue starts")
  endLine int @description("Line number where the issue ends")
  recommendation string @description("Suggested fix or action")
  explanation string @description("Longer reasoning about why this is an issue")
  confidence float @description("0.0 to 1.0, how confident you are in this finding")
  fixReplacementText string? @description("Optional raw replacement text for the flagged region (startLine to endLine). Leave empty for vague or structural findings. Do not use markdown code fences.")
}

function AnalyzeCode(
  code: string,
  policies: string,
  personaPrompt: string,
  additionalContext: string
) -> Finding[] {
  client OpenRouter
  prompt #"
    {{ personaPrompt }}

    {% if additionalContext != "" %}
    ===== ADDITIONAL CONTEXT =====
    The following context may be relevant to your analysis:

    {{ additionalContext }}

    ===== END CONTEXT =====
    {% endif %}

    ===== POLICIES TO CHECK =====
    Analyze the content against these specific policies. Only report genuine violations.
    If a policy doesn't apply to this content, don't force a finding.

    {{ policies }}

    ===== CONTENT TO ANALYZE =====
    {{ code }}

    ===== INSTRUCTIONS =====
    For each issue you find:
    1. Identify the exact line numbers where it occurs
    2. Write a concise message (one sentence)
    3. Provide a detailed explanation following your persona's tone
    4. Suggest a specific, actionable recommendation
    5. Assign an appropriate confidence level based on the guidance above
    6. If you can express a machine-applicable fix, populate fixReplacementText with
       the raw replacement text that should replace lines startLine through endLine
       (inclusive). Provide raw text only — no markdown code fences, no prose. If the
       fix is vague or structural (e.g. "consider restructuring this module"), leave
       fixReplacementText empty.

    Only report genuine issues. Quality over quantity.

    {{ ctx.output_format }}
  "#
}
```

- [ ] **Step 2: Regenerate the BAML client**

Run: `task generate`

Expected: `baml-cli generate` runs and updates files under `baml_client/`. The generated `types.Finding` struct in `baml_client/types/classes.go` should now have a `FixReplacementText` field. The BAML client generator decides the Go type — for an optional string field, it is typically `*string` or `string` (Go zero-value is sufficient).

- [ ] **Step 3: Verify `baml_client/` regenerated the `Finding` type**

Run: `grep -n 'FixReplacementText' baml_client/types/classes.go`

Expected: at least one match showing the new field. Note the exact Go type used (`string` or `*string`) — you'll need it in Task 4.

If the grep returns no matches, `task generate` did not pick up the schema change. Re-run and inspect the output for errors.

- [ ] **Step 4: Commit**

```bash
git add baml_src/analyze.baml baml_client/
git commit -m "$(cat <<'EOF'
feat(baml): add optional fixReplacementText to Finding (#78)

Extends the Finding schema with a raw replacement text field the LLM
can populate when it knows a machine-applicable fix for a finding.
Regenerates baml_client via task generate.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Propagate `FixReplacementText` through `analyzer.Finding`

**Files:**
- Modify: `internal/analyzer/analyzer.go` (add field to `Finding` struct, ~line 20-30)
- Modify: `internal/analyzer/bamlclient.go` (update `convertFindings`, ~line 150-166)

- [ ] **Step 1: Add the field to `analyzer.Finding`**

In `internal/analyzer/analyzer.go`, update the `Finding` struct to add `FixReplacementText`:

```go
// Finding represents a single finding returned by the BAML analysis.
type Finding struct {
	RuleID             string  `json:"ruleId"`
	Level              string  `json:"level"`
	Message            string  `json:"message"`
	FilePath           string  `json:"filePath"`
	StartLine          int     `json:"startLine"`
	EndLine            int     `json:"endLine"`
	Recommendation     string  `json:"recommendation"`
	Explanation        string  `json:"explanation"`
	Confidence         float64 `json:"confidence"`
	FixReplacementText string  `json:"fixReplacementText,omitempty"`
}
```

- [ ] **Step 2: Update `convertFindings` to copy the new field**

In `internal/analyzer/bamlclient.go`, update `convertFindings` (currently lines 150-166). The exact assignment depends on what type baml-cli generated in Task 3 Step 3:

If baml generated `FixReplacementText string`:

```go
func convertFindings(bamlFindings []types.Finding) []Finding {
	findings := make([]Finding, len(bamlFindings))
	for i, f := range bamlFindings {
		findings[i] = Finding{
			RuleID:             f.RuleId,
			Level:              f.Level,
			Message:            f.Message,
			FilePath:           f.FilePath,
			StartLine:          int(f.StartLine),
			EndLine:            int(f.EndLine),
			Recommendation:     f.Recommendation,
			Explanation:        f.Explanation,
			Confidence:         f.Confidence,
			FixReplacementText: f.FixReplacementText,
		}
	}
	return findings
}
```

If baml generated `FixReplacementText *string` (pointer for optional), dereference safely:

```go
func convertFindings(bamlFindings []types.Finding) []Finding {
	findings := make([]Finding, len(bamlFindings))
	for i, f := range bamlFindings {
		fixText := ""
		if f.FixReplacementText != nil {
			fixText = *f.FixReplacementText
		}
		findings[i] = Finding{
			RuleID:             f.RuleId,
			Level:              f.Level,
			Message:            f.Message,
			FilePath:           f.FilePath,
			StartLine:          int(f.StartLine),
			EndLine:            int(f.EndLine),
			Recommendation:     f.Recommendation,
			Explanation:        f.Explanation,
			Confidence:         f.Confidence,
			FixReplacementText: fixText,
		}
	}
	return findings
}
```

Use whichever form matches the generated type you recorded in Task 3 Step 3.

- [ ] **Step 3: Run analyzer package build to confirm it compiles**

Run: `go build ./internal/analyzer/`

Expected: builds cleanly. If you get `f.FixReplacementText undefined`, the BAML generator did not add the field — go back to Task 3 and fix.

- [ ] **Step 4: Run existing analyzer tests to confirm no regressions**

Run: `go test ./internal/analyzer/ -v`

Expected: all existing tests still pass. The new field defaults to empty string everywhere, which matches the pre-change behavior.

- [ ] **Step 5: Commit**

```bash
git add internal/analyzer/analyzer.go internal/analyzer/bamlclient.go
git commit -m "$(cat <<'EOF'
feat(analyzer): propagate FixReplacementText from BAML (#78)

Adds FixReplacementText to analyzer.Finding and copies it from the
generated BAML type in convertFindings. No behavior change yet — the
analyzer does not use the field until the next task.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Wire analyzer to emit `sarif.Fixes`

**Files:**
- Modify: `internal/analyzer/analyzer.go` (result construction inside `Analyze`, ~lines 134-144)
- Modify: `internal/analyzer/analyzer_test.go` (add test cases)

- [ ] **Step 1: Write failing tests for fix emission and the empty case**

Add these tests to `internal/analyzer/analyzer_test.go` (after `TestAnalyzer_Analyze`):

```go
func TestAnalyzer_EmitsFixWhenReplacementPresent(t *testing.T) {
	mock := &mockBAMLClient{
		findings: []Finding{
			{
				RuleID:             "hardcoded-credentials",
				Level:              "error",
				Message:            "Hardcoded password",
				FilePath:           "config.go",
				StartLine:          42,
				EndLine:            42,
				Recommendation:     "Use an environment variable",
				Confidence:         0.95,
				FixReplacementText: `password := os.Getenv("DB_PASSWORD")`,
			},
		},
	}

	a := NewAnalyzer(mock)
	artifacts := []input.Artifact{
		{Path: "config.go", Content: "package main\n\n" + strings.Repeat("line\n", 50), Kind: input.KindFile},
	}
	policies := map[string]config.Policy{
		"hardcoded-credentials": {
			Severity:    "error",
			Instruction: "No hardcoded credentials",
			Enabled:     true,
		},
	}

	results, err := a.Analyze(context.Background(), artifacts, policies, "test persona")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if len(r.Fixes) != 1 {
		t.Fatalf("expected 1 fix on result, got %d", len(r.Fixes))
	}
	fix := r.Fixes[0]
	if fix.Description.Text != "Use an environment variable" {
		t.Errorf("fix description should mirror recommendation, got %q", fix.Description.Text)
	}
	if len(fix.ArtifactChanges) != 1 {
		t.Fatalf("expected 1 artifactChange, got %d", len(fix.ArtifactChanges))
	}
	ac := fix.ArtifactChanges[0]
	if ac.ArtifactLocation.URI != "config.go" {
		t.Errorf("expected artifactLocation URI 'config.go', got %q", ac.ArtifactLocation.URI)
	}
	if len(ac.Replacements) != 1 {
		t.Fatalf("expected 1 replacement, got %d", len(ac.Replacements))
	}
	rep := ac.Replacements[0]
	if rep.DeletedRegion.StartLine != 42 || rep.DeletedRegion.EndLine != 42 {
		t.Errorf("deletedRegion should span finding region, got start=%d end=%d",
			rep.DeletedRegion.StartLine, rep.DeletedRegion.EndLine)
	}
	if rep.InsertedContent == nil || rep.InsertedContent.Text != `password := os.Getenv("DB_PASSWORD")` {
		t.Errorf("insertedContent not set correctly: %+v", rep.InsertedContent)
	}
}

func TestAnalyzer_NoFixWhenReplacementEmpty(t *testing.T) {
	mock := &mockBAMLClient{
		findings: []Finding{
			{
				RuleID:             "structural-issue",
				Level:              "note",
				Message:            "Consider restructuring this module",
				FilePath:           "pkg/foo.go",
				StartLine:          5,
				EndLine:            50,
				Recommendation:     "Extract smaller functions",
				Confidence:         0.6,
				FixReplacementText: "", // no machine-applicable fix
			},
		},
	}

	a := NewAnalyzer(mock)
	artifacts := []input.Artifact{
		{Path: "pkg/foo.go", Content: strings.Repeat("line\n", 60), Kind: input.KindFile},
	}
	policies := map[string]config.Policy{
		"structural-issue": {Severity: "note", Instruction: "Design feedback", Enabled: true},
	}

	results, err := a.Analyze(context.Background(), artifacts, policies, "test persona")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(results[0].Fixes) != 0 {
		t.Errorf("expected no fixes when FixReplacementText is empty, got %d", len(results[0].Fixes))
	}
}
```

Note: the test file already imports `strings` (used in other tests) per the existing file. If not, add it to the import block.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/analyzer/ -run 'TestAnalyzer_EmitsFixWhenReplacementPresent|TestAnalyzer_NoFixWhenReplacementEmpty' -v`

Expected: `TestAnalyzer_EmitsFixWhenReplacementPresent` FAILS with "expected 1 fix on result, got 0". `TestAnalyzer_NoFixWhenReplacementEmpty` PASSES (no code emits fixes yet, so it naturally passes).

- [ ] **Step 3: Wire fix emission in `Analyze`**

In `internal/analyzer/analyzer.go`, inside the inner `for _, f := range findings` loop (currently ending around line 145), replace the existing `allResults = append(...)` block with logic that builds the `Result` separately and conditionally attaches `Fixes`:

```go
			result := sarif.Result{
				RuleID:    f.RuleID,
				Level:     f.Level,
				Message:   sarif.Message{Text: f.Message},
				Locations: []sarif.Location{loc},
				Properties: map[string]interface{}{
					"gavel/recommendation": f.Recommendation,
					"gavel/explanation":    f.Explanation,
					"gavel/confidence":     f.Confidence,
				},
			}

			if f.FixReplacementText != "" {
				result.Fixes = []sarif.Fix{{
					Description: sarif.Message{Text: f.Recommendation},
					ArtifactChanges: []sarif.ArtifactChange{{
						ArtifactLocation: sarif.ArtifactLocation{URI: path},
						Replacements: []sarif.Replacement{{
							DeletedRegion: sarif.Region{
								StartLine: f.StartLine,
								EndLine:   f.EndLine,
							},
							InsertedContent: &sarif.ArtifactContent{
								Text: f.FixReplacementText,
							},
						}},
					}},
				}}
			}

			allResults = append(allResults, result)
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./internal/analyzer/ -run 'TestAnalyzer_EmitsFixWhenReplacementPresent|TestAnalyzer_NoFixWhenReplacementEmpty' -v`

Expected: both PASS.

- [ ] **Step 5: Run full analyzer and sarif test suites**

Run: `go test ./internal/analyzer/ ./internal/sarif/ -v`

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/analyzer/analyzer.go internal/analyzer/analyzer_test.go
git commit -m "$(cat <<'EOF'
feat(analyzer): emit SARIF Fixes when LLM provides replacement text (#78)

When FixReplacementText is populated on a Finding, attach a SARIF Fix
to the corresponding Result. The fix description mirrors the existing
recommendation; the replacement targets the finding's own region.
Findings without replacement text produce no Fixes (omitted from output).

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Run full CI-parity check

**Files:**
- No modifications. Just verification.

- [ ] **Step 1: Run `task check`**

Run: `task check`

Expected: `task check` runs `generate + lint + test + cross-compile` per CLAUDE.md and passes. This catches any regressions across the whole project — including any package that depends on `sarif.Result` or `analyzer.Finding`.

- [ ] **Step 2: If any test fails, stop and investigate**

Common failure modes:
- `baml_client/` regenerated differently on this machine than on Task 3 — re-run `task generate` and re-commit.
- A downstream test hard-asserts the exact JSON of a Result. Update the assertion only if the new `fixes` field should omit (it should, via `omitempty`).
- Lint flags an unused field. Verify the field is referenced in `convertFindings` and in `Analyze`.

- [ ] **Step 3: No commit needed for this task (verification only)**

If `task check` passed without changes, proceed to Task 7.

---

## Task 7: Self-review and cleanup

**Files:**
- No modifications. Just review.

- [ ] **Step 1: Review the commit log**

Run: `git log --oneline origin/main..HEAD`

Expected: 5 commits (one per implementation task) with clear, conventional messages referencing #78.

- [ ] **Step 2: Review the full diff**

Run: `git diff origin/main..HEAD -- internal/sarif/ internal/analyzer/ baml_src/analyze.baml`

Look for:
- No stray debug prints, `fmt.Println`, or commented-out code.
- No changes outside the listed scope (sarif, analyzer, baml_src, plus baml_client regeneration).
- No modifications to unrelated files.

- [ ] **Step 3: Sanity-check an end-to-end run locally (optional but recommended)**

If you have an Ollama or OpenRouter backend available:

```bash
task build
OPENROUTER_API_KEY=... ./dist/gavel analyze --file internal/config/config.go
```

Then inspect the most recent SARIF result to confirm:
- Without a fix: no `fixes` array in the JSON.
- With a fix (if the LLM supplies one): the `fixes` array matches the expected shape.

This is a sanity check only — the tests are authoritative.

- [ ] **Step 4: No commit needed (review only)**

The implementation is complete once `task check` passes and this self-review finds nothing.

---

## Done criteria

- [ ] `task check` passes.
- [ ] `internal/sarif/sarif.go` exports `Fix`, `ArtifactChange`, `Replacement` types and `Result.Fixes` field.
- [ ] `internal/analyzer/analyzer.go` populates `Result.Fixes` when `Finding.FixReplacementText` is non-empty.
- [ ] `internal/analyzer/bamlclient.go` copies `FixReplacementText` from the generated BAML type.
- [ ] `baml_src/analyze.baml` has the new field and updated prompt instructions.
- [ ] Backward compatibility confirmed: existing SARIF output is unchanged for findings without a fix.
- [ ] Issue chris-regnier/gavel#78 can be closed (referencing this plan and commits).
