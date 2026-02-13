# Vendable Rules Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Externalize pattern rules from hardcoded Go structs into YAML-based rule packs that can be loaded from embedded defaults, user home dir, project dir, or custom paths — using the same tiered-merging pattern as config.

**Architecture:** Introduce a new `internal/rules` package that owns the YAML rule schema, loading, validation, and compilation (string→regexp). The existing `PatternRule` struct moves there. Default rules ship as an embedded YAML file (`rules.yaml`) via `//go:embed`, mirroring how `evaluator` embeds `default.rego`. The tiered config loader gains a rules-loading step. External rule packs are plain YAML files dropped into `~/.config/gavel/rules/` or `.gavel/rules/`.

**Tech Stack:** Go, `gopkg.in/yaml.v3`, `regexp`, `//go:embed`

---

### Task 1: Create `internal/rules` package with YAML schema types

**Files:**
- Create: `internal/rules/rules.go`
- Create: `internal/rules/rules_test.go`

**Step 1: Write the failing test**

Create `internal/rules/rules_test.go`:

```go
package rules

import (
	"testing"
)

func TestRuleFileSchema_Unmarshal(t *testing.T) {
	yamlData := `
rules:
  - id: TEST001
    name: test-rule
    category: security
    pattern: '(?i)password\s*=\s*"[^"]+"'
    languages: [go]
    level: error
    confidence: 0.9
    message: "Test finding"
    explanation: "Detailed explanation"
    remediation: "Fix it"
    source: CWE
    cwe: [CWE-798]
    owasp: [A07:2021]
    references:
      - https://example.com
`
	rf, err := ParseRuleFile([]byte(yamlData))
	if err != nil {
		t.Fatalf("ParseRuleFile: %v", err)
	}
	if len(rf.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rf.Rules))
	}
	r := rf.Rules[0]
	if r.ID != "TEST001" {
		t.Errorf("ID = %q, want TEST001", r.ID)
	}
	if r.Pattern == nil {
		t.Fatal("Pattern should be compiled")
	}
	if !r.Pattern.MatchString(`password = "secret"`) {
		t.Error("Pattern should match")
	}
	if r.Category != CategorySecurity {
		t.Errorf("Category = %q, want security", r.Category)
	}
	if len(r.CWE) != 1 || r.CWE[0] != "CWE-798" {
		t.Errorf("CWE = %v, want [CWE-798]", r.CWE)
	}
}

func TestRuleFileSchema_InvalidRegex(t *testing.T) {
	yamlData := `
rules:
  - id: BAD
    name: bad-regex
    category: security
    pattern: '(?P<invalid'
    level: error
    confidence: 0.5
    message: "bad"
`
	_, err := ParseRuleFile([]byte(yamlData))
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestRuleFileSchema_Validation(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		err  bool
	}{
		{"missing ID", `rules: [{name: x, category: security, pattern: "x", level: error, confidence: 0.5, message: m}]`, true},
		{"missing pattern", `rules: [{id: X, name: x, category: security, level: error, confidence: 0.5, message: m}]`, true},
		{"missing level", `rules: [{id: X, name: x, category: security, pattern: "x", confidence: 0.5, message: m}]`, true},
		{"confidence out of range", `rules: [{id: X, name: x, category: security, pattern: "x", level: error, confidence: 1.5, message: m}]`, true},
		{"valid minimal", `rules: [{id: X, name: x, category: security, pattern: "x", level: error, confidence: 0.5, message: m}]`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRuleFile([]byte(tc.yaml))
			if tc.err && err == nil {
				t.Error("expected error")
			}
			if !tc.err && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/rules/ -v`
Expected: FAIL — package does not exist

**Step 3: Write the implementation**

Create `internal/rules/rules.go`:

```go
package rules

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

type RuleCategory string

const (
	CategorySecurity        RuleCategory = "security"
	CategoryReliability     RuleCategory = "reliability"
	CategoryMaintainability RuleCategory = "maintainability"
)

type RuleSource string

const (
	SourceCWE       RuleSource = "CWE"
	SourceOWASP     RuleSource = "OWASP"
	SourceSonarQube RuleSource = "SonarQube"
	SourceCustom    RuleSource = "Custom"
)

type Rule struct {
	ID          string         `yaml:"id"`
	Name        string         `yaml:"name"`
	Category    RuleCategory   `yaml:"category"`
	Pattern     *regexp.Regexp `yaml:"-"`
	RawPattern  string         `yaml:"pattern"`
	Languages   []string       `yaml:"languages,omitempty"`
	Level       string         `yaml:"level"`
	Confidence  float64        `yaml:"confidence"`
	Message     string         `yaml:"message"`
	Explanation string         `yaml:"explanation,omitempty"`
	Remediation string         `yaml:"remediation,omitempty"`
	Source      RuleSource     `yaml:"source,omitempty"`
	CWE         []string       `yaml:"cwe,omitempty"`
	OWASP       []string       `yaml:"owasp,omitempty"`
	References  []string       `yaml:"references,omitempty"`
}

type RuleFile struct {
	Rules []Rule `yaml:"rules"`
}

func ParseRuleFile(data []byte) (*RuleFile, error) {
	var rf RuleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parsing rule file: %w", err)
	}

	for i := range rf.Rules {
		if err := validateRule(&rf.Rules[i]); err != nil {
			return nil, fmt.Errorf("rule %q: %w", rf.Rules[i].ID, err)
		}
		compiled, err := regexp.Compile(rf.Rules[i].RawPattern)
		if err != nil {
			return nil, fmt.Errorf("rule %q: invalid pattern: %w", rf.Rules[i].ID, err)
		}
		rf.Rules[i].Pattern = compiled
	}

	return &rf, nil
}

func validateRule(r *Rule) error {
	if r.ID == "" {
		return fmt.Errorf("missing id")
	}
	if r.RawPattern == "" {
		return fmt.Errorf("missing pattern")
	}
	if r.Level == "" {
		return fmt.Errorf("missing level")
	}
	if r.Message == "" {
		return fmt.Errorf("missing message")
	}
	if r.Confidence <= 0 || r.Confidence > 1 {
		return fmt.Errorf("confidence must be in (0, 1], got %f", r.Confidence)
	}
	return nil
}

func ByCategory(rules []Rule, category RuleCategory) []Rule {
	var filtered []Rule
	for _, r := range rules {
		if r.Category == category {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func ByCWE(rules []Rule, cweID string) []Rule {
	var filtered []Rule
	for _, r := range rules {
		for _, cwe := range r.CWE {
			if cwe == cweID {
				filtered = append(filtered, r)
				break
			}
		}
	}
	return filtered
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/rules/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/rules/
git commit -m "feat: add internal/rules package with YAML schema and parser"
```

---

### Task 2: Convert hardcoded rules to embedded YAML default

**Files:**
- Create: `internal/rules/default_rules.yaml`
- Create: `internal/rules/embed.go`
- Create: `internal/rules/embed_test.go`

**Step 1: Write the failing test**

Create `internal/rules/embed_test.go`:

```go
package rules

import (
	"testing"
)

func TestDefaultRules_LoadsEmbedded(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatalf("DefaultRules: %v", err)
	}
	if len(rules) < 10 {
		t.Errorf("expected at least 10 default rules, got %d", len(rules))
	}
}

func TestDefaultRules_HasAllCategories(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatal(err)
	}
	categories := map[RuleCategory]int{}
	for _, r := range rules {
		categories[r.Category]++
	}
	if categories[CategorySecurity] == 0 {
		t.Error("no security rules")
	}
	if categories[CategoryReliability] == 0 {
		t.Error("no reliability rules")
	}
	if categories[CategoryMaintainability] == 0 {
		t.Error("no maintainability rules")
	}
}

func TestDefaultRules_UniqueIDs(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range rules {
		if seen[r.ID] {
			t.Errorf("duplicate rule ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestDefaultRules_PatternsCompile(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if r.Pattern == nil {
			t.Errorf("rule %s: Pattern is nil", r.ID)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/rules/ -run TestDefaultRules -v`
