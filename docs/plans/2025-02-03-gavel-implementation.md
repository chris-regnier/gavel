# Gavel Initial Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a working CLI that analyzes code against declarative policies using an LLM (via BAML), produces SARIF output, and evaluates it with Rego to produce verdicts.

**Architecture:** A pipeline of five components — Input Handler, BAML Analyzer, SARIF Assembler, Rego Evaluator, Storage Backend — wired together by a CLI entry point. Each component has a clean interface and is independently testable.

**Tech Stack:** Go 1.25, BAML (boundaryml), OPA/Rego (`github.com/open-policy-agent/opa/v1/rego`), SARIF 2.1.0 (custom types), Cobra (CLI)

---

### Task 1: Project Scaffolding

Set up the Go module, directory structure, BAML project, and Taskfile commands.

**Files:**
- Modify: `go.mod`
- Create: `cmd/gavel/main.go`
- Create: `internal/config/config.go`
- Create: `internal/input/handler.go`
- Create: `internal/analyzer/analyzer.go`
- Create: `internal/sarif/sarif.go`
- Create: `internal/evaluator/evaluator.go`
- Create: `internal/store/store.go`
- Create: `baml_src/generators.baml`
- Modify: `Taskfile.yml`

**Step 1: Create directory structure**

```bash
cd /Users/chris-regnier/code/gavel/.worktrees/feature-initial-implementation
mkdir -p cmd/gavel internal/{config,input,analyzer,sarif,evaluator,store}
```

**Step 2: Create minimal main.go**

```go
// cmd/gavel/main.go
package main

import "fmt"

func main() {
	fmt.Println("gavel")
}
```

**Step 3: Initialize BAML**

```bash
go install github.com/boundaryml/baml/baml-cli@latest
baml-cli init
```

Review the generated `baml_src/` files. Update the generator in `baml_src/generators.baml` to target Go:

```baml
generator target {
    output_type "go"
    output_dir "../"
    version "0.205.0"
    client_package_name "github.com/chris-regnier/gavel"
    on_generate "gofmt -w . && goimports -w . && go mod tidy"
}
```

**Step 4: Update Taskfile.yml**

```yaml
version: '3'

vars:
  BINARY: gavel

tasks:
  default:
    cmds:
      - task: build

  build:
    cmds:
      - go build -o {{.BINARY}} ./cmd/gavel

  test:
    cmds:
      - go test ./... -v

  generate:
    cmds:
      - baml-cli generate

  lint:
    cmds:
      - go vet ./...
```

**Step 5: Verify build**

Run: `task build && ./gavel`
Expected: prints `gavel`

**Step 6: Commit**

```bash
git add -A
git commit -m "feat: scaffold project structure with BAML and Taskfile"
```

---

### Task 2: Configuration System

Build the tiered config loader that merges system → machine → project → human overrides.

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/config/defaults.go`

**Step 1: Write the failing test — policy merging**

```go
// internal/config/config_test.go
package config

import (
	"testing"
)

func TestMergePolicies_HigherTierOverrides(t *testing.T) {
	system := &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Description: "Handle errors",
				Severity:    "warning",
				Instruction: "Check error handling",
				Enabled:     true,
			},
		},
	}

	project := &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Severity: "error", // override severity
			},
		},
	}

	merged := MergeConfigs(system, project)

	pol := merged.Policies["error-handling"]
	if pol.Severity != "error" {
		t.Errorf("expected severity 'error', got %q", pol.Severity)
	}
	if pol.Description != "Handle errors" {
		t.Errorf("expected description preserved, got %q", pol.Description)
	}
	if !pol.Enabled {
		t.Error("expected enabled to remain true")
	}
}

func TestMergePolicies_HigherTierAddsNew(t *testing.T) {
	system := &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Description: "Handle errors",
				Severity:    "warning",
				Instruction: "Check error handling",
				Enabled:     true,
			},
		},
	}

	project := &Config{
		Policies: map[string]Policy{
			"function-length": {
				Description: "Keep functions short",
				Severity:    "note",
				Instruction: "Flag long functions",
				Enabled:     true,
			},
		},
	}

	merged := MergeConfigs(system, project)

	if len(merged.Policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(merged.Policies))
	}
}

func TestMergePolicies_DisablePolicy(t *testing.T) {
	system := &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Description: "Handle errors",
				Severity:    "warning",
				Instruction: "Check error handling",
				Enabled:     true,
			},
		},
	}

	project := &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Enabled: false,
			},
		},
	}

	merged := MergeConfigs(system, project)

	if merged.Policies["error-handling"].Enabled {
		t.Error("expected policy to be disabled")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test`
Expected: FAIL — types and functions not defined

**Step 3: Write minimal implementation**

```go
// internal/config/config.go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Policy defines a single analysis policy.
type Policy struct {
	Description string `yaml:"description"`
	Severity    string `yaml:"severity"`
	Instruction string `yaml:"instruction"`
	Enabled     bool   `yaml:"enabled"`
}

// Config holds the full gavel configuration.
type Config struct {
	Policies map[string]Policy `yaml:"policies"`
}

// MergeConfigs merges configs in order of increasing precedence.
// Later configs override earlier ones. Fields in a policy override
// only if they are non-zero values, except Enabled which always applies.
func MergeConfigs(configs ...*Config) *Config {
	merged := &Config{
		Policies: make(map[string]Policy),
	}

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		for name, pol := range cfg.Policies {
			existing, ok := merged.Policies[name]
			if !ok {
				merged.Policies[name] = pol
				continue
			}
			if pol.Description != "" {
				existing.Description = pol.Description
			}
			if pol.Severity != "" {
				existing.Severity = pol.Severity
			}
			if pol.Instruction != "" {
				existing.Instruction = pol.Instruction
			}
			// Enabled always takes effect from higher tier
			existing.Enabled = pol.Enabled
			merged.Policies[name] = existing
		}
	}

	return merged
}

