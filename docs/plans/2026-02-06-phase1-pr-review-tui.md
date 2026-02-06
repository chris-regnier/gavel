# Phase 1: PR Review TUI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a terminal-based PR review interface with inline AI feedback, syntax highlighting, and markdown-rendered explanations.

**Architecture:** Leverage existing SARIF analysis pipeline, add TUI layer with bubbletea for interactive review. Three-pane layout (file tree, code view, finding details) with navigation and review state persistence.

**Tech Stack:** Go, bubbletea (TUI framework), lipgloss (styling), glamour (markdown rendering), chroma (syntax highlighting)

---

## Task 1: Add Cache Metadata to SARIF

**Goal:** Extend SARIF results with cache key and analyzer metadata for cross-environment sharing.

**Files:**
- Modify: `internal/sarif/sarif.go`
- Modify: `internal/sarif/assembler.go`
- Test: `internal/sarif/assembler_test.go`

### Step 1: Write failing test for cache metadata

Add to `internal/sarif/assembler_test.go`:

```go
func TestAssembler_AddsCacheMetadata(t *testing.T) {
	findings := []analyzer.Finding{
		{
			RuleID:         1,
			Message:        "Test finding",
			Confidence:     0.9,
			Explanation:    "Test explanation",
			Recommendation: "Test recommendation",
			StartLine:      10,
			EndLine:        12,
		},
	}

	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Name: "openrouter",
			OpenRouter: config.OpenRouterConfig{
				Model: "anthropic/claude-sonnet-4",
			},
		},
		Policies: map[string]config.PolicyConfig{
			"test-policy": {
				Enabled:     true,
				Instruction: "Test instruction",
			},
		},
	}

	fileHash := "abc123def456" // Mock file content hash
	bamlVersion := "v1.0.0"    // Mock BAML version

	log := NewAssembler().
		WithCacheMetadata(fileHash, cfg, bamlVersion).
		AddFile("test.go", findings).
		Build()

	result := log.Runs[0].Results[0]

	// Check cache_key property
	cacheKey, ok := result.Properties["gavel/cache_key"].(string)
	if !ok || cacheKey == "" {
		t.Fatal("Expected gavel/cache_key property")
	}

	// Check analyzer metadata
	analyzer, ok := result.Properties["gavel/analyzer"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected gavel/analyzer property")
	}

	if analyzer["provider"] != "openrouter" {
		t.Errorf("Expected provider=openrouter, got %v", analyzer["provider"])
	}

	if analyzer["model"] != "anthropic/claude-sonnet-4" {
		t.Errorf("Expected model=anthropic/claude-sonnet-4, got %v", analyzer["model"])
	}

	policies, ok := analyzer["policies"].(map[string]interface{})
	if !ok || len(policies) == 0 {
		t.Fatal("Expected policies in analyzer metadata")
	}
}
```

### Step 2: Run test to verify it fails

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/sarif/ -run TestAssembler_AddsCacheMetadata -v
```

Expected: FAIL with "undefined: NewAssembler" or "undefined: WithCacheMetadata"

### Step 3: Implement cache metadata types

Add to `internal/sarif/sarif.go`:

```go
// CacheMetadata represents metadata for content-addressable caching
type CacheMetadata struct {
	FileHash    string
	Provider    string
	Model       string
	BAMLVersion string
	Policies    map[string]PolicyMetadata
}

// PolicyMetadata represents policy configuration for cache key
type PolicyMetadata struct{
	Instruction string
	Version     string
}