Expected: FAIL — `DefaultRules` undefined

**Step 3: Create the default rules YAML**

Create `internal/rules/default_rules.yaml` — transliterate all 16 rules from the current `rules.go` `DefaultRules()` function into YAML format. Each entry has the same fields: `id`, `name`, `category`, `pattern` (as string), `languages`, `level`, `confidence`, `message`, `explanation`, `remediation`, `source`, `cwe`, `owasp`, `references`.

The YAML file should contain the exact same regex patterns and metadata as the current Go code in `internal/analyzer/rules.go` lines 54–361.

**Step 4: Create the embed loader**

Create `internal/rules/embed.go`:

```go
package rules

import (
	_ "embed"
)

//go:embed default_rules.yaml
var defaultRulesYAML []byte

func DefaultRules() ([]Rule, error) {
	rf, err := ParseRuleFile(defaultRulesYAML)
	if err != nil {
		return nil, err
	}
	return rf.Rules, nil
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/rules/ -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/rules/
git commit -m "feat: embed default rules as YAML with go:embed"
```

---

### Task 3: Add tiered rule loading (user home + project dir)

**Files:**
- Create: `internal/rules/loader.go`
- Create: `internal/rules/loader_test.go`

**Step 1: Write the failing test**

Create `internal/rules/loader_test.go`:

```go
package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRules_DefaultsOnly(t *testing.T) {
	rules, err := LoadRules("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 10 {
		t.Errorf("expected at least 10 default rules, got %d", len(rules))
	}
}

func TestLoadRules_ProjectOverride(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, "rules")
	os.MkdirAll(rulesDir, 0o755)

	yamlContent := `rules:
  - id: PROJ001
    name: project-rule
    category: security
    pattern: 'PROJECT_MARKER'
    level: warning
    confidence: 0.9
    message: "Project-specific finding"
`
	os.WriteFile(filepath.Join(rulesDir, "custom.yaml"), []byte(yamlContent), 0o644)

	rules, err := LoadRules("", rulesDir)
	if err != nil {
		t.Fatal(err)
	}

	// Should have defaults + project rule
	found := false
	for _, r := range rules {
		if r.ID == "PROJ001" {
			found = true
		}
	}
	if !found {
		t.Error("project rule PROJ001 not found")
	}
}

func TestLoadRules_UserHomeRules(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `rules:
  - id: HOME001
    name: home-rule
    category: maintainability
    pattern: 'HOME_MARKER'
    level: note
    confidence: 0.8
    message: "User home finding"
`
	os.WriteFile(filepath.Join(dir, "personal.yaml"), []byte(yamlContent), 0o644)

	rules, err := LoadRules(dir, "")
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, r := range rules {
		if r.ID == "HOME001" {
			found = true
		}
	}
	if !found {
		t.Error("home rule HOME001 not found")
	}
}

func TestLoadRules_ProjectOverridesHome(t *testing.T) {
	homeDir := t.TempDir()
	projDir := t.TempDir()

	homeYAML := `rules:
  - id: SHARED001
    name: shared-rule
    category: security
    pattern: 'home_pattern'
    level: warning
    confidence: 0.5
    message: "From home"
`
	projYAML := `rules:
  - id: SHARED001
    name: shared-rule
    category: security
    pattern: 'project_pattern'
    level: error
    confidence: 0.9
    message: "From project"
`
	os.WriteFile(filepath.Join(homeDir, "shared.yaml"), []byte(homeYAML), 0o644)
	os.WriteFile(filepath.Join(projDir, "shared.yaml"), []byte(projYAML), 0o644)

	rules, err := LoadRules(homeDir, projDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range rules {
		if r.ID == "SHARED001" {
			if r.Level != "error" {
				t.Errorf("expected project override level=error, got %s", r.Level)
			}
			if r.Message != "From project" {
				t.Errorf("expected project override message, got %q", r.Message)
			}
			return
		}
	}
	t.Error("SHARED001 not found")
}

func TestLoadRules_MissingDirIsOK(t *testing.T) {
	rules, err := LoadRules("/nonexistent/path", "/another/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	// Should still have defaults
	if len(rules) < 10 {
		t.Errorf("expected defaults, got %d rules", len(rules))
	}
}

func TestLoadRules_InvalidYAMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("{{{{invalid"), 0o644)

	_, err := LoadRules(dir, "")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/rules/ -run TestLoadRules -v`