// LoadFromFile reads a config from a YAML file.
// Returns nil config (not error) if the file does not exist.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
```

```go
// internal/config/defaults.go
package config

// SystemDefaults returns the built-in default policies.
func SystemDefaults() *Config {
	return &Config{
		Policies: map[string]Policy{
			"error-handling": {
				Description: "Public functions must handle errors explicitly",
				Severity:    "warning",
				Instruction: "Check that all public functions either return an error or handle errors from called functions. Flag functions that silently discard errors.",
				Enabled:     true,
			},
			"function-length": {
				Description: "Functions should not exceed a reasonable length",
				Severity:    "note",
				Instruction: "Flag functions longer than 50 lines. Consider whether the function could be decomposed.",
				Enabled:     true,
			},
		},
	}
}
```

**Step 4: Run test to verify it passes**

Run: `task test`
Expected: PASS

**Step 5: Write the failing test — LoadFromFile**

```go
// Add to internal/config/config_test.go

func TestLoadFromFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/policies.yaml"
	data := []byte(`policies:
  test-policy:
    description: "Test"
    severity: "warning"
    instruction: "Do the thing"
    enabled: true
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Policies) != 1 {
		t.Errorf("expected 1 policy, got %d", len(cfg.Policies))
	}
}

func TestLoadFromFile_Missing(t *testing.T) {
	cfg, err := LoadFromFile("/nonexistent/path.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Error("expected nil config for missing file")
	}
}
```

**Step 6: Run test to verify it passes**

Run: `task test`
Expected: PASS (implementation already handles this)

**Step 7: Write the failing test — full tier loading**

```go
// Add to internal/config/config_test.go

func TestLoadTiered(t *testing.T) {
	dir := t.TempDir()

	machineConf := dir + "/machine.yaml"
	os.WriteFile(machineConf, []byte(`policies:
  error-handling:
    severity: "error"
`), 0644)

	projectConf := dir + "/project.yaml"
	os.WriteFile(projectConf, []byte(`policies:
  custom-rule:
    description: "Custom"
    severity: "warning"
    instruction: "Check custom thing"
    enabled: true
`), 0644)

	cfg, err := LoadTiered(machineConf, projectConf)
	if err != nil {
		t.Fatal(err)
	}

	// System default + machine override
	if cfg.Policies["error-handling"].Severity != "error" {
		t.Errorf("expected machine override severity 'error', got %q", cfg.Policies["error-handling"].Severity)
	}

	// Project addition
	if _, ok := cfg.Policies["custom-rule"]; !ok {
		t.Error("expected project policy 'custom-rule'")
	}

	// System default preserved
	if _, ok := cfg.Policies["function-length"]; !ok {
		t.Error("expected system default 'function-length'")
	}
}
```

**Step 8: Implement LoadTiered**

Add to `internal/config/config.go`:

```go
// LoadTiered loads and merges config from all tiers:
// system defaults → machine → project.
// Human overrides (CLI flags) are applied separately.
func LoadTiered(machinePath, projectPath string) (*Config, error) {
	system := SystemDefaults()

	machine, err := LoadFromFile(machinePath)
	if err != nil {
		return nil, fmt.Errorf("loading machine config: %w", err)
	}

	project, err := LoadFromFile(projectPath)
	if err != nil {
		return nil, fmt.Errorf("loading project config: %w", err)
	}

	return MergeConfigs(system, machine, project), nil
}
```

**Step 9: Run tests**

Run: `task test`
Expected: PASS

**Step 10: Commit**

```bash
git add internal/config/
git commit -m "feat: add tiered configuration system with policy merging"
```

---

### Task 3: SARIF Types

Define Go types for SARIF 2.1.0 with gavel extensions.

**Files:**
- Create: `internal/sarif/sarif.go`
- Create: `internal/sarif/sarif_test.go`

**Step 1: Write the failing test — SARIF serialization**

```go
// internal/sarif/sarif_test.go
package sarif

import (
	"encoding/json"
	"testing"
)

func TestSarifLog_MarshalJSON(t *testing.T) {
	log := NewLog("gavel", "0.1.0")
	log.Runs[0].Results = append(log.Runs[0].Results, Result{
		RuleID:  "error-handling",
		Level:   "warning",
		Message: Message{Text: "Function Foo does not handle errors"},
		Locations: []Location{{
			PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "pkg/bar/bar.go"},
				Region:           Region{StartLine: 10, EndLine: 15},
			},
		}},
		Properties: map[string]interface{}{
			"gavel/recommendation": "Add error return",
			"gavel/explanation":    "Function calls DB but ignores error",
			"gavel/confidence":     0.9,
		},
	})

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	// Verify it round-trips
	var parsed Log
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if len(parsed.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(parsed.Runs))
	}
	if len(parsed.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(parsed.Runs[0].Results))
	}
	r := parsed.Runs[0].Results[0]
	if r.RuleID != "error-handling" {
		t.Errorf("expected ruleId 'error-handling', got %q", r.RuleID)
	}
	if r.Properties["gavel/recommendation"] != "Add error return" {
		t.Errorf("expected recommendation preserved")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test`
Expected: FAIL — types not defined

**Step 3: Implement SARIF types**

```go
// internal/sarif/sarif.go
package sarif

const SchemaURI = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json"
const Version = "2.1.0"

// Log is the top-level SARIF object.
type Log struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

// Run represents a single analysis run.
type Run struct {
	Tool       Tool                   `json:"tool"`
	Results    []Result               `json:"results"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// Tool identifies the analysis tool.
type Tool struct {
	Driver Driver `json:"driver"`
}

// Driver is the primary tool component.
type Driver struct {
	Name           string           `json:"name"`
	Version        string           `json:"version,omitempty"`
	InformationURI string           `json:"informationUri,omitempty"`
	Rules          []ReportingDescriptor `json:"rules,omitempty"`
}

// ReportingDescriptor describes a rule.
type ReportingDescriptor struct {
	ID               string  `json:"id"`
	ShortDescription Message `json:"shortDescription,omitempty"`
	DefaultConfig    *ReportingConfiguration `json:"defaultConfiguration,omitempty"`
}

// ReportingConfiguration holds default severity for a rule.
type ReportingConfiguration struct {
	Level string `json:"level,omitempty"`
}

// Result is a single finding.
type Result struct {
	RuleID     string                 `json:"ruleId"`
	Level      string                 `json:"level"`
	Message    Message                `json:"message"`
	Locations  []Location             `json:"locations,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// Message holds text content.
type Message struct {
	Text string `json:"text"`
}

// Location identifies where a result was found.
type Location struct {
	PhysicalLocation PhysicalLocation `json:"physicalLocation"`
}

// PhysicalLocation points to a file and region.
type PhysicalLocation struct {
	ArtifactLocation ArtifactLocation `json:"artifactLocation"`
	Region           Region           `json:"region,omitempty"`
}

// ArtifactLocation identifies a file.
type ArtifactLocation struct {
	URI string `json:"uri"`
}

// Region identifies a range within a file.
type Region struct {
	StartLine int `json:"startLine,omitempty"`
	EndLine   int `json:"endLine,omitempty"`
}

// NewLog creates a new SARIF log with a single empty run.
func NewLog(toolName, toolVersion string) *Log {
	return &Log{
		Schema:  SchemaURI,
		Version: Version,
		Runs: []Run{{
			Tool: Tool{
				Driver: Driver{
					Name:    toolName,
					Version: toolVersion,
				},
			},
			Results: []Result{},
		}},
	}
}
```

**Step 4: Run tests**

Run: `task test`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/sarif/
git commit -m "feat: add SARIF 2.1.0 types with gavel extensions"
```

---

### Task 4: Storage Backend Interface + Filesystem Implementation

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/filestore.go`
- Create: `internal/store/filestore_test.go`

**Step 1: Write the failing test**

```go
// internal/store/filestore_test.go
package store

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestFileStore_WriteAndReadSARIF(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	ctx := context.Background()

	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = append(log.Runs[0].Results, sarif.Result{
		RuleID:  "test-rule",
		Level:   "warning",
		Message: sarif.Message{Text: "test finding"},
	})

	id, err := fs.WriteSARIF(ctx, log)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	loaded, err := fs.ReadSARIF(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(loaded.Runs[0].Results))
	}
	if loaded.Runs[0].Results[0].RuleID != "test-rule" {
		t.Errorf("expected ruleId 'test-rule', got %q", loaded.Runs[0].Results[0].RuleID)
	}
}

func TestFileStore_WriteAndReadVerdict(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	ctx := context.Background()

	log := sarif.NewLog("gavel", "0.1.0")
	id, err := fs.WriteSARIF(ctx, log)
	if err != nil {
		t.Fatal(err)
	}

	verdict := &Verdict{
		Decision: "merge",
		Reason:   "No issues found",
	}

	if err := fs.WriteVerdict(ctx, id, verdict); err != nil {
		t.Fatal(err)
	}

	loaded, err := fs.ReadVerdict(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Decision != "merge" {
		t.Errorf("expected decision 'merge', got %q", loaded.Decision)
	}
}

func TestFileStore_List(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	ctx := context.Background()

	log := sarif.NewLog("gavel", "0.1.0")
	fs.WriteSARIF(ctx, log)
	fs.WriteSARIF(ctx, log)

	ids, err := fs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 results, got %d", len(ids))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test`
Expected: FAIL

**Step 3: Implement the interface and file store**

```go
// internal/store/store.go
package store

import (
	"context"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// Verdict represents the decision from Rego evaluation.
type Verdict struct {
	Decision         string                 `json:"decision"`
	Reason           string                 `json:"reason"`
	RelevantFindings []sarif.Result         `json:"relevant_findings,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// Store persists SARIF documents and verdicts.
type Store interface {
	WriteSARIF(ctx context.Context, doc *sarif.Log) (string, error)
	WriteVerdict(ctx context.Context, sarifID string, verdict *Verdict) error
	ReadSARIF(ctx context.Context, id string) (*sarif.Log, error)
	ReadVerdict(ctx context.Context, sarifID string) (*Verdict, error)
	List(ctx context.Context) ([]string, error)
}
```

```go
// internal/store/filestore.go
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// FileStore implements Store using the filesystem.
type FileStore struct {
	dir string
}

// NewFileStore creates a new file-based store at the given directory.
func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

func (s *FileStore) generateID() string {
	b := make([]byte, 3)
	rand.Read(b)
	ts := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	return fmt.Sprintf("%s-%s", ts, hex.EncodeToString(b))
}

func (s *FileStore) resultDir(id string) string {
	return filepath.Join(s.dir, id)
}

func (s *FileStore) WriteSARIF(ctx context.Context, doc *sarif.Log) (string, error) {
	id := s.generateID()
	dir := s.resultDir(id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(filepath.Join(dir, "sarif.json"), data, 0644); err != nil {
		return "", err
	}

	return id, nil
}

func (s *FileStore) WriteVerdict(ctx context.Context, sarifID string, verdict *Verdict) error {
	dir := s.resultDir(sarifID)
	data, err := json.MarshalIndent(verdict, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "verdict.json"), data, 0644)
}

func (s *FileStore) ReadSARIF(ctx context.Context, id string) (*sarif.Log, error) {
	data, err := os.ReadFile(filepath.Join(s.resultDir(id), "sarif.json"))
	if err != nil {
		return nil, err
	}
	var log sarif.Log
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}
	return &log, nil
}

func (s *FileStore) ReadVerdict(ctx context.Context, sarifID string) (*Verdict, error) {
	data, err := os.ReadFile(filepath.Join(s.resultDir(sarifID), "verdict.json"))
	if err != nil {
		return nil, err
	}
	var v Verdict
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *FileStore) List(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	return ids, nil
}
```

**Step 4: Run tests**

Run: `task test`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/store/
git commit -m "feat: add storage backend interface with filesystem implementation"
```

---

### Task 5: Input Handler

Reads code artifacts from various sources and prepares them for analysis.

**Files:**
- Create: `internal/input/handler.go`
- Create: `internal/input/handler_test.go`

**Step 1: Write the failing test**

```go
// internal/input/handler_test.go
package input

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHandler_ReadFiles(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	os.MkdirAll(filepath.Join(dir, "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "pkg", "foo.go"), []byte("package pkg\n\nfunc Foo() {}\n"), 0644)

	h := NewHandler()
	artifacts, err := h.ReadFiles([]string{
		filepath.Join(dir, "main.go"),
		filepath.Join(dir, "pkg", "foo.go"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}
	if artifacts[0].Path != filepath.Join(dir, "main.go") {
		t.Errorf("unexpected path: %s", artifacts[0].Path)
	}
	if artifacts[0].Content != "package main\n\nfunc main() {}\n" {
		t.Errorf("unexpected content: %q", artifacts[0].Content)
	}
}

func TestHandler_ReadDiff(t *testing.T) {
	diff := `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,5 @@
 package main

-func main() {}
+func main() {
+	fmt.Println("hello")
+}
`

	h := NewHandler()
	artifacts, err := h.ReadDiff(diff)
	if err != nil {
		t.Fatal(err)
	}

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].Path != "main.go" {
		t.Errorf("expected path 'main.go', got %q", artifacts[0].Path)
	}
	if artifacts[0].Kind != KindDiff {
		t.Errorf("expected kind Diff, got %v", artifacts[0].Kind)
	}
}

func TestHandler_ReadDirectory(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b\n"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Hi\n"), 0644)

	h := NewHandler()
	artifacts, err := h.ReadDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should read all files (no filtering at this layer)
	if len(artifacts) < 2 {
		t.Errorf("expected at least 2 artifacts, got %d", len(artifacts))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test`
Expected: FAIL

**Step 3: Implement input handler**

```go
// internal/input/handler.go
package input

import (
	"os"
	"path/filepath"
	"strings"
)

// Kind describes the type of input artifact.
type Kind int

const (
	KindFile Kind = iota
	KindDiff
)

// Artifact represents a code artifact prepared for analysis.
type Artifact struct {
	Path    string
	Content string
	Kind    Kind
}

// Handler reads code artifacts from various sources.
type Handler struct{}

// NewHandler creates a new input handler.
func NewHandler() *Handler {
	return &Handler{}
}

// ReadFiles reads specific files and returns them as artifacts.
func (h *Handler) ReadFiles(paths []string) ([]Artifact, error) {
	var artifacts []Artifact
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, Artifact{
			Path:    p,
			Content: string(data),
			Kind:    KindFile,
		})
	}
	return artifacts, nil
}

// ReadDiff parses a unified diff and returns artifacts for each changed file.
func (h *Handler) ReadDiff(diff string) ([]Artifact, error) {
	var artifacts []Artifact
	var currentPath string
	var currentLines []string

	flush := func() {
		if currentPath != "" {
			artifacts = append(artifacts, Artifact{
				Path:    currentPath,
				Content: strings.Join(currentLines, "\n"),
				Kind:    KindDiff,
			})
		}
	}

	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			flush()
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				// Extract path from "b/path"
				currentPath = strings.TrimPrefix(parts[len(parts)-1], "b/")
			}
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	flush()

	return artifacts, nil
}

// ReadDirectory walks a directory and returns all files as artifacts.
func (h *Handler) ReadDirectory(dir string) ([]Artifact, error) {
	var artifacts []Artifact
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, Artifact{
			Path:    path,
			Content: string(data),
			Kind:    KindFile,
		})
		return nil
	})
	return artifacts, err
}
```

**Step 4: Run tests**

Run: `task test`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/input/
git commit -m "feat: add input handler for files, diffs, and directories"
```

---

### Task 6: BAML Analyzer

Define the BAML functions for code analysis and the Go wrapper that invokes them.

**Files:**
- Create: `baml_src/analyze.baml`
- Create: `internal/analyzer/analyzer.go`
- Create: `internal/analyzer/analyzer_test.go`

**Step 1: Write the BAML function definition**

```baml
// baml_src/analyze.baml

class Finding {
  ruleId string
  level string @description("One of: error, warning, note, none")
  message string @description("Concise description of the issue")
  filePath string
  startLine int
  endLine int
  recommendation string @description("Suggested fix or action")
  explanation string @description("Longer reasoning about why this is an issue")
  confidence float @description("0.0 to 1.0, how confident you are in this finding")
}

function AnalyzeCode(code: string, policies: string) -> Finding[] {
  client GPT4o
  prompt #"
    You are a code analyzer. Analyze the following code against the given policies.
    For each policy violation found, produce a finding. If no violations are found
    for a policy, do not produce a finding for it.

    Be precise about line numbers. Be concise in messages. Be thorough in explanations.
    Set confidence based on how certain you are — use lower confidence for ambiguous cases.

    Policies:
    ---
    {{ policies }}
    ---

    Code:
    ---
    {{ code }}
    ---

    {{ ctx.output_format }}
  "#
}
```

**Step 2: Generate the Go client**

```bash
baml-cli generate
```

**Step 3: Write the Go analyzer wrapper**

```go
// internal/analyzer/analyzer.go
package analyzer

import (
	"context"
	"fmt"
	"strings"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
)

// BAMLClient is the interface for the generated BAML client.
// This allows testing with a mock.
type BAMLClient interface {
	AnalyzeCode(ctx context.Context, code string, policies string) ([]Finding, error)
}

// Finding mirrors the BAML-generated Finding type.
type Finding struct {
	RuleID         string  `json:"ruleId"`
	Level          string  `json:"level"`
	Message        string  `json:"message"`
	FilePath       string  `json:"filePath"`
	StartLine      int     `json:"startLine"`
	EndLine        int     `json:"endLine"`
	Recommendation string  `json:"recommendation"`
	Explanation    string  `json:"explanation"`
	Confidence     float64 `json:"confidence"`
}

// Analyzer performs LLM-based code analysis via BAML.
type Analyzer struct {
	client BAMLClient
}

// NewAnalyzer creates an analyzer with the given BAML client.
func NewAnalyzer(client BAMLClient) *Analyzer {
	return &Analyzer{client: client}
}

// FormatPolicies renders policies into the text format for the LLM prompt.
func FormatPolicies(policies map[string]config.Policy) string {
	var sb strings.Builder
	for name, p := range policies {
		if !p.Enabled {
			continue
		}
		fmt.Fprintf(&sb, "- %s [%s]: %s\n", name, p.Severity, p.Instruction)
	}
	return sb.String()
}

// Analyze runs analysis on the given artifacts against the given policies.
func (a *Analyzer) Analyze(ctx context.Context, artifacts []input.Artifact, policies map[string]config.Policy) ([]sarif.Result, error) {
	policyText := FormatPolicies(policies)
	if policyText == "" {
		return nil, nil
	}

	var allResults []sarif.Result

	for _, art := range artifacts {
		findings, err := a.client.AnalyzeCode(ctx, art.Content, policyText)
		if err != nil {
			return nil, fmt.Errorf("analyzing %s: %w", art.Path, err)
		}

		for _, f := range findings {
			path := f.FilePath
			if path == "" {
				path = art.Path
			}

			allResults = append(allResults, sarif.Result{
				RuleID:  f.RuleID,
				Level:   f.Level,
				Message: sarif.Message{Text: f.Message},
				Locations: []sarif.Location{{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{URI: path},
						Region:           sarif.Region{StartLine: f.StartLine, EndLine: f.EndLine},
					},
				}},
				Properties: map[string]interface{}{
					"gavel/recommendation": f.Recommendation,
					"gavel/explanation":    f.Explanation,
					"gavel/confidence":     f.Confidence,
				},
			})
		}
	}

	return allResults, nil
}
```

**Step 4: Write the test with a mock BAML client**

```go
// internal/analyzer/analyzer_test.go
package analyzer

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
)

