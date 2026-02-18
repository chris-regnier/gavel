# Output Formatting & Structured Logging Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `--format` flag (json/sarif/markdown/pretty) with TTY auto-detection, GitHub-compliant SARIF, GFM markdown for PR comments, and structured logging via slog.

**Architecture:** New `internal/output/` package with a `Formatter` interface and four implementations (JSON, SARIF, Markdown, Pretty). Factory function + TTY detection selects the formatter. Structured logging via `log/slog` replaces scattered `log.Printf`/`fmt.Fprintln` calls, with `--quiet`/`--verbose`/`--debug` flags on the root command.

**Tech Stack:** Go 1.24, `log/slog` (stdlib), `github.com/mattn/go-isatty` (already indirect dep), `github.com/charmbracelet/lipgloss` (already direct dep)

---

### Task 1: Formatter Interface & Factory

**Files:**
- Create: `internal/output/formatter.go`
- Test: `internal/output/formatter_test.go`

**Step 1: Write the failing test**

Create `internal/output/formatter_test.go`:

```go
package output

import (
	"testing"
)

func TestResolveFormat_ExplicitFlag(t *testing.T) {
	tests := []struct {
		flag   string
		tty    bool
		want   string
	}{
		{"json", true, "json"},
		{"json", false, "json"},
		{"sarif", true, "sarif"},
		{"markdown", false, "markdown"},
		{"pretty", false, "pretty"},
	}

	for _, tt := range tests {
		got := ResolveFormat(tt.flag, tt.tty)
		if got != tt.want {
			t.Errorf("ResolveFormat(%q, %v) = %q, want %q", tt.flag, tt.tty, got, tt.want)
		}
	}
}

func TestResolveFormat_AutoDetect(t *testing.T) {
	if got := ResolveFormat("", true); got != "pretty" {
		t.Errorf("ResolveFormat('', true) = %q, want 'pretty'", got)
	}
	if got := ResolveFormat("", false); got != "json" {
		t.Errorf("ResolveFormat('', false) = %q, want 'json'", got)
	}
}

func TestNewFormatter_ValidFormats(t *testing.T) {
	for _, name := range []string{"json", "sarif", "markdown", "pretty"} {
		f, err := NewFormatter(name)
		if err != nil {
			t.Errorf("NewFormatter(%q) error: %v", name, err)
		}
		if f == nil {
			t.Errorf("NewFormatter(%q) returned nil", name)
		}
	}
}

func TestNewFormatter_InvalidFormat(t *testing.T) {
	_, err := NewFormatter("xml")
	if err == nil {
		t.Error("NewFormatter('xml') should return error")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/output/ -v`
Expected: FAIL — package doesn't exist yet

**Step 3: Write minimal implementation**

Create `internal/output/formatter.go`:

```go
package output

import (
	"fmt"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

// Formatter converts analysis output to a specific format.
type Formatter interface {
	Format(result *AnalysisOutput) ([]byte, error)
}

// AnalysisOutput bundles everything a formatter needs.
type AnalysisOutput struct {
	Verdict  *store.Verdict
	SARIFLog *sarif.Log
	Stats    *analyzer.TieredAnalyzerStats // optional, nil if not collected
}

// ResolveFormat determines the output format from the --format flag and TTY state.
// If format is empty: returns "pretty" if stdout is a TTY, "json" otherwise.
func ResolveFormat(flagValue string, stdoutIsTTY bool) string {
	if flagValue != "" {
		return flagValue
	}
	if stdoutIsTTY {
		return "pretty"
	}
	return "json"
}

// NewFormatter returns the formatter for the given format name.
func NewFormatter(format string) (Formatter, error) {
	switch format {
	case "json":
		return &JSONFormatter{}, nil
	case "sarif":
		return &SARIFFormatter{}, nil
	case "markdown":
		return &MarkdownFormatter{}, nil
	case "pretty":
		return &PrettyFormatter{}, nil
	default:
		return nil, fmt.Errorf("unknown format %q (valid: json, sarif, markdown, pretty)", format)
	}
}
```

Also create stubs for each formatter so the package compiles:

Create `internal/output/json.go`:
```go
package output

import "encoding/json"

// JSONFormatter outputs the verdict as indented JSON.
type JSONFormatter struct{}

func (f *JSONFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	return json.MarshalIndent(result.Verdict, "", "  ")
}
```

Create `internal/output/sarif.go`:
```go
package output

// SARIFFormatter outputs GitHub-compliant SARIF JSON.
type SARIFFormatter struct{}

func (f *SARIFFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	// Stub — implemented in Task 3
	return nil, nil
}
```

Create `internal/output/markdown.go`:
```go
package output

// MarkdownFormatter outputs GFM markdown for PR comments.
type MarkdownFormatter struct{}

func (f *MarkdownFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	// Stub — implemented in Task 4
	return nil, nil
}
```

Create `internal/output/pretty.go`:
```go
package output

// PrettyFormatter outputs colored terminal text.
type PrettyFormatter struct{}

func (f *PrettyFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	// Stub — implemented in Task 5
	return nil, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/output/ -v`
Expected: PASS — all 4 tests pass

**Step 5: Commit**

```bash
git add internal/output/
git commit -m "feat(output): add Formatter interface, factory, and TTY resolution"
```

---

### Task 2: JSON Formatter

**Files:**
- Modify: `internal/output/json.go`
- Test: `internal/output/json_test.go`

**Step 1: Write the failing test**

Create `internal/output/json_test.go`:

```go
package output

import (
	"encoding/json"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

func TestJSONFormatter_Format(t *testing.T) {
	verdict := &store.Verdict{
		Decision: "review",
		Reason:   "Decision: review based on 2 findings",
		RelevantFindings: []sarif.Result{
			{RuleID: "SEC001", Level: "error", Message: sarif.Message{Text: "issue"}},
		},
	}

	f := &JSONFormatter{}
	data, err := f.Format(&AnalysisOutput{Verdict: verdict})
	if err != nil {
		t.Fatal(err)
	}

	// Verify it's valid JSON
	var parsed store.Verdict
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed.Decision != "review" {
		t.Errorf("decision = %q, want 'review'", parsed.Decision)
	}
}

func TestJSONFormatter_NilVerdict(t *testing.T) {
	f := &JSONFormatter{}
	_, err := f.Format(&AnalysisOutput{})
	if err != nil {
		t.Fatalf("expected nil verdict to marshal without error, got: %v", err)
	}
}

func TestJSONFormatter_TrailingNewline(t *testing.T) {
	f := &JSONFormatter{}
	data, err := f.Format(&AnalysisOutput{
		Verdict: &store.Verdict{Decision: "merge"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Error("JSON output should end with newline for shell friendliness")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/output/ -run TestJSON -v`
Expected: FAIL — `TestJSONFormatter_TrailingNewline` fails (current impl doesn't add newline)

**Step 3: Update implementation**

Update `internal/output/json.go`:

```go
package output

import "encoding/json"

// JSONFormatter outputs the verdict as indented JSON.
type JSONFormatter struct{}

func (f *JSONFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	data, err := json.MarshalIndent(result.Verdict, "", "  ")
	if err != nil {
		return nil, err
	}
	// Append newline for shell friendliness
	return append(data, '\n'), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/output/ -run TestJSON -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/output/json.go internal/output/json_test.go
git commit -m "feat(output): implement JSON formatter with trailing newline"
```

---

### Task 3: SARIF Formatter (GitHub-Compliant)

**Files:**
- Modify: `internal/output/sarif.go`
- Test: `internal/output/sarif_test.go`
- Modify: `internal/sarif/sarif.go` (add `PartialFingerprints` and `Invocation` types)

**Step 1: Write the failing tests**

Create `internal/output/sarif_test.go`:

```go
package output

import (
	"encoding/json"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

func testSARIFLog() *sarif.Log {
	return &sarif.Log{
		Schema:  sarif.SchemaURI,
		Version: sarif.Version,
		Runs: []sarif.Run{{
			Tool: sarif.Tool{
				Driver: sarif.Driver{
					Name:    "gavel",
					Version: "0.1.0",
					Rules: []sarif.ReportingDescriptor{
						{ID: "SEC001", ShortDescription: sarif.Message{Text: "Hardcoded secret"}},
					},
				},
			},
			Results: []sarif.Result{
				{
					RuleID:  "SEC001",
					Level:   "error",
					Message: sarif.Message{Text: "Hardcoded secret detected"},
					Locations: []sarif.Location{{
						PhysicalLocation: sarif.PhysicalLocation{
							ArtifactLocation: sarif.ArtifactLocation{URI: "config/db.go"},
							Region:           sarif.Region{StartLine: 42, EndLine: 42},
						},
					}},
					Properties: map[string]interface{}{
						"gavel/confidence": 0.95,
						"gavel/tier":       "comprehensive",
						"gavel/cwe":        []string{"CWE-798"},
					},
				},
			},
			Properties: map[string]interface{}{
				"gavel/inputScope": "files",
				"gavel/persona":    "security",
			},
		}},
	}
}

func TestSARIFFormatter_ValidJSON(t *testing.T) {
	f := &SARIFFormatter{}
	data, err := f.Format(&AnalysisOutput{
		SARIFLog: testSARIFLog(),
		Verdict:  &store.Verdict{Decision: "reject"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed sarif.Log
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid SARIF JSON: %v", err)
	}
	if parsed.Version != "2.1.0" {
		t.Errorf("version = %q, want '2.1.0'", parsed.Version)
	}
}

func TestSARIFFormatter_HasPartialFingerprints(t *testing.T) {
	f := &SARIFFormatter{}
	data, err := f.Format(&AnalysisOutput{
		SARIFLog: testSARIFLog(),
		Verdict:  &store.Verdict{Decision: "reject"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed sarif.Log
	json.Unmarshal(data, &parsed)

	result := parsed.Runs[0].Results[0]
	if result.PartialFingerprints == nil {
		t.Fatal("expected partialFingerprints to be set")
	}
	hash, ok := result.PartialFingerprints["primaryLocationLineHash"]
	if !ok || hash == "" {
		t.Error("expected primaryLocationLineHash in partialFingerprints")
	}
}

func TestSARIFFormatter_HasSecuritySeverity(t *testing.T) {
	f := &SARIFFormatter{}
	data, err := f.Format(&AnalysisOutput{
		SARIFLog: testSARIFLog(),
		Verdict:  &store.Verdict{Decision: "reject"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed sarif.Log
	json.Unmarshal(data, &parsed)

	result := parsed.Runs[0].Results[0]
	severity, ok := result.Properties["security-severity"].(float64)
	if !ok {
		t.Fatal("expected security-severity in properties")
	}
	if severity != 8.0 {
		t.Errorf("security-severity = %v, want 8.0 for error level", severity)
	}
}

func TestSARIFFormatter_HasPrecision(t *testing.T) {
	f := &SARIFFormatter{}
	data, err := f.Format(&AnalysisOutput{
		SARIFLog: testSARIFLog(),
		Verdict:  &store.Verdict{Decision: "reject"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed sarif.Log
	json.Unmarshal(data, &parsed)

	result := parsed.Runs[0].Results[0]
	precision, ok := result.Properties["precision"].(string)
	if !ok {
		t.Fatal("expected precision in properties")
	}
	if precision != "high" {
		t.Errorf("precision = %q, want 'high' for comprehensive tier", precision)
	}
}

func TestSARIFFormatter_HasInformationURI(t *testing.T) {
	f := &SARIFFormatter{}
	data, err := f.Format(&AnalysisOutput{
		SARIFLog: testSARIFLog(),
		Verdict:  &store.Verdict{Decision: "reject"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed sarif.Log
	json.Unmarshal(data, &parsed)

	uri := parsed.Runs[0].Tool.Driver.InformationURI
	if uri != "https://github.com/chris-regnier/gavel" {
		t.Errorf("informationUri = %q, want gavel repo URL", uri)
	}
}

func TestSARIFFormatter_HasInvocations(t *testing.T) {
	f := &SARIFFormatter{}
	data, err := f.Format(&AnalysisOutput{
		SARIFLog: testSARIFLog(),
		Verdict:  &store.Verdict{Decision: "reject"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var parsed sarif.Log
	json.Unmarshal(data, &parsed)

	if len(parsed.Runs[0].Invocations) == 0 {
		t.Fatal("expected invocations to be set")
	}
	inv := parsed.Runs[0].Invocations[0]
	if inv.WorkingDirectory.URI == "" {
		t.Error("expected workingDirectory.uri to be set")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/output/ -run TestSARIF -v`
Expected: FAIL — compilation errors (missing types) and nil return from stub

**Step 3: Add SARIF types for new fields**

Modify `internal/sarif/sarif.go` — add these fields to existing types:

Add `PartialFingerprints` to `Result`:
```go
type Result struct {
	RuleID              string                 `json:"ruleId"`
	Level               string                 `json:"level"`
	Message             Message                `json:"message"`
	Locations           []Location             `json:"locations,omitempty"`
	PartialFingerprints map[string]string      `json:"partialFingerprints,omitempty"`
	Properties          map[string]interface{} `json:"properties,omitempty"`
}
```

Add `Invocations` to `Run`:
```go
type Run struct {
	Tool        Tool                   `json:"tool"`
	Results     []Result               `json:"results"`
	Invocations []Invocation           `json:"invocations,omitempty"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
}
```

Add new types:
```go
type Invocation struct {
	WorkingDirectory ArtifactLocation `json:"workingDirectory"`
	ExecutionSuccessful bool          `json:"executionSuccessful"`
}
```

**Step 4: Implement the SARIF formatter**

Update `internal/output/sarif.go`:

```go
package output

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// SARIFFormatter outputs GitHub Code Scanning compliant SARIF JSON.
type SARIFFormatter struct{}