// ComputeCacheKey generates deterministic hash from cache metadata
func (m *CacheMetadata) ComputeCacheKey() string {
	data := struct {
		File     string
		Provider string
		Model    string
		BAML     string
		Policies map[string]PolicyMetadata
	}{
		File:     m.FileHash,
		Provider: m.Provider,
		Model:    m.Model,
		BAML:     m.BAMLVersion,
		Policies: m.Policies,
	}

	b, _ := json.Marshal(data)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
```

### Step 4: Implement Assembler with cache metadata

Modify `internal/sarif/assembler.go`:

```go
package sarif

import (
	"time"

	"github.com/chris-regnier/gavel/internal/analyzer"
	"github.com/chris-regnier/gavel/internal/config"
)

type Assembler struct {
	files        map[string][]analyzer.Finding
	cacheMetadata *CacheMetadata
	environment  string
	analysisID   string
}

func NewAssembler() *Assembler {
	return &Assembler{
		files:       make(map[string][]analyzer.Finding),
		environment: "local",
		analysisID:  generateAnalysisID(),
	}
}

func (a *Assembler) WithCacheMetadata(fileHash string, cfg *config.Config, bamlVersion string) *Assembler {
	policies := make(map[string]PolicyMetadata)
	for id, policy := range cfg.Policies {
		if policy.Enabled {
			policies[id] = PolicyMetadata{
				Instruction: policy.Instruction,
				Version:     "v1",
			}
		}
	}

	provider := cfg.Provider.Name
	model := ""
	if provider == "openrouter" {
		model = cfg.Provider.OpenRouter.Model
	} else if provider == "ollama" {
		model = cfg.Provider.Ollama.Model
	}

	a.cacheMetadata = &CacheMetadata{
		FileHash:    fileHash,
		Provider:    provider,
		Model:       model,
		BAMLVersion: bamlVersion,
		Policies:    policies,
	}
	return a
}

func (a *Assembler) AddFile(filePath string, findings []analyzer.Finding) *Assembler {
	a.files[filePath] = findings
	return a
}

func (a *Assembler) Build() *Log {
	var results []Result

	for filePath, findings := range a.files {
		for _, f := range findings {
			result := Result{
				RuleID: f.RuleID,
				Level:  severityToLevel(f.Severity),
				Message: Message{
					Text: f.Message,
				},
				Locations: []Location{
					{
						PhysicalLocation: PhysicalLocation{
							ArtifactLocation: ArtifactLocation{
								URI: filePath,
							},
							Region: Region{
								StartLine: f.StartLine,
								EndLine:   f.EndLine,
							},
						},
					},
				},
				Properties: make(map[string]interface{}),
			}

			// Add existing gavel properties
			result.Properties["gavel/confidence"] = f.Confidence
			result.Properties["gavel/explanation"] = f.Explanation
			result.Properties["gavel/recommendation"] = f.Recommendation

			// Add cache metadata if available
			if a.cacheMetadata != nil {
				result.Properties["gavel/cache_key"] = a.cacheMetadata.ComputeCacheKey()
				result.Properties["gavel/analysis_id"] = a.analysisID
				result.Properties["gavel/analyzed_at"] = time.Now().UTC().Format(time.RFC3339)
				result.Properties["gavel/environment"] = a.environment

				result.Properties["gavel/analyzer"] = map[string]interface{}{
					"provider":     a.cacheMetadata.Provider,
					"model":        a.cacheMetadata.Model,
					"baml_version": a.cacheMetadata.BAMLVersion,
					"policies":     a.cacheMetadata.Policies,
				}
			}

			results = append(results, result)
		}
	}

	return &Log{
		Schema:  SchemaURI,
		Version: Version,
		Runs: []Run{
			{
				Tool: Tool{
					Driver: Driver{
						Name:           "gavel",
						Version:        "0.1.0",
						InformationURI: "https://github.com/chris-regnier/gavel",
					},
				},
				Results: results,
			},
		},
	}
}

func generateAnalysisID() string {
	return time.Now().UTC().Format("2006-01-02T15-04-05Z")
}

func severityToLevel(severity string) string {
	switch severity {
	case "error":
		return "error"
	case "warning":
		return "warning"
	default:
		return "note"
	}
}
```

### Step 5: Run test to verify it passes

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/sarif/ -run TestAssembler_AddsCacheMetadata -v
```

Expected: PASS

### Step 6: Commit

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
git add internal/sarif/sarif.go internal/sarif/assembler.go internal/sarif/assembler_test.go
git commit -m "feat(sarif): add cache metadata and analyzer properties

- Add CacheMetadata type with deterministic hash generation
- Extend Assembler with WithCacheMetadata builder method
- Include gavel/cache_key, gavel/analyzer in SARIF results
- Add test for cache metadata presence and correctness"
```

---

## Task 2: Add TUI Dependencies

**Goal:** Add bubbletea, lipgloss, and glamour dependencies for TUI implementation.

**Files:**
- Modify: `go.mod`

### Step 1: Add dependencies

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/glamour@latest
go get github.com/alecthomas/chroma/v2@latest
```

### Step 2: Verify dependencies downloaded

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go mod tidy
go mod verify
```

Expected: "all modules verified"

### Step 3: Commit

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
git add go.mod go.sum
git commit -m "deps: add TUI dependencies (bubbletea, lipgloss, glamour, chroma)"
```

---

## Task 3: Create Review Model Structure

**Goal:** Define the bubbletea model for the review TUI.

**Files:**
- Create: `internal/review/model.go`
- Create: `internal/review/model_test.go`

### Step 1: Write failing test for model initialization

Create `internal/review/model_test.go`:

```go
package review

import (
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestNewReviewModel(t *testing.T) {
	log := &sarif.Log{
		Runs: []sarif.Run{
			{
				Results: []sarif.Result{
					{
						RuleID: "test-rule",
						Level:  "error",
						Message: sarif.Message{
							Text: "Test finding",
						},
						Locations: []sarif.Location{
							{
								PhysicalLocation: sarif.PhysicalLocation{
									ArtifactLocation: sarif.ArtifactLocation{
										URI: "test.go",
									},
									Region: sarif.Region{
										StartLine: 10,
										EndLine:   12,
									},
								},
							},
						},
						Properties: map[string]interface{}{
							"gavel/confidence":     0.9,
							"gavel/explanation":    "Test explanation",
							"gavel/recommendation": "Test recommendation",
						},
					},
				},
			},
		},
	}

	model := NewReviewModel(log)

	if model.sarif == nil {
		t.Fatal("Expected sarif log to be set")
	}

	if len(model.findings) != 1 {
		t.Fatalf("Expected 1 finding, got %d", len(model.findings))
	}

	if len(model.files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(model.files))
	}

	if model.currentFile != 0 {
		t.Errorf("Expected currentFile=0, got %d", model.currentFile)
	}

	if model.currentFinding != 0 {
		t.Errorf("Expected currentFinding=0, got %d", model.currentFinding)
	}

	if model.activePane != PaneFiles {
		t.Errorf("Expected activePane=PaneFiles, got %v", model.activePane)
	}
}
```

### Step 2: Run test to verify it fails

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/review/ -run TestNewReviewModel -v
```

Expected: FAIL with "package internal/review is not in std"

### Step 3: Create directory and implement model

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
mkdir -p internal/review
```

Create `internal/review/model.go`:

```go
package review

import (
	"github.com/chris-regnier/gavel/internal/sarif"
)

// Pane represents which pane is currently active
type Pane int

const (
	PaneFiles Pane = iota
	PaneCode
	PaneDetails
)

// Filter represents the severity filter
type Filter int

const (
	FilterAll Filter = iota
	FilterErrors
	FilterWarnings
)

// ReviewModel is the bubbletea model for the review TUI
type ReviewModel struct {
	sarif    *sarif.Log
	findings []sarif.Result
	files    map[string][]sarif.Result

	currentFile    int
	currentFinding int
	activePane     Pane
	filter         Filter

	accepted map[string]bool
	rejected map[string]bool
	comments map[string]string
}

// NewReviewModel creates a new ReviewModel from a SARIF log
func NewReviewModel(log *sarif.Log) *ReviewModel {
	m := &ReviewModel{
		sarif:      log,
		findings:   []sarif.Result{},
		files:      make(map[string][]sarif.Result),
		activePane: PaneFiles,
		filter:     FilterAll,
		accepted:   make(map[string]bool),
		rejected:   make(map[string]bool),
		comments:   make(map[string]string),
	}

	// Extract findings and group by file
	if len(log.Runs) > 0 {
		for _, result := range log.Runs[0].Results {
			m.findings = append(m.findings, result)

			if len(result.Locations) > 0 {
				filePath := result.Locations[0].PhysicalLocation.ArtifactLocation.URI
				m.files[filePath] = append(m.files[filePath], result)
			}
		}
	}

	return m
}
```

### Step 4: Run test to verify it passes

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/review/ -run TestNewReviewModel -v
```

Expected: PASS

### Step 5: Commit

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
git add internal/review/model.go internal/review/model_test.go
git commit -m "feat(review): add ReviewModel with SARIF parsing and state management

- Define Pane and Filter types for TUI state
- Implement NewReviewModel constructor with finding extraction
- Group findings by file path for file tree display
- Initialize review state maps (accepted, rejected, comments)"
```

---

## Task 4: Implement Review State Persistence

**Goal:** Add functionality to save and load review state to/from JSON.

**Files:**
- Create: `internal/review/persistence.go`
- Create: `internal/review/persistence_test.go`

### Step 1: Write failing test for save/load

Create `internal/review/persistence_test.go`:

```go
package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadReviewState(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "test-review.json")

	model := &ReviewModel{
		accepted: map[string]bool{
			"rule1:file.go:10": true,
		},
		rejected: map[string]bool{
			"rule2:file.go:20": true,
		},
		comments: map[string]string{
			"rule1:file.go:10": "Looks good",
			"rule2:file.go:20": "False positive",
		},
	}

	// Save
	if err := SaveReviewState(model, "test-sarif-id", stateFile); err != nil {
		t.Fatalf("SaveReviewState failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Fatal("State file was not created")
	}

	// Load
	loaded, err := LoadReviewState(stateFile)
	if err != nil {
		t.Fatalf("LoadReviewState failed: %v", err)
	}

	if loaded.SarifID != "test-sarif-id" {
		t.Errorf("Expected SarifID='test-sarif-id', got '%s'", loaded.SarifID)
	}

	if len(loaded.Findings) != 2 {
		t.Errorf("Expected 2 findings, got %d", len(loaded.Findings))
	}

	// Check accepted finding
	finding1 := loaded.Findings["rule1:file.go:10"]
	if finding1.Status != "accepted" {
		t.Errorf("Expected status='accepted', got '%s'", finding1.Status)
	}
	if finding1.Comment != "Looks good" {
		t.Errorf("Expected comment='Looks good', got '%s'", finding1.Comment)
	}

	// Check rejected finding
	finding2 := loaded.Findings["rule2:file.go:20"]
	if finding2.Status != "rejected" {
		t.Errorf("Expected status='rejected', got '%s'", finding2.Status)
	}
}
```

### Step 2: Run test to verify it fails

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/review/ -run TestSaveAndLoadReviewState -v
```

Expected: FAIL with "undefined: SaveReviewState"

### Step 3: Implement persistence

Create `internal/review/persistence.go`:

```go
package review

import (
	"encoding/json"
	"os"
	"time"
)

// ReviewState represents persisted review state
type ReviewState struct {
	SarifID    string                     `json:"sarif_id"`
	ReviewedAt string                     `json:"reviewed_at"`
	Reviewer   string                     `json:"reviewer"`
	Findings   map[string]FindingReview   `json:"findings"`
}

// FindingReview represents review status for a single finding
type FindingReview struct {
	Status  string `json:"status"`  // "accepted", "rejected", or ""
	Comment string `json:"comment"`
}

// SaveReviewState saves the review model state to a JSON file
func SaveReviewState(model *ReviewModel, sarifID string, filePath string) error {
	state := ReviewState{
		SarifID:    sarifID,
		ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Reviewer:   getUserEmail(),
		Findings:   make(map[string]FindingReview),
	}

	// Collect all reviewed findings
	for findingID := range model.accepted {
		state.Findings[findingID] = FindingReview{
			Status:  "accepted",
			Comment: model.comments[findingID],
		}
	}

	for findingID := range model.rejected {
		state.Findings[findingID] = FindingReview{
			Status:  "rejected",
			Comment: model.comments[findingID],
		}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// LoadReviewState loads review state from a JSON file
func LoadReviewState(filePath string) (*ReviewState, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var state ReviewState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// getUserEmail returns the current user's email (stub for now)
func getUserEmail() string {
	// TODO: Get from git config or environment
	return os.Getenv("USER") + "@localhost"
}
```

### Step 4: Run test to verify it passes

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/review/ -run TestSaveAndLoadReviewState -v
```

Expected: PASS

### Step 5: Commit

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
git add internal/review/persistence.go internal/review/persistence_test.go
git commit -m "feat(review): add review state persistence to JSON

- Define ReviewState and FindingReview types
- Implement SaveReviewState with timestamp and reviewer
- Implement LoadReviewState with JSON unmarshaling
- Add test for save/load round-trip"
```

---

## Task 5: Add Bubbletea Init and Update Methods

**Goal:** Implement bubbletea lifecycle methods (Init, Update, View stub).

**Files:**
- Modify: `internal/review/model.go`
- Create: `internal/review/update.go`
- Create: `internal/review/update_test.go`

### Step 1: Write failing test for key navigation

Create `internal/review/update_test.go`:

```go
package review

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdate_NextFinding(t *testing.T) {
	model := &ReviewModel{
		findings:       make([]sarif.Result, 3), // 3 findings
		currentFinding: 0,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}
	updatedModel, _ := model.Update(msg)

	m := updatedModel.(ReviewModel)
	if m.currentFinding != 1 {
		t.Errorf("Expected currentFinding=1, got %d", m.currentFinding)
	}
}

func TestUpdate_PreviousFinding(t *testing.T) {
	model := &ReviewModel{
		findings:       make([]sarif.Result, 3),
		currentFinding: 1,
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	updatedModel, _ := model.Update(msg)

	m := updatedModel.(ReviewModel)
	if m.currentFinding != 0 {
		t.Errorf("Expected currentFinding=0, got %d", m.currentFinding)
	}
}

func TestUpdate_AcceptFinding(t *testing.T) {
	model := &ReviewModel{
		findings: []sarif.Result{
			{RuleID: "test-rule"},
		},
		currentFinding: 0,
		accepted:       make(map[string]bool),
	}

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updatedModel, _ := model.Update(msg)

	m := updatedModel.(ReviewModel)
	if !m.accepted["test-rule"] {
		t.Error("Expected finding to be marked as accepted")
	}
}
```

### Step 2: Run test to verify it fails

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/review/ -run TestUpdate -v
```

Expected: FAIL with "undefined: Update" or method not found

### Step 3: Implement Init method

Add to `internal/review/model.go`:

```go
import (
	tea "github.com/charmbracelet/bubbletea"
)

// Init implements tea.Model
func (m ReviewModel) Init() tea.Cmd {
	return nil
}
```

### Step 4: Implement Update method

Create `internal/review/update.go`:

```go
package review

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update implements tea.Model
func (m ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "n": // Next finding
			if len(m.findings) > 0 {
				m.currentFinding = (m.currentFinding + 1) % len(m.findings)
			}

		case "p": // Previous finding
			if len(m.findings) > 0 {
				m.currentFinding--
				if m.currentFinding < 0 {
					m.currentFinding = len(m.findings) - 1
				}
			}

		case "a": // Accept finding
			if len(m.findings) > 0 {
				findingID := m.getFindingID(m.currentFinding)
				m.accepted[findingID] = true
				delete(m.rejected, findingID)
			}

		case "r": // Reject finding
			if len(m.findings) > 0 {
				findingID := m.getFindingID(m.currentFinding)
				m.rejected[findingID] = true
				delete(m.accepted, findingID)
			}

		case "tab": // Switch panes
			m.activePane = (m.activePane + 1) % 3

		case "e": // Filter: errors only
			m.filter = FilterErrors

		case "w": // Filter: warnings+
			m.filter = FilterWarnings

		case "f": // Filter: all
			m.filter = FilterAll
		}
	}

	return m, nil
}

// getFindingID generates a unique ID for a finding
func (m *ReviewModel) getFindingID(idx int) string {
	if idx < 0 || idx >= len(m.findings) {
		return ""
	}

	result := m.findings[idx]
	filePath := ""
	line := 0

	if len(result.Locations) > 0 {
		filePath = result.Locations[0].PhysicalLocation.ArtifactLocation.URI
		line = result.Locations[0].PhysicalLocation.Region.StartLine
	}

	return result.RuleID + ":" + filePath + ":" + string(rune(line))
}
```

### Step 5: Run test to verify it passes

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/review/ -run TestUpdate -v
```

Expected: PASS (might need to fix getFindingID implementation for test to pass)

### Step 6: Fix getFindingID for integer conversion

Modify `getFindingID` in `internal/review/update.go`:

```go
import (
	"fmt"
)

func (m *ReviewModel) getFindingID(idx int) string {
	if idx < 0 || idx >= len(m.findings) {
		return ""
	}

	result := m.findings[idx]
	filePath := ""
	line := 0

	if len(result.Locations) > 0 {
		filePath = result.Locations[0].PhysicalLocation.ArtifactLocation.URI
		line = result.Locations[0].PhysicalLocation.Region.StartLine
	}

	return fmt.Sprintf("%s:%s:%d", result.RuleID, filePath, line)
}
```

### Step 7: Run test again

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/review/ -run TestUpdate -v
```

Expected: PASS

### Step 8: Commit

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
git add internal/review/model.go internal/review/update.go internal/review/update_test.go
git commit -m "feat(review): implement bubbletea Update with key navigation

- Add Init method returning nil command
- Implement Update with navigation (n/p for next/prev)
- Add accept/reject actions (a/r keys)
- Add pane switching (tab key)
- Add filtering (e/w/f keys)
- Implement getFindingID for unique finding identification"
```

---

## Task 6: Implement View Rendering (Stub)

**Goal:** Add View method that returns a simple text representation for now.

**Files:**
- Create: `internal/review/view.go`
- Create: `internal/review/view_test.go`

### Step 1: Write test for basic view rendering

Create `internal/review/view_test.go`:

```go
package review

import (
	"strings"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestView_RendersBasicInfo(t *testing.T) {
	model := &ReviewModel{
		findings: []sarif.Result{
			{
				RuleID: "test-rule",
				Message: sarif.Message{
					Text: "Test finding",
				},
			},
		},
		files: map[string][]sarif.Result{
			"test.go": {
				{RuleID: "test-rule"},
			},
		},
	}

	view := model.View()

	// Check for file count
	if !strings.Contains(view, "1 file") {
		t.Error("View should contain file count")
	}

	// Check for finding count
	if !strings.Contains(view, "1 finding") {
		t.Error("View should contain finding count")
	}
}
```

### Step 2: Run test to verify it fails

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/review/ -run TestView -v
```

Expected: FAIL with "undefined: View"

### Step 3: Implement stub View method

Create `internal/review/view.go`:

```go
package review

import (
	"fmt"
)

// View implements tea.Model
func (m ReviewModel) View() string {
	fileCount := len(m.files)
	findingCount := len(m.findings)

	fileText := "files"
	if fileCount == 1 {
		fileText = "file"
	}

	findingText := "findings"
	if findingCount == 1 {
		findingText = "finding"
	}

	return fmt.Sprintf("PR Review: %d %s, %d %s\n\nPress q to quit",
		fileCount, fileText, findingCount, findingText)
}
```

### Step 4: Run test to verify it passes

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
go test ./internal/review/ -run TestView -v
```

Expected: PASS

### Step 5: Commit

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
git add internal/review/view.go internal/review/view_test.go
git commit -m "feat(review): add stub View method for TUI rendering

- Implement basic View showing file and finding counts
- Add pluralization for file/files and finding/findings
- Include quit instruction in view
- Add test for basic view content"
```

---

## Task 7: Add Review Command to CLI

**Goal:** Create `gavel review` command that launches the TUI.

**Files:**
- Create: `cmd/gavel/review.go`
- Modify: `cmd/gavel/main.go`

### Step 1: Create review command

Create `cmd/gavel/review.go`:

```go
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chris-regnier/gavel/internal/review"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Launch interactive PR review TUI",
	Long:  `Analyze code and launch an interactive terminal UI for reviewing findings.`,
	RunE:  runReview,
}

var (
	reviewDiff  string
	reviewFiles string
	reviewDir   string
)

func init() {
	reviewCmd.Flags().StringVar(&reviewDiff, "diff", "", "Path to unified diff (- for stdin)")
	reviewCmd.Flags().StringVar(&reviewFiles, "files", "", "Comma-separated list of files")
	reviewCmd.Flags().StringVar(&reviewDir, "dir", "", "Directory to analyze")
}

func runReview(cmd *cobra.Command, args []string) error {
	// For now, just load a SARIF file if provided as argument
	// TODO: Integrate with full analysis pipeline
	if len(args) == 0 {
		return fmt.Errorf("provide path to SARIF file for now (full analysis integration coming)")
	}

	sarifPath := args[0]
	log, err := loadSARIF(sarifPath)
	if err != nil {
		return fmt.Errorf("failed to load SARIF: %w", err)
	}

	model := review.NewReviewModel(log)
	p := tea.NewProgram(model)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func loadSARIF(path string) (*sarif.Log, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var log sarif.Log
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}

	return &log, nil
}
```

### Step 2: Add json import

Modify the imports in `cmd/gavel/review.go`:

```go
import (
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chris-regnier/gavel/internal/review"
	"github.com/chris-regnier/gavel/internal/sarif"
	"github.com/spf13/cobra"
)
```

### Step 3: Register command in main

Modify `cmd/gavel/main.go` to add the review command:

```go
func init() {
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(reviewCmd)  // Add this line
}
```

### Step 4: Build and test manually

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
task build
```

Expected: Build succeeds

### Step 5: Commit

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
git add cmd/gavel/review.go cmd/gavel/main.go
git commit -m "feat(cli): add gavel review command with TUI launch

- Create review subcommand accepting SARIF file path
- Load SARIF and initialize ReviewModel
- Launch bubbletea program with model
- Register command in root CLI
- TODO: integrate with full analysis pipeline"
```

---

## Task 8: Manual Testing

**Goal:** Verify the TUI launches and basic navigation works.

### Step 1: Generate test SARIF file

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
# Run existing analyze to get a SARIF file
mkdir -p test-data
echo 'package main

func broken() {
	// TODO: implement
}' > test-data/test.go

# Analyze (will fail without API key, but we can use integration test)
go test -run TestIntegration -v 2>&1 | head -20
```

If integration test runs, find the SARIF output path and use it for testing.

### Step 2: Launch TUI with test SARIF

```bash
cd ~/.config/superpowers/worktrees/gavel/lsp-integration
./gavel review .gavel/results/*/sarif.json
```

Expected: TUI launches showing file and finding counts. Press 'q' to quit.

### Step 3: Document manual test results

Create a note of what worked:
- TUI launches: [yes/no]
- Shows file count: [yes/no]
- Shows finding count: [yes/no]
- 'q' quits: [yes/no]
- Navigation (n/p): [yes/no]

---

## Next Steps

The remaining tasks for Phase 1 include:

**Task 9**: Implement file tree pane with lipgloss styling
**Task 10**: Implement code view pane with syntax highlighting (chroma)
**Task 11**: Implement finding details pane with markdown rendering (glamour)
**Task 12**: Add three-pane layout composition
**Task 13**: Integrate with full analysis pipeline (remove SARIF file requirement)
**Task 14**: Add filtering logic
**Task 15**: Add save review state on quit

This plan provides the foundation. Continue with remaining tasks once these are validated.