type mockBAMLClient struct {
	findings []Finding
	err      error
}

func (m *mockBAMLClient) AnalyzeCode(ctx context.Context, code string, policies string) ([]Finding, error) {
	return m.findings, m.err
}

func TestAnalyzer_Analyze(t *testing.T) {
	mock := &mockBAMLClient{
		findings: []Finding{
			{
				RuleID:         "error-handling",
				Level:          "warning",
				Message:        "Function Foo ignores error from Bar()",
				FilePath:       "pkg/foo.go",
				StartLine:      10,
				EndLine:        12,
				Recommendation: "Return the error",
				Explanation:    "Bar() returns an error that is discarded",
				Confidence:     0.85,
			},
		},
	}

	a := NewAnalyzer(mock)
	artifacts := []input.Artifact{
		{Path: "pkg/foo.go", Content: "package pkg\n\nfunc Foo() { Bar() }\n", Kind: input.KindFile},
	}
	policies := map[string]config.Policy{
		"error-handling": {
			Description: "Handle errors",
			Severity:    "warning",
			Instruction: "Check error handling",
			Enabled:     true,
		},
	}

	results, err := a.Analyze(context.Background(), artifacts, policies)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.RuleID != "error-handling" {
		t.Errorf("expected ruleId 'error-handling', got %q", r.RuleID)
	}
	if r.Properties["gavel/confidence"] != 0.85 {
		t.Errorf("expected confidence 0.85, got %v", r.Properties["gavel/confidence"])
	}
}