Expected: FAIL — `LoadRules` undefined

**Step 3: Write the implementation**

Create `internal/rules/loader.go`:

```go
package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadRules(userDir, projectDir string) ([]Rule, error) {
	defaults, err := DefaultRules()
	if err != nil {
		return nil, fmt.Errorf("loading default rules: %w", err)
	}

	rulesByID := make(map[string]Rule, len(defaults))
	for _, r := range defaults {
		rulesByID[r.ID] = r
	}

	// Layer 2: user home rules (lower precedence)
	if userDir != "" {
		userRules, err := loadDir(userDir)
		if err != nil {
			return nil, fmt.Errorf("loading user rules from %s: %w", userDir, err)
		}
		for _, r := range userRules {
			rulesByID[r.ID] = r
		}
	}

	// Layer 3: project rules (highest precedence)
	if projectDir != "" {
		projRules, err := loadDir(projectDir)
		if err != nil {
			return nil, fmt.Errorf("loading project rules from %s: %w", projectDir, err)
		}
		for _, r := range projRules {
			rulesByID[r.ID] = r
		}
	}

	result := make([]Rule, 0, len(rulesByID))
	for _, r := range rulesByID {
		result = append(result, r)
	}
	return result, nil
}

func loadDir(dir string) ([]Rule, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var all []Rule
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		rf, err := ParseRuleFile(data)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		all = append(all, rf.Rules...)
	}
	return all, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/rules/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/rules/
git commit -m "feat: add tiered rule loading from user home and project dirs"
```

---

### Task 4: Wire rules loading into the analyzer and CLI

**Files:**
- Modify: `internal/analyzer/tiered.go`
- Modify: `cmd/gavel/analyze.go`
- Modify: `internal/analyzer/rules.go` (deprecate / redirect)

This task converts consumers of the old `analyzer.PatternRule` / `analyzer.DefaultRules()` to use `rules.Rule` / `rules.LoadRules()` instead.

**Step 1: Update `PatternRule` references in tiered.go**

In `internal/analyzer/tiered.go`, change the `instantPatterns` field type from `[]PatternRule` to `[]rules.Rule`. Update `WithInstantPatterns`, `AddPattern`, `SetPatterns`, `runPatternMatching`, and `NewTieredAnalyzer` to use `rules.Rule`. Update the `defaultPatterns()` function to call `rules.DefaultRules()`.

The import will be: `"github.com/chris-regnier/gavel/internal/rules"`

Key changes:
- `instantPatterns []PatternRule` → `instantPatterns []rules.Rule`
- `func WithInstantPatterns(patterns []PatternRule)` → `func WithInstantPatterns(patterns []rules.Rule)`
- `func (ta *TieredAnalyzer) AddPattern(rule PatternRule)` → `func (ta *TieredAnalyzer) AddPattern(rule rules.Rule)`
- `func (ta *TieredAnalyzer) SetPatterns(rules []PatternRule)` → `func (ta *TieredAnalyzer) SetPatterns(r []rules.Rule)`
- `defaultPatterns()` calls `rules.DefaultRules()` instead of `DefaultRules()`
- `runPatternMatching` iterates `rules.Rule` fields (same field names, just different type)

**Step 2: Add rule loading to analyze.go**

In `cmd/gavel/analyze.go`, after config loading and before analyzer creation, add:

```go
import "github.com/chris-regnier/gavel/internal/rules"

// Load rules (tiered: embedded defaults → user home → project)
userRulesDir := os.ExpandEnv("$HOME/.config/gavel/rules")
projectRulesDir := filepath.Join(flagPolicyDir, "rules")
loadedRules, err := rules.LoadRules(userRulesDir, projectRulesDir)
if err != nil {
    return fmt.Errorf("loading rules: %w", err)
}
```