func (f *SARIFFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	log := result.SARIFLog
	if log == nil {
		return nil, fmt.Errorf("SARIF log is required for sarif format")
	}

	// Enrich for GitHub compliance (operates on a shallow copy of slices)
	f.enrich(log)

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (f *SARIFFormatter) enrich(log *sarif.Log) {
	if len(log.Runs) == 0 {
		return
	}
	run := &log.Runs[0]

	// Set informationUri
	run.Tool.Driver.InformationURI = "https://github.com/chris-regnier/gavel"

	// Add invocations with working directory
	wd, _ := os.Getwd()
	run.Invocations = []sarif.Invocation{{
		WorkingDirectory:    sarif.ArtifactLocation{URI: wd},
		ExecutionSuccessful: true,
	}}

	// Enrich each result
	for i := range run.Results {
		r := &run.Results[i]

		// Partial fingerprints
		if r.PartialFingerprints == nil {
			r.PartialFingerprints = make(map[string]string)
		}
		r.PartialFingerprints["primaryLocationLineHash"] = computeFingerprint(r)

		// Properties enrichment
		if r.Properties == nil {
			r.Properties = make(map[string]interface{})
		}

		// security-severity
		r.Properties["security-severity"] = severityScore(r.Level)

		// precision from tier
		r.Properties["precision"] = precisionFromTier(r.Properties)
	}
}

func computeFingerprint(r *sarif.Result) string {
	uri := ""
	startLine := 0
	if len(r.Locations) > 0 {
		uri = r.Locations[0].PhysicalLocation.ArtifactLocation.URI
		startLine = r.Locations[0].PhysicalLocation.Region.StartLine
	}
	input := r.RuleID + "|" + uri + "|" + strconv.Itoa(startLine) + "|" + r.Message.Text
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:16]) // 32 hex chars
}

func severityScore(level string) float64 {
	switch level {
	case "error":
		return 8.0
	case "warning":
		return 5.0
	case "note":
		return 2.0
	default:
		return 5.0
	}
}