func TestAnalyzer_SkipsDisabledPolicies(t *testing.T) {
	mock := &mockBAMLClient{findings: nil}

	a := NewAnalyzer(mock)
	policies := map[string]config.Policy{
		"disabled-rule": {
			Instruction: "This should not run",
			Enabled:     false,
		},
	}

	results, err := a.Analyze(context.Background(), []input.Artifact{{Content: "code"}}, policies)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil results for all-disabled policies, got %v", results)
	}
}

func TestFormatPolicies(t *testing.T) {
	policies := map[string]config.Policy{
		"rule-a": {Severity: "warning", Instruction: "Do A", Enabled: true},
		"rule-b": {Severity: "error", Instruction: "Do B", Enabled: false},
	}

	text := FormatPolicies(policies)
	if !contains(text, "rule-a") {
		t.Error("expected rule-a in output")
	}
	if contains(text, "rule-b") {
		t.Error("did not expect disabled rule-b in output")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

**Step 5: Run tests**

Run: `task test`
Expected: PASS

**Step 6: Commit**

```bash
git add baml_src/analyze.baml internal/analyzer/
git commit -m "feat: add BAML analyzer with mock-testable interface"
```

---

### Task 7: SARIF Assembler

Merges results from the analyzer into a complete SARIF document.

**Files:**
- Create: `internal/sarif/assembler.go`
- Create: `internal/sarif/assembler_test.go`

**Step 1: Write the failing test**

```go
// internal/sarif/assembler_test.go
package sarif

import (
	"testing"
)

func TestAssemble(t *testing.T) {
	results := []Result{
		{RuleID: "rule-a", Level: "warning", Message: Message{Text: "issue A"}},
		{RuleID: "rule-b", Level: "error", Message: Message{Text: "issue B"}},
	}

	rules := []ReportingDescriptor{
		{ID: "rule-a", ShortDescription: Message{Text: "Rule A"}},
		{ID: "rule-b", ShortDescription: Message{Text: "Rule B"}},
	}

	log := Assemble(results, rules, "diff")

	if len(log.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(log.Runs))
	}
	run := log.Runs[0]
	if run.Tool.Driver.Name != "gavel" {
		t.Errorf("expected tool name 'gavel', got %q", run.Tool.Driver.Name)
	}
	if len(run.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(run.Results))
	}
	if len(run.Tool.Driver.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(run.Tool.Driver.Rules))
	}
	if run.Properties["gavel/inputScope"] != "diff" {
		t.Errorf("expected inputScope 'diff', got %v", run.Properties["gavel/inputScope"])
	}
}

func TestAssemble_Dedup(t *testing.T) {
	results := []Result{
		{
			RuleID: "rule-a", Level: "warning", Message: Message{Text: "issue"},
			Locations: []Location{{PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "foo.go"},
				Region:           Region{StartLine: 10, EndLine: 15},
			}}},
			Properties: map[string]interface{}{"gavel/confidence": 0.7},
		},
		{
			RuleID: "rule-a", Level: "warning", Message: Message{Text: "issue duplicate"},
			Locations: []Location{{PhysicalLocation: PhysicalLocation{
				ArtifactLocation: ArtifactLocation{URI: "foo.go"},
				Region:           Region{StartLine: 12, EndLine: 18},
			}}},
			Properties: map[string]interface{}{"gavel/confidence": 0.9},
		},
	}

	log := Assemble(results, nil, "files")
	if len(log.Runs[0].Results) != 1 {
		t.Errorf("expected dedup to 1 result, got %d", len(log.Runs[0].Results))
	}
	// Should keep the higher confidence one
	if log.Runs[0].Results[0].Properties["gavel/confidence"] != 0.9 {
		t.Errorf("expected to keep higher confidence finding")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `task test`
Expected: FAIL

**Step 3: Implement assembler**

```go
// internal/sarif/assembler.go
package sarif

// Assemble creates a SARIF log from analysis results, deduplicating overlapping findings.
func Assemble(results []Result, rules []ReportingDescriptor, inputScope string) *Log {
	deduped := dedup(results)

	log := NewLog("gavel", "0.1.0")
	log.Runs[0].Tool.Driver.Rules = rules
	log.Runs[0].Results = deduped
	log.Runs[0].Properties = map[string]interface{}{
		"gavel/inputScope": inputScope,
	}

	return log
}

// dedup removes duplicate findings (same ruleId + overlapping location),
// keeping the one with higher confidence.
func dedup(results []Result) []Result {
	type key struct {
		ruleID string
		uri    string
	}

	best := make(map[key]Result)
	for _, r := range results {
		uri := ""
		if len(r.Locations) > 0 {
			uri = r.Locations[0].PhysicalLocation.ArtifactLocation.URI
		}
		k := key{ruleID: r.RuleID, uri: uri}

		existing, ok := best[k]
		if !ok {
			best[k] = r
			continue
		}

		// Check for overlapping regions
		if len(r.Locations) > 0 && len(existing.Locations) > 0 {
			rRegion := r.Locations[0].PhysicalLocation.Region
			eRegion := existing.Locations[0].PhysicalLocation.Region
			if regionsOverlap(rRegion, eRegion) {
				if confidence(r) > confidence(existing) {
					best[k] = r
				}
				continue
			}
		}

		// Non-overlapping same rule+file: keep both by making key unique
		// Use a unique key to avoid collision
		for i := 1; ; i++ {
			newKey := key{ruleID: r.RuleID + string(rune(i)), uri: uri}
			if _, exists := best[newKey]; !exists {
				best[newKey] = r
				break
			}
		}
	}

	out := make([]Result, 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	return out
}

func regionsOverlap(a, b Region) bool {
	return a.StartLine <= b.EndLine && b.StartLine <= a.EndLine
}

func confidence(r Result) float64 {
	if r.Properties == nil {
		return 0
	}
	if c, ok := r.Properties["gavel/confidence"].(float64); ok {
		return c
	}
	return 0
}
```

**Step 4: Run tests**

Run: `task test`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/sarif/assembler.go internal/sarif/assembler_test.go
git commit -m "feat: add SARIF assembler with deduplication"
```

---

### Task 8: Rego Evaluator

Evaluates SARIF documents against Rego policies to produce verdicts.

**Files:**
- Create: `internal/evaluator/evaluator.go`
- Create: `internal/evaluator/evaluator_test.go`
- Create: `internal/evaluator/default.rego`

**Step 1: Write the failing test**

```go
// internal/evaluator/evaluator_test.go
package evaluator

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

func TestEvaluator_Reject(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{
			RuleID:  "error-handling",
			Level:   "error",
			Message: sarif.Message{Text: "Critical error"},
			Properties: map[string]interface{}{
				"gavel/confidence": 0.9,
			},
		},
	}

	e, err := NewEvaluator("")
	if err != nil {
		t.Fatal(err)
	}

	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "reject" {
		t.Errorf("expected 'reject', got %q", verdict.Decision)
	}
}

func TestEvaluator_Merge(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")
	// No results = clean

	e, err := NewEvaluator("")
	if err != nil {
		t.Fatal(err)
	}

	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "merge" {
		t.Errorf("expected 'merge', got %q", verdict.Decision)
	}
}

func TestEvaluator_Review(t *testing.T) {
	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{
			RuleID:  "function-length",
			Level:   "warning",
			Message: sarif.Message{Text: "Function too long"},
			Properties: map[string]interface{}{
				"gavel/confidence": 0.7,
			},
		},
	}

	e, err := NewEvaluator("")
	if err != nil {
		t.Fatal(err)
	}

	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "review" {
		t.Errorf("expected 'review', got %q", verdict.Decision)
	}
}

func TestEvaluator_CustomPolicy(t *testing.T) {
	policy := `
package gavel.gate

default decision = "merge"

decision = "reject" {
    count(input.runs[0].results) > 0
}
`

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "strict.rego"), []byte(policy), 0644)

	log := sarif.NewLog("gavel", "0.1.0")
	log.Runs[0].Results = []sarif.Result{
		{RuleID: "any-rule", Level: "note", Message: sarif.Message{Text: "Minor"}},
	}

	e, err := NewEvaluator(dir)
	if err != nil {
		t.Fatal(err)
	}

	verdict, err := e.Evaluate(context.Background(), log)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "reject" {
		t.Errorf("expected 'reject' from strict policy, got %q", verdict.Decision)
	}
}
```

Note: Add `"os"` and `"path/filepath"` to the test imports.

**Step 2: Run test to verify it fails**

Run: `task test`
Expected: FAIL

**Step 3: Write the default Rego policy**

```rego
// internal/evaluator/default.rego
package gavel.gate

default decision = "review"

decision = "reject" {
    some result in input.runs[0].results
    result.level == "error"
    result.properties["gavel/confidence"] > 0.8
}

decision = "merge" {
    count(input.runs[0].results) == 0
}
```

**Step 4: Implement the evaluator**

```go
// internal/evaluator/evaluator.go
package evaluator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-policy-agent/opa/v1/rego"

	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

//go:embed default.rego
var defaultPolicy string

// Evaluator runs Rego policies against SARIF documents.
type Evaluator struct {
	query rego.PreparedEvalQuery
}

// NewEvaluator creates an evaluator. If policyDir is empty, uses the default policy.
// If policyDir is set, loads all .rego files from that directory.
func NewEvaluator(policyDir string) (*Evaluator, error) {
	ctx := context.Background()

	modules := []func(*rego.Rego){
		rego.Query("data.gavel.gate.decision"),
		rego.Module("default.rego", defaultPolicy),
	}

	if policyDir != "" {
		entries, err := os.ReadDir(policyDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading policy dir: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".rego") {
				data, err := os.ReadFile(filepath.Join(policyDir, e.Name()))
				if err != nil {
					return nil, err
				}
				// Custom policies override the default
				modules = []func(*rego.Rego){
					rego.Query("data.gavel.gate.decision"),
					rego.Module(e.Name(), string(data)),
				}
			}
		}
	}

	query, err := rego.New(modules...).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("preparing rego query: %w", err)
	}

	return &Evaluator{query: query}, nil
}

// Evaluate runs the Rego policy against a SARIF document and returns a verdict.
func (e *Evaluator) Evaluate(ctx context.Context, log *sarif.Log) (*store.Verdict, error) {
	// Convert SARIF log to a generic map for OPA input
	data, err := json.Marshal(log)
	if err != nil {
		return nil, err
	}
	var input interface{}
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}

	results, err := e.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("evaluating rego: %w", err)
	}

	decision := "review" // default
	if len(results) > 0 && len(results[0].Expressions) > 0 {
		if d, ok := results[0].Expressions[0].Value.(string); ok {
			decision = d
		}
	}

	// Collect relevant findings based on decision
	var relevant []sarif.Result
	if len(log.Runs) > 0 {
		for _, r := range log.Runs[0].Results {
			if decision == "reject" && r.Level == "error" {
				relevant = append(relevant, r)
			} else if decision == "review" && (r.Level == "warning" || r.Level == "error") {
				relevant = append(relevant, r)
			}
		}
	}

	return &store.Verdict{
		Decision:         decision,
		Reason:           fmt.Sprintf("Decision: %s based on %d findings", decision, len(log.Runs[0].Results)),
		RelevantFindings: relevant,
	}, nil
}
```

**Step 5: Run tests**

Run: `task test`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/evaluator/
git commit -m "feat: add Rego evaluator with default policy"
```

---

### Task 9: CLI Wiring

Wire all components together with a Cobra CLI.

**Files:**
- Modify: `cmd/gavel/main.go`
- Create: `cmd/gavel/analyze.go`

**Step 1: Install Cobra**

```bash
go get github.com/spf13/cobra
```

**Step 2: Implement the root command**

```go
// cmd/gavel/main.go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gavel",
	Short: "AI-powered code analysis with structured output",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

**Step 3: Implement the analyze command**

```go
// cmd/gavel/analyze.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

var (
	flagFiles     []string
	flagDiff      string
	flagDir       string
	flagOutput    string
	flagPolicyDir string
	flagRegoDir   string
)

func init() {
	analyzeCmd := &cobra.Command{
		Use:   "analyze",
		Short: "Analyze code against policies",
		RunE:  runAnalyze,
	}

	analyzeCmd.Flags().StringSliceVar(&flagFiles, "files", nil, "Files to analyze")
	analyzeCmd.Flags().StringVar(&flagDiff, "diff", "", "Path to diff file (or - for stdin)")
	analyzeCmd.Flags().StringVar(&flagDir, "dir", "", "Directory to analyze")
	analyzeCmd.Flags().StringVar(&flagOutput, "output", ".gavel/results", "Output directory for results")
	analyzeCmd.Flags().StringVar(&flagPolicyDir, "policies", ".gavel", "Directory containing policies.yaml")
	analyzeCmd.Flags().StringVar(&flagRegoDir, "rego", ".gavel/rego", "Directory containing Rego policies")

	rootCmd.AddCommand(analyzeCmd)
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load configuration
	machineConfig := os.ExpandEnv("$HOME/.config/gavel/policies.yaml")
	projectConfig := flagPolicyDir + "/policies.yaml"
	cfg, err := config.LoadTiered(machineConfig, projectConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Read input
	h := input.NewHandler()
	var artifacts []input.Artifact
	var inputScope string

	switch {
	case len(flagFiles) > 0:
		artifacts, err = h.ReadFiles(flagFiles)
		inputScope = "files"
	case flagDiff != "":
		var diffContent string
		if flagDiff == "-" {
			data, readErr := os.ReadFile("/dev/stdin")
			if readErr != nil {
				return readErr
			}
			diffContent = string(data)
		} else {
			data, readErr := os.ReadFile(flagDiff)
			if readErr != nil {
				return readErr
			}
			diffContent = string(data)
		}
		artifacts, err = h.ReadDiff(diffContent)
		inputScope = "diff"
	case flagDir != "":
		artifacts, err = h.ReadDirectory(flagDir)
		inputScope = "directory"
	default:
		return fmt.Errorf("specify --files, --diff, or --dir")
	}
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}

	// TODO: Replace with real BAML client once generated
	_ = analyzer.NewAnalyzer(nil)
	_ = artifacts
	_ = cfg

	// For now, create empty results to demonstrate the pipeline
	results := []sarif.Result{}
	rules := []sarif.ReportingDescriptor{}
	for name, p := range cfg.Policies {
		if p.Enabled {
			rules = append(rules, sarif.ReportingDescriptor{
				ID:               name,
				ShortDescription: sarif.Message{Text: p.Description},
				DefaultConfig:    &sarif.ReportingConfiguration{Level: p.Severity},
			})
		}
	}

	// Assemble SARIF
	sarifLog := sarif.Assemble(results, rules, inputScope)

	// Store results
	fs := store.NewFileStore(flagOutput)
	id, err := fs.WriteSARIF(ctx, sarifLog)
	if err != nil {
		return fmt.Errorf("storing SARIF: %w", err)
	}

	// Evaluate with Rego
	eval, err := evaluator.NewEvaluator(flagRegoDir)
	if err != nil {
		return fmt.Errorf("creating evaluator: %w", err)
	}

	verdict, err := eval.Evaluate(ctx, sarifLog)
	if err != nil {
		return fmt.Errorf("evaluating: %w", err)
	}

	if err := fs.WriteVerdict(ctx, id, verdict); err != nil {
		return fmt.Errorf("storing verdict: %w", err)
	}

	// Output verdict
	out, _ := json.MarshalIndent(verdict, "", "  ")
	fmt.Println(string(out))

	return nil
}
```

**Step 4: Build and test manually**

```bash
task build
./gavel analyze --help
```

Expected: prints analyze command help with flags

**Step 5: Commit**

```bash
git add cmd/gavel/ go.mod go.sum
git commit -m "feat: add CLI with analyze command wiring all components"
```

---

### Task 10: Integration Test

Write an end-to-end test that exercises the full pipeline with a mock BAML client.

**Files:**
- Create: `integration_test.go`

**Step 1: Write the integration test**

```go
// integration_test.go
package gavel_test

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/evaluator"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/chris-regnier/gavel/internal/store"
)

type mockClient struct{}

func (m *mockClient) AnalyzeCode(ctx context.Context, code string, policies string) ([]analyzer.Finding, error) {
	return []analyzer.Finding{
		{
			RuleID:         "error-handling",
			Level:          "warning",
			Message:        "Function Foo ignores error",
			FilePath:       "main.go",
			StartLine:      3,
			EndLine:        3,
			Recommendation: "Handle the error",
			Explanation:    "Error from Bar() is discarded",
			Confidence:     0.8,
		},
	}, nil
}

func TestFullPipeline(t *testing.T) {
	ctx := context.Background()

	// 1. Config
	cfg := config.SystemDefaults()

	// 2. Input
	h := input.NewHandler()
	artifacts, err := h.ReadFiles([]string{}) // Would use real files
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an artifact
	artifacts = []input.Artifact{
		{Path: "main.go", Content: "package main\n\nfunc Foo() { Bar() }\n", Kind: input.KindFile},
	}

	// 3. Analyze
	a := analyzer.NewAnalyzer(&mockClient{})
	results, err := a.Analyze(ctx, artifacts, cfg.Policies)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// 4. Assemble SARIF
	sarifLog := sarif.Assemble(results, nil, "files")
	if len(sarifLog.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 SARIF result, got %d", len(sarifLog.Runs[0].Results))
	}

	// 5. Store
	dir := t.TempDir()
	fs := store.NewFileStore(dir)
	id, err := fs.WriteSARIF(ctx, sarifLog)
	if err != nil {
		t.Fatal(err)
	}

	// 6. Evaluate
	eval, err := evaluator.NewEvaluator("")
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := eval.Evaluate(ctx, sarifLog)
	if err != nil {
		t.Fatal(err)
	}

	if verdict.Decision != "review" {
		t.Errorf("expected 'review' for warning-level finding, got %q", verdict.Decision)
	}

	// 7. Store verdict
	if err := fs.WriteVerdict(ctx, id, verdict); err != nil {
		t.Fatal(err)
	}

	// 8. Verify storage
	loaded, err := fs.ReadVerdict(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Decision != "review" {
		t.Errorf("expected stored verdict 'review', got %q", loaded.Decision)
	}
}
```

**Step 2: Run integration test**

Run: `task test`
Expected: PASS

**Step 3: Commit**

```bash
git add integration_test.go
git commit -m "feat: add end-to-end integration test with mock BAML client"
```

---

### Summary of Tasks

| Task | Component | Dependencies |
|------|-----------|-------------|
| 1 | Project Scaffolding | None |
| 2 | Configuration System | Task 1 |
| 3 | SARIF Types | Task 1 |
| 4 | Storage Backend | Task 3 |
| 5 | Input Handler | Task 1 |
| 6 | BAML Analyzer | Tasks 2, 3, 5 |
| 7 | SARIF Assembler | Task 3 |
| 8 | Rego Evaluator | Tasks 3, 4 |
| 9 | CLI Wiring | Tasks 2–8 |
| 10 | Integration Test | Tasks 2–8 |