Then pass `loadedRules` via `WithInstantPatterns(loadedRules)` when constructing the tiered analyzer (or when instant-tier integration is wired up).

**Step 3: Update `internal/analyzer/rules.go`**

Keep the file but make it a thin adapter that re-exports from `internal/rules` for backward compatibility. Replace `DefaultRules()` body with a call to `rules.DefaultRules()` (returning the old `PatternRule` type converted from `rules.Rule`). Or, if no external consumers exist (it's `internal/`), delete the old types and redirect all call sites. Since everything is `internal/`, prefer deleting the old types.

Remove the old types (`PatternRule`, `RuleCategory`, `RuleSource`, constants, `DefaultRules`, filter functions) from `internal/analyzer/rules.go` entirely. They now live in `internal/rules/`.

**Step 4: Update tests**

Update `internal/analyzer/tiered_test.go`:
- Change `PatternRule{...}` to `rules.Rule{...}` in `TestTieredAnalyzer_CustomPatterns`
- Add import for `"github.com/chris-regnier/gavel/internal/rules"`

Delete or move `internal/analyzer/rules_test.go` — its tests are now covered by `internal/rules/embed_test.go` and `internal/rules/rules_test.go`.

**Step 5: Run all tests**

Run: `go test ./... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/analyzer/ cmd/gavel/ internal/rules/
git commit -m "feat: wire vendable rules into analyzer and CLI"
```

---

### Task 5: Add `--rules-dir` CLI flag

**Files:**
- Modify: `cmd/gavel/analyze.go`

**Step 1: Add the flag**

In the `init()` function of `cmd/gavel/analyze.go`, add:

```go
analyzeCmd.Flags().StringVar(&flagRulesDir, "rules-dir", "", "Directory containing custom rule YAML files")
```

Add `flagRulesDir` to the var block.

**Step 2: Update `runAnalyze` to use it**

If `flagRulesDir` is set, use it as the project rules dir (overriding the default `.gavel/rules`):

```go
projectRulesDir := filepath.Join(flagPolicyDir, "rules")
if flagRulesDir != "" {
    projectRulesDir = flagRulesDir
}
```

**Step 3: Run tests**

Run: `go test ./... -v`
Expected: PASS

**Step 4: Build**

Run: `task build`
Expected: SUCCESS

**Step 5: Commit**

```bash
git add cmd/gavel/analyze.go
git commit -m "feat: add --rules-dir CLI flag for custom rule packs"
```

---

### Task 6: Update documentation

**Files:**
- Modify: `README.md` — add section on custom rules
- Modify: `CLAUDE.md` — add rules architecture notes
- Create: `example-rules.yaml` — example custom rule pack

**Step 1: Create example-rules.yaml**

Create an example file at the repo root showing how to write a custom rule pack with 2-3 example rules (one per category).

**Step 2: Update README.md**

Add a "Custom Rules" section explaining:
- Default rules ship embedded (CWE/OWASP/SonarQube based)
- Users can add rules in `~/.config/gavel/rules/*.yaml` (personal) or `.gavel/rules/*.yaml` (project)
- Project rules override home rules; home rules override defaults (by ID)
- `--rules-dir` flag for one-off overrides
- Link to `example-rules.yaml`

**Step 3: Update CLAUDE.md**

Add rules loading to the Architecture and Key Design Decisions sections.

**Step 4: Commit**

```bash
git add README.md CLAUDE.md example-rules.yaml
git commit -m "docs: add custom rules documentation and examples"
```

---

### Task 7: Final verification

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 2: Run lint**

Run: `go vet ./...`
Expected: No errors

**Step 3: Build**

Run: `task build`
Expected: SUCCESS

**Step 4: Verify embedded rules load**

Run: `./gavel analyze --dir ./internal/rules 2>&1 | head -20`
Expected: Runs without rule-loading errors (may fail on LLM connection, that's OK — we're verifying rule loading)