func precisionFromTier(props map[string]interface{}) string {
	tier, _ := props["gavel/tier"].(string)
	switch tier {
	case "comprehensive":
		return "high"
	case "fast":
		return "medium"
	case "instant":
		return "medium"
	default:
		return "medium"
	}
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/output/ -run TestSARIF -v`
Expected: PASS

**Step 6: Run existing SARIF tests to ensure no regressions**

Run: `go test ./internal/sarif/ -v`
Expected: PASS — adding optional fields with `omitempty` is backwards-compatible

**Step 7: Commit**

```bash
git add internal/sarif/sarif.go internal/output/sarif.go internal/output/sarif_test.go
git commit -m "feat(output): implement GitHub-compliant SARIF formatter

Adds partialFingerprints, security-severity, precision, invocations,
and informationUri for GitHub Code Scanning compatibility."
```

---

### Task 4: Markdown Formatter (GFM)

**Files:**
- Modify: `internal/output/markdown.go`
- Test: `internal/output/markdown_test.go`

**Step 1: Write the failing tests**

Create `internal/output/markdown_test.go`:

```go
package output

import (
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

func testMarkdownInput() *AnalysisOutput {
	return &AnalysisOutput{
		Verdict: &store.Verdict{
			Decision: "review",
			Reason:   "Decision: review based on 2 findings",
		},
		SARIFLog: &sarif.Log{
			Runs: []sarif.Run{{
				Tool: sarif.Tool{Driver: sarif.Driver{Name: "gavel"}},
				Results: []sarif.Result{
					{
						RuleID:  "SEC001",
						Level:   "error",
						Message: sarif.Message{Text: "Hardcoded secret detected"},
						Locations: []sarif.Location{{
							PhysicalLocation: sarif.PhysicalLocation{
								ArtifactLocation: sarif.ArtifactLocation{URI: "config/db.go"},
								Region:           sarif.Region{StartLine: 42, EndLine: 42},
							},
						}},
						Properties: map[string]interface{}{
							"gavel/confidence":     0.95,
							"gavel/recommendation": "Use environment variables",
						},
					},
					{
						RuleID:  "ERR003",
						Level:   "warning",
						Message: sarif.Message{Text: "Error not checked"},
						Locations: []sarif.Location{{
							PhysicalLocation: sarif.PhysicalLocation{
								ArtifactLocation: sarif.ArtifactLocation{URI: "handler.go"},
								Region:           sarif.Region{StartLine: 15, EndLine: 15},
							},
						}},
						Properties: map[string]interface{}{
							"gavel/confidence": 0.82,
						},
					},
				},
				Properties: map[string]interface{}{
					"gavel/persona": "code-reviewer",
				},
			}},
		},
	}
}

func TestMarkdownFormatter_HasSummaryHeader(t *testing.T) {
	f := &MarkdownFormatter{}
	data, err := f.Format(testMarkdownInput())
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	if !strings.Contains(md, "## Gavel Analysis Summary") {
		t.Error("expected summary header")
	}
}

func TestMarkdownFormatter_HasDecisionBanner(t *testing.T) {
	f := &MarkdownFormatter{}
	data, err := f.Format(testMarkdownInput())
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	if !strings.Contains(md, "Review") {
		t.Error("expected decision in output")
	}
	if !strings.Contains(md, "Findings") {
		t.Error("expected findings count")
	}
}

func TestMarkdownFormatter_HasSeverityTable(t *testing.T) {
	f := &MarkdownFormatter{}
	data, err := f.Format(testMarkdownInput())
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	if !strings.Contains(md, "| Severity") {
		t.Error("expected severity table")
	}
	if !strings.Contains(md, "error") {
		t.Error("expected error in severity table")
	}
}

func TestMarkdownFormatter_HasCollapsibleFindings(t *testing.T) {
	f := &MarkdownFormatter{}
	data, err := f.Format(testMarkdownInput())
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	if !strings.Contains(md, "<details>") {
		t.Error("expected collapsible details sections")
	}
	if !strings.Contains(md, "SEC001") {
		t.Error("expected rule ID in findings")
	}
}

func TestMarkdownFormatter_SortsBySeverity(t *testing.T) {
	f := &MarkdownFormatter{}
	data, err := f.Format(testMarkdownInput())
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	errorPos := strings.Index(md, "SEC001")
	warningPos := strings.Index(md, "ERR003")
	if errorPos > warningPos {
		t.Error("expected error findings before warning findings")
	}
}

func TestMarkdownFormatter_HasFooter(t *testing.T) {
	f := &MarkdownFormatter{}
	data, err := f.Format(testMarkdownInput())
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	if !strings.Contains(md, "Generated by") {
		t.Error("expected footer with attribution")
	}
	if !strings.Contains(md, "code-reviewer") {
		t.Error("expected persona in footer")
	}
}

func TestMarkdownFormatter_NoFindings(t *testing.T) {
	input := &AnalysisOutput{
		Verdict: &store.Verdict{Decision: "merge"},
		SARIFLog: &sarif.Log{
			Runs: []sarif.Run{{
				Tool:    sarif.Tool{Driver: sarif.Driver{Name: "gavel"}},
				Results: []sarif.Result{},
				Properties: map[string]interface{}{
					"gavel/persona": "code-reviewer",
				},
			}},
		},
	}

	f := &MarkdownFormatter{}
	data, err := f.Format(input)
	if err != nil {
		t.Fatal(err)
	}
	md := string(data)
	if !strings.Contains(md, "Merge") {
		t.Error("expected merge decision")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/output/ -run TestMarkdown -v`
Expected: FAIL — stub returns nil

**Step 3: Implement the Markdown formatter**

Update `internal/output/markdown.go`:

```go
package output

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// MarkdownFormatter outputs GFM markdown suitable for PR comments.
type MarkdownFormatter struct{}

func (f *MarkdownFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	var b strings.Builder

	verdict := result.Verdict
	log := result.SARIFLog

	var results []sarif.Result
	persona := "code-reviewer"
	if log != nil && len(log.Runs) > 0 {
		results = log.Runs[0].Results
		if p, ok := log.Runs[0].Properties["gavel/persona"].(string); ok {
			persona = p
		}
	}

	// Count files and severities
	files := make(map[string]bool)
	severityCounts := make(map[string]int)
	for _, r := range results {
		if len(r.Locations) > 0 {
			files[r.Locations[0].PhysicalLocation.ArtifactLocation.URI] = true
		}
		severityCounts[r.Level]++
	}

	// Header
	b.WriteString("## Gavel Analysis Summary\n\n")

	// Decision banner
	decision := "Review"
	emoji := ":warning:"
	if verdict != nil {
		switch verdict.Decision {
		case "merge":
			decision = "Merge"
			emoji = ":white_check_mark:"
		case "reject":
			decision = "Reject"
			emoji = ":x:"
		case "review":
			decision = "Review Required"
			emoji = ":warning:"
		}
	}
	fmt.Fprintf(&b, "**Decision:** %s %s | **Findings:** %d | **Files:** %d\n\n",
		emoji, decision, len(results), len(files))

	if len(results) == 0 {
		b.WriteString("No findings detected.\n\n")
	} else {
		// Severity table
		b.WriteString("### Findings by Severity\n")
		b.WriteString("| Severity | Count |\n")
		b.WriteString("|----------|-------|\n")
		for _, level := range []string{"error", "warning", "note"} {
			if count, ok := severityCounts[level]; ok {
				fmt.Fprintf(&b, "| %s | %d |\n", level, count)
			}
		}
		b.WriteString("\n")

		// Sort results: error first, then warning, then note; within same level, by file path
		sorted := make([]sarif.Result, len(results))
		copy(sorted, results)
		sort.Slice(sorted, func(i, j int) bool {
			pi := severityPriority(sorted[i].Level)
			pj := severityPriority(sorted[j].Level)
			if pi != pj {
				return pi < pj
			}
			ui := resultURI(sorted[i])
			uj := resultURI(sorted[j])
			return ui < uj
		})

		// Findings
		b.WriteString("### Findings\n\n")
		for _, r := range sorted {
			uri := resultURI(r)
			startLine := 0
			if len(r.Locations) > 0 {
				startLine = r.Locations[0].PhysicalLocation.Region.StartLine
			}

			levelEmoji := severityEmoji(r.Level)
			fmt.Fprintf(&b, "<details>\n<summary>%s <strong>%s</strong> — %s: %s in <code>%s:%d</code></summary>\n\n",
				levelEmoji, r.Level, r.RuleID, r.Message.Text, uri, startLine)

			fmt.Fprintf(&b, "**Rule:** %s\n", r.RuleID)

			if conf, ok := r.Properties["gavel/confidence"].(float64); ok {
				fmt.Fprintf(&b, "**Confidence:** %.2f\n", conf)
			}

			if len(r.Locations) > 0 {
				loc := r.Locations[0].PhysicalLocation
				fmt.Fprintf(&b, "**File:** `%s` lines %d-%d\n",
					loc.ArtifactLocation.URI,
					loc.Region.StartLine,
					loc.Region.EndLine)
			}

			b.WriteString("\n")
			fmt.Fprintf(&b, "> %s\n", r.Message.Text)

			if rec, ok := r.Properties["gavel/recommendation"].(string); ok && rec != "" {
				fmt.Fprintf(&b, "\n**Recommendation:** %s\n", rec)
			}

			b.WriteString("\n</details>\n\n")
		}
	}

	// Footer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "*Generated by [Gavel](https://github.com/chris-regnier/gavel) · %s persona*\n", persona)

	return []byte(b.String()), nil
}

func severityPriority(level string) int {
	switch level {
	case "error":
		return 0
	case "warning":
		return 1
	case "note":
		return 2
	default:
		return 3
	}
}

func severityEmoji(level string) string {
	switch level {
	case "error":
		return ":red_circle:"
	case "warning":
		return ":warning:"
	case "note":
		return ":information_source:"
	default:
		return ""
	}
}

func resultURI(r sarif.Result) string {
	if len(r.Locations) > 0 {
		return r.Locations[0].PhysicalLocation.ArtifactLocation.URI
	}
	return ""
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/output/ -run TestMarkdown -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/output/markdown.go internal/output/markdown_test.go
git commit -m "feat(output): implement GFM markdown formatter for PR comments

Produces collapsible findings sorted by severity with summary table,
decision banner, and persona attribution footer."
```

---

### Task 5: Pretty Terminal Formatter

**Files:**
- Modify: `internal/output/pretty.go`
- Test: `internal/output/pretty_test.go`

**Step 1: Write the failing tests**

Create `internal/output/pretty_test.go`:

```go
package output

import (
	"os"
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

func testPrettyInput() *AnalysisOutput {
	return &AnalysisOutput{
		Verdict: &store.Verdict{
			Decision: "review",
		},
		SARIFLog: &sarif.Log{
			Runs: []sarif.Run{{
				Tool: sarif.Tool{Driver: sarif.Driver{Name: "gavel"}},
				Results: []sarif.Result{
					{
						RuleID:  "SEC001",
						Level:   "error",
						Message: sarif.Message{Text: "Hardcoded secret"},
						Locations: []sarif.Location{{
							PhysicalLocation: sarif.PhysicalLocation{
								ArtifactLocation: sarif.ArtifactLocation{URI: "config/db.go"},
								Region:           sarif.Region{StartLine: 42, EndLine: 42},
							},
						}},
						Properties: map[string]interface{}{"gavel/confidence": 0.95},
					},
					{
						RuleID:  "ERR003",
						Level:   "warning",
						Message: sarif.Message{Text: "Error not checked"},
						Locations: []sarif.Location{{
							PhysicalLocation: sarif.PhysicalLocation{
								ArtifactLocation: sarif.ArtifactLocation{URI: "config/db.go"},
								Region:           sarif.Region{StartLine: 78, EndLine: 78},
							},
						}},
						Properties: map[string]interface{}{"gavel/confidence": 0.82},
					},
					{
						RuleID:  "STY001",
						Level:   "note",
						Message: sarif.Message{Text: "Function too long"},
						Locations: []sarif.Location{{
							PhysicalLocation: sarif.PhysicalLocation{
								ArtifactLocation: sarif.ArtifactLocation{URI: "handler.go"},
								Region:           sarif.Region{StartLine: 15, EndLine: 15},
							},
						}},
						Properties: map[string]interface{}{"gavel/confidence": 0.70},
					},
				},
				Properties: map[string]interface{}{
					"gavel/persona": "code-reviewer",
				},
			}},
		},
	}
}

func TestPrettyFormatter_ContainsDecision(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	f := &PrettyFormatter{}
	data, err := f.Format(testPrettyInput())
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "review") {
		t.Error("expected decision in output")
	}
}

func TestPrettyFormatter_GroupsByFile(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	f := &PrettyFormatter{}
	data, err := f.Format(testPrettyInput())
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "config/db.go") {
		t.Error("expected file path in output")
	}
	if !strings.Contains(out, "handler.go") {
		t.Error("expected second file path in output")
	}
}

func TestPrettyFormatter_ContainsRuleIDs(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	f := &PrettyFormatter{}
	data, err := f.Format(testPrettyInput())
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "SEC001") {
		t.Error("expected SEC001 rule ID")
	}
	if !strings.Contains(out, "ERR003") {
		t.Error("expected ERR003 rule ID")
	}
}

func TestPrettyFormatter_HasSummaryLine(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	f := &PrettyFormatter{}
	data, err := f.Format(testPrettyInput())
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "1 error") {
		t.Error("expected error count in summary")
	}
	if !strings.Contains(out, "1 warning") {
		t.Error("expected warning count in summary")
	}
}

func TestPrettyFormatter_NoFindings(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	input := &AnalysisOutput{
		Verdict: &store.Verdict{Decision: "merge"},
		SARIFLog: &sarif.Log{
			Runs: []sarif.Run{{
				Tool:       sarif.Tool{Driver: sarif.Driver{Name: "gavel"}},
				Results:    []sarif.Result{},
				Properties: map[string]interface{}{"gavel/persona": "code-reviewer"},
			}},
		},
	}

	f := &PrettyFormatter{}
	data, err := f.Format(input)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "No findings") {
		t.Error("expected 'No findings' message")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/output/ -run TestPretty -v`
Expected: FAIL — stub returns nil

**Step 3: Implement the Pretty formatter**

Update `internal/output/pretty.go`:

```go
package output

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// PrettyFormatter outputs colored terminal text grouped by file.
type PrettyFormatter struct{}

func (f *PrettyFormatter) Format(result *AnalysisOutput) ([]byte, error) {
	noColor := os.Getenv("NO_COLOR") != ""

	var results []sarif.Result
	persona := "code-reviewer"
	if result.SARIFLog != nil && len(result.SARIFLog.Runs) > 0 {
		results = result.SARIFLog.Runs[0].Results
		if p, ok := result.SARIFLog.Runs[0].Properties["gavel/persona"].(string); ok {
			persona = p
		}
	}

	decision := "unknown"
	if result.Verdict != nil {
		decision = result.Verdict.Decision
	}

	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true)
	fileStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	if noColor {
		headerStyle = lipgloss.NewStyle()
		fileStyle = lipgloss.NewStyle()
		errorStyle = lipgloss.NewStyle()
		warningStyle = lipgloss.NewStyle()
		noteStyle = lipgloss.NewStyle()
		dimStyle = lipgloss.NewStyle()
	}

	var b strings.Builder

	// Header
	separator := "──────────────────────────────────"
	b.WriteString("\n")
	b.WriteString("  " + headerStyle.Render("Gavel Analysis") + "\n")
	b.WriteString("  " + dimStyle.Render(separator) + "\n")

	// Count files
	files := make(map[string]bool)
	for _, r := range results {
		if len(r.Locations) > 0 {
			files[r.Locations[0].PhysicalLocation.ArtifactLocation.URI] = true
		}
	}
	fmt.Fprintf(&b, "  Decision: %s  |  %d findings  |  %d files\n", decision, len(results), len(files))
	fmt.Fprintf(&b, "  Persona: %s\n\n", persona)

	if len(results) == 0 {
		b.WriteString("  No findings detected.\n\n")
	} else {
		// Group by file
		fileResults := make(map[string][]sarif.Result)
		var fileOrder []string
		for _, r := range results {
			uri := resultURI(r)
			if _, seen := fileResults[uri]; !seen {
				fileOrder = append(fileOrder, uri)
			}
			fileResults[uri] = append(fileResults[uri], r)
		}
		sort.Strings(fileOrder)

		for _, file := range fileOrder {
			b.WriteString("  " + fileStyle.Render(file) + "\n")

			// Sort by line number within file
			fr := fileResults[file]
			sort.Slice(fr, func(i, j int) bool {
				li := 0
				lj := 0
				if len(fr[i].Locations) > 0 {
					li = fr[i].Locations[0].PhysicalLocation.Region.StartLine
				}
				if len(fr[j].Locations) > 0 {
					lj = fr[j].Locations[0].PhysicalLocation.Region.StartLine
				}
				return li < lj
			})

			for _, r := range fr {
				line := 0
				if len(r.Locations) > 0 {
					line = r.Locations[0].PhysicalLocation.Region.StartLine
				}

				var levelStr string
				switch r.Level {
				case "error":
					levelStr = errorStyle.Render("error  ")
				case "warning":
					levelStr = warningStyle.Render("warning")
				case "note":
					levelStr = noteStyle.Render("note   ")
				default:
					levelStr = r.Level
				}

				conf := ""
				if c, ok := r.Properties["gavel/confidence"].(float64); ok {
					conf = dimStyle.Render(fmt.Sprintf("(%.2f)", c))
				}

				fmt.Fprintf(&b, "    %-6s %s  %-7s  %s  %s\n",
					fmt.Sprintf("%d:1", line), levelStr, r.RuleID, r.Message.Text, conf)
			}
			b.WriteString("\n")
		}
	}

	// Summary
	b.WriteString("  " + dimStyle.Render(separator) + "\n")

	errorCount := 0
	warningCount := 0
	noteCount := 0
	for _, r := range results {
		switch r.Level {
		case "error":
			errorCount++
		case "warning":
			warningCount++
		case "note":
			noteCount++
		}
	}

	var parts []string
	if errorCount > 0 {
		word := "errors"
		if errorCount == 1 {
			word = "error"
		}
		parts = append(parts, errorStyle.Render(fmt.Sprintf("%d %s", errorCount, word)))
	}
	if warningCount > 0 {
		word := "warnings"
		if warningCount == 1 {
			word = "warning"
		}
		parts = append(parts, warningStyle.Render(fmt.Sprintf("%d %s", warningCount, word)))
	}
	if noteCount > 0 {
		word := "notes"
		if noteCount == 1 {
			word = "note"
		}
		parts = append(parts, noteStyle.Render(fmt.Sprintf("%d %s", noteCount, word)))
	}

	if len(parts) > 0 {
		b.WriteString("  " + strings.Join(parts, ", ") + "\n")
	} else {
		b.WriteString("  No findings\n")
	}
	b.WriteString("\n")

	return []byte(b.String()), nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/output/ -run TestPretty -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/output/pretty.go internal/output/pretty_test.go
git commit -m "feat(output): implement pretty terminal formatter with color and grouping

Groups findings by file, sorts by line, color-codes severity levels.
Respects NO_COLOR env var. Uses lipgloss for styling."
```

---

### Task 6: Structured Logging Setup

**Files:**
- Modify: `cmd/gavel/main.go`
- Test: `cmd/gavel/main_test.go` (if feasible) or manual verification

**Step 1: Write the test**

This is infrastructure — test via the integration in Task 7. For now, write a simple unit test.

Create `internal/output/logging_test.go`:

```go
package output

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestSetupLogger_DefaultLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := SetupLogger(false, false, false, &buf)

	logger.Info("should not appear")
	logger.Warn("should appear")

	out := buf.String()
	if len(out) == 0 {
		t.Error("expected warn message in output")
	}
	if !contains(out, "should appear") {
		t.Error("expected warn message text")
	}
}

func TestSetupLogger_Quiet(t *testing.T) {
	var buf bytes.Buffer
	logger := SetupLogger(true, false, false, &buf)

	logger.Warn("should not appear")
	logger.Error("should not appear")

	if buf.Len() > 0 {
		t.Error("quiet mode should suppress all output")
	}
}

func TestSetupLogger_Verbose(t *testing.T) {
	var buf bytes.Buffer
	logger := SetupLogger(false, true, false, &buf)

	logger.Info("should appear")

	if !contains(buf.String(), "should appear") {
		t.Error("verbose mode should show info messages")
	}
}

func TestSetupLogger_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := SetupLogger(false, false, true, &buf)

	logger.Debug("debug msg")

	if !contains(buf.String(), "debug msg") {
		t.Error("debug mode should show debug messages")
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/output/ -run TestSetupLogger -v`
Expected: FAIL — `SetupLogger` doesn't exist

**Step 3: Implement SetupLogger**

Create `internal/output/logging.go`:

```go
package output

import (
	"io"
	"log/slog"
	"math"
)

// SetupLogger creates a slog.Logger configured for the given verbosity.
// Output is written to w (typically os.Stderr).
// quiet suppresses all output, verbose enables info level, debug enables debug level.
func SetupLogger(quiet, verbose, debug bool, w io.Writer) *slog.Logger {
	var level slog.Level
	switch {
	case quiet:
		// Set level higher than any real level to suppress all output
		level = slog.Level(math.MaxInt)
	case debug:
		level = slog.LevelDebug
	case verbose:
		level = slog.LevelInfo
	default:
		level = slog.LevelWarn
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(handler)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/output/ -run TestSetupLogger -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/output/logging.go internal/output/logging_test.go
git commit -m "feat(output): add structured logging setup with slog"
```

---

### Task 7: Wire Into CLI (analyze command + root flags)

**Files:**
- Modify: `cmd/gavel/main.go` (add logging flags + setup)
- Modify: `cmd/gavel/analyze.go` (add `--format` flag, use formatter, use slog)

**Step 1: Add `go-isatty` as direct dependency**

Run: `go get github.com/mattn/go-isatty`

(It's already an indirect dep, this just promotes it to direct.)

**Step 2: Modify `cmd/gavel/main.go`**

Add persistent logging flags to root command and set up the default slog logger:

```go
// Add after existing init() body, within the init() function:
rootCmd.PersistentFlags().BoolP("quiet", "q", false, "Suppress all log output")
rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose (info-level) logging")
rootCmd.PersistentFlags().Bool("debug", false, "Enable debug-level logging")
```

Add a `PersistentPreRunE` to `rootCmd` that sets up slog:

```go
rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
    quiet, _ := cmd.Flags().GetBool("quiet")
    verbose, _ := cmd.Flags().GetBool("verbose")
    debug, _ := cmd.Flags().GetBool("debug")
    logger := output.SetupLogger(quiet, verbose, debug, os.Stderr)
    slog.SetDefault(logger)
    return nil
}
```

Add imports: `"log/slog"`, `"os"`, `"github.com/chris-regnier/gavel/internal/output"`

**Step 3: Modify `cmd/gavel/analyze.go`**

Add `--format` flag:

```go
// In var block, add:
flagFormat string

// In init(), add:
analyzeCmd.Flags().StringVarP(&flagFormat, "format", "f", "", "Output format: json, sarif, markdown, pretty (default: auto-detect)")
```

Replace the output section at the end of `runAnalyze` (lines 187-189):

```go
// Replace:
//   out, _ := json.MarshalIndent(verdict, "", "  ")
//   fmt.Println(string(out))
// With:

format := output.ResolveFormat(flagFormat, isatty.IsTerminal(os.Stdout.Fd()))
formatter, err := output.NewFormatter(format)
if err != nil {
    return err
}
data, err := formatter.Format(&output.AnalysisOutput{
    Verdict:  verdict,
    SARIFLog: sarifLog,
})
if err != nil {
    return fmt.Errorf("formatting output: %w", err)
}
os.Stdout.Write(data)
```

Add imports: `"github.com/mattn/go-isatty"`, `"github.com/chris-regnier/gavel/internal/output"`

Remove now-unused import if `encoding/json` is only used for the old output line (check — it's also used by `uploadResultsToCache` so it stays).

Replace the `log.Printf` on line 183:

```go
// Replace:
//   log.Printf("Warning: failed to upload results to remote cache: %v", err)
// With:
slog.Warn("cache upload failed", "err", err)
```

Add import: `"log/slog"`

**Step 4: Run existing tests**

Run: `go test ./... -count=1 -timeout 60s 2>&1 | head -100`
Expected: All existing tests PASS (no behavioral change for tests that don't use --format)

**Step 5: Run a manual smoke test**

Run: `go build -o gavel ./cmd/gavel && echo 'package main' > /tmp/test.go && ./gavel analyze --files /tmp/test.go --format json 2>/dev/null; echo "exit: $?"`

(This will fail if no provider is configured, but it validates the flag parsing and format resolution work.)

**Step 6: Commit**

```bash
git add cmd/gavel/main.go cmd/gavel/analyze.go go.mod go.sum
git commit -m "feat(cli): wire --format flag and structured logging into analyze command

Adds --format (json/sarif/markdown/pretty) with TTY auto-detection.
Adds --quiet, --verbose, --debug flags for log level control.
Replaces log.Printf with slog in analyze command."
```

---

### Task 8: Migrate Remaining Log Calls

**Files:**
- Modify: `internal/cache/multitier.go`
- Modify: `cmd/gavel/create.go`
- Modify: `internal/lsp/server.go`

**Step 1: Migrate `internal/cache/multitier.go`**

Replace all `log.Printf` calls with `slog` equivalents:

- `log.Printf("Failed to warm local cache: %v", putErr)` → `slog.Warn("failed to warm local cache", "err", putErr)`
- `log.Printf("Failed to write to remote cache: %v", err)` → `slog.Warn("failed to write to remote cache", "err", err)`
- `log.Printf("Failed to delete from remote cache: %v", err)` → `slog.Warn("failed to delete from remote cache", "err", err)`

Update import: replace `"log"` with `"log/slog"`.

**Step 2: Migrate `cmd/gavel/create.go`**

Replace diagnostic `fmt.Fprintln(os.Stderr, ...)` calls with `slog.Info`:

- `fmt.Fprintln(os.Stderr, "Generating policy...")` → `slog.Info("generating policy")`
- `fmt.Fprintln(os.Stderr, "Generating rule...")` → `slog.Info("generating rule")`
- `fmt.Fprintln(os.Stderr, "Generating persona...")` → `slog.Info("generating persona")`
- `fmt.Fprintln(os.Stderr, "Generating configuration...")` → `slog.Info("generating configuration")`
- `fmt.Fprintf(os.Stderr, "Created: %s\n", outputPath)` → `slog.Info("created file", "path", outputPath)`

Keep `fmt.Println(content)` on line 310 — that's data output.

Add import: `"log/slog"`. Remove `"os"` if no longer needed (check — it's used for file operations so it stays).

**Step 3: Migrate `internal/lsp/server.go`**

Replace `log.Printf` calls with `slog`:

- `log.Printf("Error handling message: %v", err)` → `slog.Error("error handling message", "err", err)`
- `log.Printf("Unhandled method: %s", msg.Method)` → `slog.Debug("unhandled method", "method", msg.Method)`
- `log.Printf("Analysis error for %s: %v", uri, err)` → `slog.Error("analysis error", "uri", uri, "err", err)`
- `log.Printf("Failed to publish diagnostics for %s: %v", uri, err)` → `slog.Error("failed to publish diagnostics", "uri", uri, "err", err)`

Update import: replace `"log"` with `"log/slog"`.

**Step 4: Run tests**

Run: `go test ./... -count=1 -timeout 60s`
Expected: PASS — slog calls are behavioral equivalents

**Step 5: Commit**

```bash
git add internal/cache/multitier.go cmd/gavel/create.go internal/lsp/server.go
git commit -m "refactor: migrate remaining log.Printf/fmt.Fprintln to slog

Migrates cache, create command, and LSP server to structured logging.
All diagnostic output now goes through slog to stderr."
```

---

### Task 9: Final Integration Test & Cleanup

**Files:**
- Modify: `internal/output/formatter_test.go` (add integration-style test)
- Verify: all existing tests pass

**Step 1: Add an integration-style round-trip test**

Add to `internal/output/formatter_test.go`:

```go
func TestAllFormatters_RoundTrip(t *testing.T) {
	input := &AnalysisOutput{
		Verdict: &store.Verdict{
			Decision: "review",
			Reason:   "test",
			RelevantFindings: []sarif.Result{
				{RuleID: "T001", Level: "warning", Message: sarif.Message{Text: "test finding"}},
			},
		},
		SARIFLog: &sarif.Log{
			Schema:  sarif.SchemaURI,
			Version: sarif.Version,
			Runs: []sarif.Run{{
				Tool: sarif.Tool{Driver: sarif.Driver{Name: "gavel", Version: "test"}},
				Results: []sarif.Result{
					{
						RuleID:  "T001",
						Level:   "warning",
						Message: sarif.Message{Text: "test finding"},
						Locations: []sarif.Location{{
							PhysicalLocation: sarif.PhysicalLocation{
								ArtifactLocation: sarif.ArtifactLocation{URI: "test.go"},
								Region:           sarif.Region{StartLine: 1, EndLine: 1},
							},
						}},
						Properties: map[string]interface{}{
							"gavel/confidence": 0.8,
							"gavel/tier":       "instant",
						},
					},
				},
				Properties: map[string]interface{}{
					"gavel/persona": "code-reviewer",
				},
			}},
		},
	}

	for _, format := range []string{"json", "sarif", "markdown", "pretty"} {
		t.Run(format, func(t *testing.T) {
			os.Setenv("NO_COLOR", "1")
			defer os.Unsetenv("NO_COLOR")

			f, err := NewFormatter(format)
			if err != nil {
				t.Fatal(err)
			}
			data, err := f.Format(input)
			if err != nil {
				t.Fatalf("Format() error: %v", err)
			}
			if len(data) == 0 {
				t.Error("Format() returned empty output")
			}
		})
	}
}
```

**Step 2: Run full test suite**

Run: `go test ./... -count=1 -timeout 120s -v 2>&1 | tail -30`
Expected: ALL PASS

**Step 3: Run `go vet`**

Run: `go vet ./...`
Expected: No issues

**Step 4: Commit**

```bash
git add internal/output/formatter_test.go
git commit -m "test(output): add integration round-trip test for all formatters"
```

---

## Summary

| Task | What | Files |
|------|------|-------|
| 1 | Formatter interface + factory + TTY detection | `internal/output/formatter.go`, stubs for all formatters |
| 2 | JSON formatter | `internal/output/json.go` |
| 3 | SARIF formatter (GitHub-compliant) | `internal/output/sarif.go`, `internal/sarif/sarif.go` |
| 4 | Markdown formatter (GFM) | `internal/output/markdown.go` |
| 5 | Pretty terminal formatter | `internal/output/pretty.go` |
| 6 | Structured logging setup | `internal/output/logging.go` |
| 7 | Wire into CLI | `cmd/gavel/main.go`, `cmd/gavel/analyze.go` |
| 8 | Migrate remaining log calls | `internal/cache/multitier.go`, `cmd/gavel/create.go`, `internal/lsp/server.go` |
| 9 | Integration test + cleanup | `internal/output/formatter_test.go` |

Tasks 1-6 are independent and can be parallelized. Task 7 depends on 1-6. Task 8 depends on 6. Task 9 depends on all.
