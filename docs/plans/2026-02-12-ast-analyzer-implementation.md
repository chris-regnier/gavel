# AST Analyzer Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add tree-sitter-based AST analysis as a new rule type alongside existing regex rules, enabling structural code checks (function length, nesting depth, empty error handlers, parameter counts) within the instant analysis tier.

**Architecture:** Extend the `Rule` struct with a `Type` field (`regex` | `ast`). Add an `internal/astcheck` package with a `Check` interface and language grammar registry. Integrate into the existing `TieredAnalyzer.runPatternMatching()` so AST checks run alongside regex in the instant tier.

**Tech Stack:** `smacker/go-tree-sitter` (Go bindings for tree-sitter), tree-sitter language grammars (Go, Python, JS, TS, Java, C, Rust)

**Design doc:** `docs/plans/2026-02-12-ast-analyzer-design.md`

---

### Task 1: Add go-tree-sitter Dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add the dependency**

Run:
```bash
cd /Users/chris-regnier/code/gavel && go get github.com/smacker/go-tree-sitter
go get github.com/smacker/go-tree-sitter/golang
go get github.com/smacker/go-tree-sitter/python
go get github.com/smacker/go-tree-sitter/javascript
go get github.com/smacker/go-tree-sitter/typescript
go get github.com/smacker/go-tree-sitter/java
go get github.com/smacker/go-tree-sitter/c
go get github.com/smacker/go-tree-sitter/rust
```

Expected: go.mod updated with tree-sitter dependencies

**Step 2: Verify the build still works**

Run: `task build`
Expected: Compiles without error

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add go-tree-sitter dependency for AST analysis"
```

---

### Task 2: Extend Rule Struct with Type Field

**Files:**
- Modify: `internal/rules/rules.go` (lines 10-43, 49-93)
- Test: `internal/rules/rules_test.go`

**Step 1: Write the failing tests**

Add to `internal/rules/rules_test.go`:

```go
func TestParseRuleFile_ASTRule(t *testing.T) {
	yaml := `rules:
  - id: "AST001"
    name: "function-length"
    type: ast
    category: "maintainability"
    ast_check: "function-length"
    ast_config:
      max_lines: 50
    level: "note"
    confidence: 1.0
    message: "Function exceeds maximum length"
`
	rf, err := ParseRuleFile([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rf.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rf.Rules))
	}
	r := rf.Rules[0]
	if r.Type != RuleTypeAST {
		t.Errorf("expected type ast, got %s", r.Type)
	}
	if r.ASTCheck != "function-length" {
		t.Errorf("expected ast_check function-length, got %s", r.ASTCheck)
	}
	if r.ASTConfig == nil {
		t.Fatal("expected ast_config to be populated")
	}
	maxLines, ok := r.ASTConfig["max_lines"]
	if !ok {
		t.Fatal("expected max_lines in ast_config")
	}
	// YAML numbers unmarshal as int
	if v, ok := maxLines.(int); !ok || v != 50 {
		t.Errorf("expected max_lines=50, got %v", maxLines)
	}
	// Pattern should be nil for AST rules
	if r.Pattern != nil {
		t.Error("expected nil pattern for AST rule")
	}
}

func TestParseRuleFile_ASTRuleMissingCheck(t *testing.T) {
	yaml := `rules:
  - id: "BAD"
    type: ast
    level: "error"
    confidence: 0.5
    message: "missing ast_check"
`
	_, err := ParseRuleFile([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for AST rule without ast_check")
	}
	if !strings.Contains(err.Error(), "ast_check") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseRuleFile_RegexRuleDefault(t *testing.T) {
	yaml := `rules:
  - id: "R001"
    pattern: 'foo'
    level: "warning"
    confidence: 0.5
    message: "found foo"
`
	rf, err := ParseRuleFile([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Type should default to regex
	if rf.Rules[0].Type != RuleTypeRegex {
		t.Errorf("expected default type regex, got %s", rf.Rules[0].Type)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/rules/ -run "TestParseRuleFile_AST|TestParseRuleFile_RegexRuleDefault" -v`
Expected: FAIL — `RuleTypeAST` undefined, `ASTCheck` field doesn't exist

**Step 3: Implement the type extension**

Modify `internal/rules/rules.go`:

1. Add `RuleType` constants after the `RuleSource` block (after line 25):

```go
type RuleType string

const (
	RuleTypeRegex RuleType = "regex"
	RuleTypeAST   RuleType = "ast"
)
```

2. Add fields to `Rule` struct (after line 32, the `RawPattern` field):

```go
	Type       RuleType               `yaml:"type,omitempty"`
	ASTCheck   string                 `yaml:"ast_check,omitempty"`
	ASTConfig  map[string]interface{} `yaml:"ast_config,omitempty"`
```

3. Update `ParseRuleFile` to handle types. Replace the validation and regex compilation block (lines 56-71) with:

```go
	for i := range rf.Rules {
		r := &rf.Rules[i]
		// Default type to regex
		if r.Type == "" {
			r.Type = RuleTypeRegex
		}

		if err := validateRule(r); err != nil {
			return nil, fmt.Errorf("rule %q (index %d): %w", r.ID, i, err)
		}
		if seen[r.ID] {
			return nil, fmt.Errorf("duplicate rule ID %q", r.ID)
		}
		seen[r.ID] = true

		// Only compile regex for regex-type rules
		if r.Type == RuleTypeRegex {
			compiled, err := regexp.Compile(r.RawPattern)
			if err != nil {
				return nil, fmt.Errorf("rule %q: invalid regex pattern: %w", r.ID, err)
			}
			r.Pattern = compiled
		}
	}
```

4. Update `validateRule` to handle type-specific validation:

```go
func validateRule(r *Rule) error {
	if r.ID == "" {
		return fmt.Errorf("missing required field: id")
	}
	if r.Level == "" {
		return fmt.Errorf("missing required field: level")
	}
	if r.Message == "" {
		return fmt.Errorf("missing required field: message")
	}
	if r.Confidence <= 0 || r.Confidence > 1 {
		return fmt.Errorf("confidence must be in range (0, 1], got %v", r.Confidence)
	}

	switch r.Type {
	case RuleTypeRegex:
		if r.RawPattern == "" {
			return fmt.Errorf("missing required field: pattern")
		}
	case RuleTypeAST:
		if r.ASTCheck == "" {
			return fmt.Errorf("missing required field: ast_check")
		}
	default:
		return fmt.Errorf("unknown rule type: %s", r.Type)
	}

	return nil
}
```

**Step 4: Run all rules tests**

Run: `go test ./internal/rules/ -v`
Expected: ALL PASS (new and existing tests)

**Step 5: Commit**

```bash
git add internal/rules/rules.go internal/rules/rules_test.go
git commit -m "feat: extend Rule struct with type field for ast/regex dispatch"
```

---

### Task 3: AST Check Registry and Language Detection

**Files:**
- Create: `internal/astcheck/registry.go`
- Create: `internal/astcheck/language.go`
- Create: `internal/astcheck/registry_test.go`
- Create: `internal/astcheck/language_test.go`

**Step 1: Write the failing tests for registry**

Create `internal/astcheck/registry_test.go`:

```go
package astcheck

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
)

type stubCheck struct {
	name    string
	matches []Match
}

func (s *stubCheck) Name() string { return s.name }
func (s *stubCheck) Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match {
	return s.matches
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	check := &stubCheck{name: "test-check", matches: []Match{{StartLine: 1, EndLine: 2}}}

	r.Register(check)

	got, ok := r.Get("test-check")
	if !ok {
		t.Fatal("expected to find registered check")
	}
	if got.Name() != "test-check" {
		t.Errorf("expected name test-check, got %s", got.Name())
	}
}

func TestRegistry_LookupMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected lookup of nonexistent check to return false")
	}
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubCheck{name: "a"})
	r.Register(&stubCheck{name: "b"})

	names := r.Names()
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}
```

**Step 2: Write the failing tests for language detection**

Create `internal/astcheck/language_test.go`:

```go
package astcheck

import "testing"

func TestLanguageRegistry_DetectGo(t *testing.T) {
	lr := NewLanguageRegistry()
	lang, name, ok := lr.Detect("main.go")
	if !ok {
		t.Fatal("expected to detect Go")
	}
	if name != "go" {
		t.Errorf("expected lang name 'go', got %s", name)
	}
	if lang == nil {
		t.Fatal("expected non-nil language")
	}
}

func TestLanguageRegistry_DetectPython(t *testing.T) {
	lr := NewLanguageRegistry()
	_, name, ok := lr.Detect("script.py")
	if !ok {
		t.Fatal("expected to detect Python")
	}
	if name != "python" {
		t.Errorf("expected 'python', got %s", name)
	}
}

func TestLanguageRegistry_DetectJavaScript(t *testing.T) {
	lr := NewLanguageRegistry()
	for _, ext := range []string{"app.js", "component.jsx"} {
		_, name, ok := lr.Detect(ext)
		if !ok {
			t.Fatalf("expected to detect JS for %s", ext)
		}
		if name != "javascript" {
			t.Errorf("expected 'javascript' for %s, got %s", ext, name)
		}
	}
}

func TestLanguageRegistry_DetectTypeScript(t *testing.T) {
	lr := NewLanguageRegistry()
	for _, ext := range []string{"app.ts", "component.tsx"} {
		_, name, ok := lr.Detect(ext)
		if !ok {
			t.Fatalf("expected to detect TS for %s", ext)
		}
		if name != "typescript" {
			t.Errorf("expected 'typescript' for %s, got %s", ext, name)
		}
	}
}

func TestLanguageRegistry_DetectUnknown(t *testing.T) {
	lr := NewLanguageRegistry()
	_, _, ok := lr.Detect("data.csv")
	if ok {
		t.Error("expected unknown extension to return false")
	}
}

func TestLanguageRegistry_AllSupportedLanguages(t *testing.T) {
	lr := NewLanguageRegistry()
	cases := []struct {
		file string
		lang string
	}{
		{"main.go", "go"},
		{"main.py", "python"},
		{"main.js", "javascript"},
		{"main.ts", "typescript"},
		{"Main.java", "java"},
		{"main.c", "c"},
		{"main.rs", "rust"},
	}
	for _, tc := range cases {
		_, name, ok := lr.Detect(tc.file)
		if !ok {
			t.Errorf("expected to detect %s for %s", tc.lang, tc.file)
			continue
		}
		if name != tc.lang {
			t.Errorf("for %s: expected %s, got %s", tc.file, tc.lang, name)
		}
	}
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/astcheck/ -v`
Expected: FAIL — package doesn't exist

**Step 4: Implement registry**

Create `internal/astcheck/registry.go`:

```go
package astcheck

import (
	"sort"

	sitter "github.com/smacker/go-tree-sitter"
)

// Check performs AST-based analysis on a parsed tree.
type Check interface {
	// Name returns the check's identifier, matching the ast_check field in rules.
	Name() string
	// Run analyzes a parsed tree and returns matches.
	Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match
}

// Match represents a single finding from an AST check.
type Match struct {
	StartLine int
	EndLine   int
	Message   string                 // optional override of rule message
	Extra     map[string]interface{} // e.g., {"actual_lines": 72, "function_name": "foo"}
}

// Registry holds available AST checks.
type Registry struct {
	checks map[string]Check
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{checks: make(map[string]Check)}
}

// Register adds a check to the registry.
func (r *Registry) Register(c Check) {
	r.checks[c.Name()] = c
}

// Get looks up a check by name.
func (r *Registry) Get(name string) (Check, bool) {
	c, ok := r.checks[name]
	return c, ok
}

// Names returns all registered check names, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.checks))
	for name := range r.checks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
```

**Step 5: Implement language detection**

Create `internal/astcheck/language.go`:

```go
package astcheck

import (
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// LanguageRegistry maps file extensions to tree-sitter grammars.
type LanguageRegistry struct {
	// ext → (language, name)
	byExt map[string]langEntry
}

type langEntry struct {
	lang *sitter.Language
	name string
}

// NewLanguageRegistry creates a registry with all supported languages.
func NewLanguageRegistry() *LanguageRegistry {
	lr := &LanguageRegistry{byExt: make(map[string]langEntry)}

	goLang := langEntry{lang: golang.GetLanguage(), name: "go"}
	lr.byExt[".go"] = goLang

	pyLang := langEntry{lang: python.GetLanguage(), name: "python"}
	lr.byExt[".py"] = pyLang

	jsLang := langEntry{lang: javascript.GetLanguage(), name: "javascript"}
	lr.byExt[".js"] = jsLang
	lr.byExt[".jsx"] = jsLang

	tsLang := langEntry{lang: typescript.GetLanguage(), name: "typescript"}
	lr.byExt[".ts"] = tsLang
	lr.byExt[".tsx"] = tsLang

	javaLang := langEntry{lang: java.GetLanguage(), name: "java"}
	lr.byExt[".java"] = javaLang

	cLang := langEntry{lang: c.GetLanguage(), name: "c"}
	lr.byExt[".c"] = cLang
	lr.byExt[".h"] = cLang

	rustLang := langEntry{lang: rust.GetLanguage(), name: "rust"}
	lr.byExt[".rs"] = rustLang

	return lr
}

// Detect returns the tree-sitter language and name for a file path.
// Returns false if the language is not supported.
func (lr *LanguageRegistry) Detect(path string) (*sitter.Language, string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	entry, ok := lr.byExt[ext]
	if !ok {
		return nil, "", false
	}
	return entry.lang, entry.name, true
}
```

**Step 6: Run all astcheck tests**

Run: `go test ./internal/astcheck/ -v`
Expected: ALL PASS

**Step 7: Commit**

```bash
git add internal/astcheck/
git commit -m "feat: add AST check registry and language detection"
```

---

### Task 4: Function Length Check (AST001)

**Files:**
- Create: `internal/astcheck/function_length.go`
- Create: `internal/astcheck/function_length_test.go`

**Step 1: Write the failing test**

Create `internal/astcheck/function_length_test.go`:

```go
package astcheck

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
)

func parseGo(t *testing.T, source string) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	return tree
}

func TestFunctionLength_ShortFunction(t *testing.T) {
	source := `package main

func short() {
	x := 1
	y := 2
	_ = x + y
}
`
	tree := parseGo(t, source)
	check := &FunctionLength{}
	matches := check.Run(tree, []byte(source), "go", map[string]interface{}{"max_lines": 50})
	if len(matches) != 0 {
		t.Errorf("expected no matches for short function, got %d", len(matches))
	}
}

func TestFunctionLength_LongFunction(t *testing.T) {
	// Build a function with 60 lines
	source := "package main\n\nfunc longFunc() {\n"
	for i := 0; i < 57; i++ {
		source += "\tx := 1\n"
	}
	source += "}\n"

	tree := parseGo(t, source)
	check := &FunctionLength{}
	matches := check.Run(tree, []byte(source), "go", map[string]interface{}{"max_lines": 50})
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for long function, got %d", len(matches))
	}
	if matches[0].StartLine < 3 {
		t.Errorf("expected start line >= 3, got %d", matches[0].StartLine)
	}
}

func TestFunctionLength_CustomThreshold(t *testing.T) {
	source := `package main

func medium() {
	a := 1
	b := 2
	c := 3
	d := 4
	e := 5
}
`
	tree := parseGo(t, source)
	check := &FunctionLength{}
	// Set threshold to 3 lines — should trigger
	matches := check.Run(tree, []byte(source), "go", map[string]interface{}{"max_lines": 3})
	if len(matches) != 1 {
		t.Errorf("expected 1 match with threshold 3, got %d", len(matches))
	}
}

func TestFunctionLength_DefaultThreshold(t *testing.T) {
	check := &FunctionLength{}
	// nil config should use default (50)
	source := `package main

func short() {
	x := 1
}
`
	tree := parseGo(t, source)
	matches := check.Run(tree, []byte(source), "go", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches with default threshold, got %d", len(matches))
	}
}

func TestFunctionLength_MultipleFunctions(t *testing.T) {
	source := "package main\n\nfunc short() {\n\tx := 1\n}\n\nfunc longFunc() {\n"
	for i := 0; i < 12; i++ {
		source += "\tx := 1\n"
	}
	source += "}\n"

	tree := parseGo(t, source)
	check := &FunctionLength{}
	matches := check.Run(tree, []byte(source), "go", map[string]interface{}{"max_lines": 10})
	// Only longFunc should match
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFunctionLength_Name(t *testing.T) {
	check := &FunctionLength{}
	if check.Name() != "function-length" {
		t.Errorf("expected name 'function-length', got %s", check.Name())
	}
}
```

**Step 2: Run to verify failure**

Run: `go test ./internal/astcheck/ -run TestFunctionLength -v`
Expected: FAIL — `FunctionLength` undefined

**Step 3: Implement function length check**

Create `internal/astcheck/function_length.go`:

```go
package astcheck

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
)

// FunctionLength checks for functions exceeding a maximum line count.
type FunctionLength struct{}

func (f *FunctionLength) Name() string { return "function-length" }

func (f *FunctionLength) Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match {
	maxLines := 50
	if config != nil {
		if v, ok := config["max_lines"]; ok {
			switch n := v.(type) {
			case int:
				maxLines = n
			case float64:
				maxLines = int(n)
			}
		}
	}

	var matches []Match
	root := tree.RootNode()

	nodeTypes := functionNodeTypes(lang)
	findNodes(root, nodeTypes, func(node *sitter.Node) {
		startLine := int(node.StartPoint().Row) + 1 // 0-indexed → 1-indexed
		endLine := int(node.EndPoint().Row) + 1
		lineCount := endLine - startLine + 1

		if lineCount > maxLines {
			name := functionName(node, lang)
			matches = append(matches, Match{
				StartLine: startLine,
				EndLine:   endLine,
				Message:   fmt.Sprintf("Function %s has %d lines (max %d)", name, lineCount, maxLines),
				Extra: map[string]interface{}{
					"actual_lines":  lineCount,
					"function_name": name,
				},
			})
		}
	})

	return matches
}

// functionNodeTypes returns the tree-sitter node types for functions/methods per language.
func functionNodeTypes(lang string) []string {
	switch lang {
	case "go":
		return []string{"function_declaration", "method_declaration"}
	case "python":
		return []string{"function_definition"}
	case "javascript", "typescript":
		return []string{"function_declaration", "method_definition", "arrow_function"}
	case "java":
		return []string{"method_declaration", "constructor_declaration"}
	case "c":
		return []string{"function_definition"}
	case "rust":
		return []string{"function_item"}
	default:
		return nil
	}
}

// functionName extracts the function name from a node.
func functionName(node *sitter.Node, lang string) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return nameNode.Content(nil) // Will use the source in the tree
	}
	return "<anonymous>"
}

// findNodes does a DFS to find all nodes matching the given types.
func findNodes(node *sitter.Node, types []string, fn func(*sitter.Node)) {
	if node == nil {
		return
	}
	for _, t := range types {
		if node.Type() == t {
			fn(node)
			break
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		findNodes(node.Child(i), types, fn)
	}
}
```

Note: `functionName` uses `node.ChildByFieldName("name")` — tree-sitter nodes have named fields. The `name` field exists on function declarations in Go, Python, JS, Java, C, and Rust. For anonymous functions (arrow functions), it returns `<anonymous>`. The `Content(nil)` call needs the source bytes passed in — adjust if needed based on the actual tree-sitter API behavior. If `Content(nil)` doesn't work, you'll need to slice the source bytes using `node.StartByte()` and `node.EndByte()`.

**Step 4: Run tests**

Run: `go test ./internal/astcheck/ -run TestFunctionLength -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/astcheck/function_length.go internal/astcheck/function_length_test.go
git commit -m "feat: add function-length AST check"
```

---

### Task 5: Nesting Depth Check (AST002)

**Files:**
- Create: `internal/astcheck/nesting_depth.go`
- Create: `internal/astcheck/nesting_depth_test.go`

**Step 1: Write the failing test**

Create `internal/astcheck/nesting_depth_test.go`:

```go
package astcheck

import (
	"testing"
)

func TestNestingDepth_Shallow(t *testing.T) {
	source := `package main

func foo() {
	if true {
		x := 1
		_ = x
	}
}
`
	tree := parseGo(t, source)
	check := &NestingDepth{}
	matches := check.Run(tree, []byte(source), "go", map[string]interface{}{"max_depth": 4})
	if len(matches) != 0 {
		t.Errorf("expected no matches for shallow nesting, got %d", len(matches))
	}
}

func TestNestingDepth_Deep(t *testing.T) {
	source := `package main

func foo() {
	if true {
		for i := 0; i < 10; i++ {
			if i > 5 {
				for j := 0; j < 10; j++ {
					if j > 5 {
						x := 1
						_ = x
					}
				}
			}
		}
	}
}
`
	tree := parseGo(t, source)
	check := &NestingDepth{}
	matches := check.Run(tree, []byte(source), "go", map[string]interface{}{"max_depth": 4})
	if len(matches) == 0 {
		t.Error("expected at least 1 match for deeply nested code")
	}
}

func TestNestingDepth_DefaultThreshold(t *testing.T) {
	source := `package main

func foo() {
	if true {
		x := 1
		_ = x
	}
}
`
	tree := parseGo(t, source)
	check := &NestingDepth{}
	matches := check.Run(tree, []byte(source), "go", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches with default threshold, got %d", len(matches))
	}
}

func TestNestingDepth_Name(t *testing.T) {
	check := &NestingDepth{}
	if check.Name() != "nesting-depth" {
		t.Errorf("expected name 'nesting-depth', got %s", check.Name())
	}
}
```

**Step 2: Run to verify failure**

Run: `go test ./internal/astcheck/ -run TestNestingDepth -v`
Expected: FAIL — `NestingDepth` undefined

**Step 3: Implement**

Create `internal/astcheck/nesting_depth.go`:

```go
package astcheck

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
)

// NestingDepth checks for deeply nested code blocks.
type NestingDepth struct{}

func (n *NestingDepth) Name() string { return "nesting-depth" }

func (n *NestingDepth) Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match {
	maxDepth := 4
	if config != nil {
		if v, ok := config["max_depth"]; ok {
			switch d := v.(type) {
			case int:
				maxDepth = d
			case float64:
				maxDepth = int(d)
			}
		}
	}

	nestingTypes := nestingNodeTypes(lang)
	var matches []Match
	walkNesting(tree.RootNode(), nestingTypes, 0, maxDepth, source, &matches)
	return matches
}

// nestingNodeTypes returns node types that increase nesting depth per language.
func nestingNodeTypes(lang string) map[string]bool {
	types := map[string]bool{}
	switch lang {
	case "go":
		for _, t := range []string{"if_statement", "for_statement", "switch_statement", "select_statement"} {
			types[t] = true
		}
	case "python":
		for _, t := range []string{"if_statement", "for_statement", "while_statement", "with_statement"} {
			types[t] = true
		}
	case "javascript", "typescript":
		for _, t := range []string{"if_statement", "for_statement", "for_in_statement", "while_statement", "switch_statement"} {
			types[t] = true
		}
	case "java":
		for _, t := range []string{"if_statement", "for_statement", "enhanced_for_statement", "while_statement", "switch_expression"} {
			types[t] = true
		}
	}
	return types
}

func walkNesting(node *sitter.Node, nestingTypes map[string]bool, depth, maxDepth int, source []byte, matches *[]Match) {
	if node == nil {
		return
	}

	currentDepth := depth
	if nestingTypes[node.Type()] {
		currentDepth++
		if currentDepth > maxDepth {
			startLine := int(node.StartPoint().Row) + 1
			endLine := int(node.EndPoint().Row) + 1
			*matches = append(*matches, Match{
				StartLine: startLine,
				EndLine:   endLine,
				Message:   fmt.Sprintf("Nesting depth %d exceeds maximum %d", currentDepth, maxDepth),
				Extra:     map[string]interface{}{"actual_depth": currentDepth},
			})
			return // Don't recurse further — already flagged
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkNesting(node.Child(i), nestingTypes, currentDepth, maxDepth, source, matches)
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/astcheck/ -run TestNestingDepth -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/astcheck/nesting_depth.go internal/astcheck/nesting_depth_test.go
git commit -m "feat: add nesting-depth AST check"
```

---

### Task 6: Empty Error Handler Check (AST003)

**Files:**
- Create: `internal/astcheck/empty_handler.go`
- Create: `internal/astcheck/empty_handler_test.go`

**Step 1: Write the failing test**

Create `internal/astcheck/empty_handler_test.go`:

```go
package astcheck

import (
	"testing"
)

func TestEmptyHandler_GoEmptyErrorBlock(t *testing.T) {
	source := `package main

func foo() error {
	err := doSomething()
	if err != nil {
	}
	return nil
}
`
	tree := parseGo(t, source)
	check := &EmptyHandler{}
	matches := check.Run(tree, []byte(source), "go", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for empty error block, got %d", len(matches))
	}
}

func TestEmptyHandler_GoNonEmptyErrorBlock(t *testing.T) {
	source := `package main

func foo() error {
	err := doSomething()
	if err != nil {
		return err
	}
	return nil
}
`
	tree := parseGo(t, source)
	check := &EmptyHandler{}
	matches := check.Run(tree, []byte(source), "go", nil)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for handled error, got %d", len(matches))
	}
}

func TestEmptyHandler_Name(t *testing.T) {
	check := &EmptyHandler{}
	if check.Name() != "empty-handler" {
		t.Errorf("expected name 'empty-handler', got %s", check.Name())
	}
}
```

**Step 2: Run to verify failure**

Run: `go test ./internal/astcheck/ -run TestEmptyHandler -v`
Expected: FAIL

**Step 3: Implement**

Create `internal/astcheck/empty_handler.go`:

```go
package astcheck

import (
	sitter "github.com/smacker/go-tree-sitter"
)

// EmptyHandler checks for empty error handling blocks.
type EmptyHandler struct{}

func (e *EmptyHandler) Name() string { return "empty-handler" }

func (e *EmptyHandler) Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match {
	switch lang {
	case "go":
		return e.checkGo(tree, source)
	case "python":
		return e.checkPython(tree, source)
	case "javascript", "typescript":
		return e.checkJS(tree, source)
	case "java":
		return e.checkJava(tree, source)
	default:
		return nil
	}
}

func (e *EmptyHandler) checkGo(tree *sitter.Tree, source []byte) []Match {
	var matches []Match
	// Find if_statement nodes where condition involves err != nil and body is empty block
	findNodes(tree.RootNode(), []string{"if_statement"}, func(node *sitter.Node) {
		// Check if this is an error check: condition contains "err != nil" or "err == nil"
		condition := node.ChildByFieldName("condition")
		if condition == nil {
			return
		}
		condText := string(source[condition.StartByte():condition.EndByte()])
		if condText != "err != nil" {
			return
		}

		// Check if the consequence (body) is an empty block
		body := node.ChildByFieldName("consequence")
		if body == nil {
			return
		}
		// Empty block: only contains { and } — no named children
		if body.NamedChildCount() == 0 {
			matches = append(matches, Match{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Message:   "Empty error handling block (if err != nil {})",
			})
		}
	})
	return matches
}

func (e *EmptyHandler) checkPython(tree *sitter.Tree, source []byte) []Match {
	var matches []Match
	findNodes(tree.RootNode(), []string{"except_clause"}, func(node *sitter.Node) {
		// Check if the body only contains "pass"
		body := node.ChildByFieldName("body")
		if body == nil {
			// Fallback: look for block child
			for i := 0; i < int(node.NamedChildCount()); i++ {
				child := node.NamedChild(i)
				if child.Type() == "block" {
					body = child
					break
				}
			}
		}
		if body == nil {
			return
		}
		if body.NamedChildCount() == 1 {
			first := body.NamedChild(0)
			if first.Type() == "pass_statement" {
				matches = append(matches, Match{
					StartLine: int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
					Message:   "Empty except block (only contains pass)",
				})
			}
		}
	})
	return matches
}

func (e *EmptyHandler) checkJS(tree *sitter.Tree, source []byte) []Match {
	var matches []Match
	findNodes(tree.RootNode(), []string{"catch_clause"}, func(node *sitter.Node) {
		body := node.ChildByFieldName("body")
		if body == nil {
			return
		}
		if body.NamedChildCount() == 0 {
			matches = append(matches, Match{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Message:   "Empty catch block",
			})
		}
	})
	return matches
}

func (e *EmptyHandler) checkJava(tree *sitter.Tree, source []byte) []Match {
	// Java catch_clause works the same as JS
	return e.checkJS(tree, source)
}
```

**Step 4: Run tests**

Run: `go test ./internal/astcheck/ -run TestEmptyHandler -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/astcheck/empty_handler.go internal/astcheck/empty_handler_test.go
git commit -m "feat: add empty-handler AST check"
```

---

### Task 7: Parameter Count Check (AST004)

**Files:**
- Create: `internal/astcheck/param_count.go`
- Create: `internal/astcheck/param_count_test.go`

**Step 1: Write the failing test**

Create `internal/astcheck/param_count_test.go`:

```go
package astcheck

import (
	"testing"
)

func TestParamCount_FewParams(t *testing.T) {
	source := `package main

func foo(a int, b string) {}
`
	tree := parseGo(t, source)
	check := &ParamCount{}
	matches := check.Run(tree, []byte(source), "go", map[string]interface{}{"max_params": 5})
	if len(matches) != 0 {
		t.Errorf("expected no matches for few params, got %d", len(matches))
	}
}

func TestParamCount_TooManyParams(t *testing.T) {
	source := `package main

func tooMany(a int, b int, c int, d int, e int, f int) {}
`
	tree := parseGo(t, source)
	check := &ParamCount{}
	matches := check.Run(tree, []byte(source), "go", map[string]interface{}{"max_params": 5})
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for 6 params, got %d", len(matches))
	}
}

func TestParamCount_ExactlyMax(t *testing.T) {
	source := `package main

func exactly(a int, b int, c int, d int, e int) {}
`
	tree := parseGo(t, source)
	check := &ParamCount{}
	matches := check.Run(tree, []byte(source), "go", map[string]interface{}{"max_params": 5})
	if len(matches) != 0 {
		t.Errorf("expected no matches for exactly max params, got %d", len(matches))
	}
}

func TestParamCount_Name(t *testing.T) {
	check := &ParamCount{}
	if check.Name() != "param-count" {
		t.Errorf("expected name 'param-count', got %s", check.Name())
	}
}
```

**Step 2: Run to verify failure**

Run: `go test ./internal/astcheck/ -run TestParamCount -v`
Expected: FAIL

**Step 3: Implement**

Create `internal/astcheck/param_count.go`:

```go
package astcheck

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
)

// ParamCount checks for functions with too many parameters.
type ParamCount struct{}

func (p *ParamCount) Name() string { return "param-count" }

func (p *ParamCount) Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match {
	maxParams := 5
	if config != nil {
		if v, ok := config["max_params"]; ok {
			switch n := v.(type) {
			case int:
				maxParams = n
			case float64:
				maxParams = int(n)
			}
		}
	}

	var matches []Match
	nodeTypes := functionNodeTypes(lang)

	findNodes(tree.RootNode(), nodeTypes, func(node *sitter.Node) {
		params := node.ChildByFieldName("parameters")
		if params == nil {
			return
		}

		count := countParams(params, lang)
		if count > maxParams {
			name := functionName(node, lang)
			matches = append(matches, Match{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.StartPoint().Row) + 1,
				Message:   fmt.Sprintf("Function %s has %d parameters (max %d)", name, count, maxParams),
				Extra: map[string]interface{}{
					"actual_params": count,
					"function_name": name,
				},
			})
		}
	})

	return matches
}

// countParams counts the number of parameters in a parameter list node.
func countParams(params *sitter.Node, lang string) int {
	count := 0
	paramTypes := paramNodeTypes(lang)
	for i := 0; i < int(params.NamedChildCount()); i++ {
		child := params.NamedChild(i)
		for _, pt := range paramTypes {
			if child.Type() == pt {
				count++
				break
			}
		}
	}
	return count
}

// paramNodeTypes returns the tree-sitter node types for parameters per language.
func paramNodeTypes(lang string) []string {
	switch lang {
	case "go":
		return []string{"parameter_declaration"}
	case "python":
		return []string{"identifier", "default_parameter", "typed_parameter", "typed_default_parameter"}
	case "javascript", "typescript":
		return []string{"identifier", "assignment_pattern", "rest_pattern", "required_parameter", "optional_parameter"}
	case "java":
		return []string{"formal_parameter", "spread_parameter"}
	case "c":
		return []string{"parameter_declaration"}
	case "rust":
		return []string{"parameter"}
	default:
		return nil
	}
}
```

Note: Go's `parameter_declaration` groups params of the same type (e.g., `a, b int` is one `parameter_declaration` with 2 names). You may need to count the individual identifiers within each `parameter_declaration` for Go. Adjust counting logic during implementation if tests reveal this.

**Step 4: Run tests**

Run: `go test ./internal/astcheck/ -run TestParamCount -v`
Expected: ALL PASS (may need adjustment for Go's grouped params)

**Step 5: Commit**

```bash
git add internal/astcheck/param_count.go internal/astcheck/param_count_test.go
git commit -m "feat: add param-count AST check"
```

---

### Task 8: Default AST Check Registration

**Files:**
- Create: `internal/astcheck/defaults.go`
- Create: `internal/astcheck/defaults_test.go`

**Step 1: Write the failing test**

Create `internal/astcheck/defaults_test.go`:

```go
package astcheck

import "testing"

func TestDefaultRegistry_AllChecksRegistered(t *testing.T) {
	r := DefaultRegistry()
	expected := []string{"empty-handler", "function-length", "nesting-depth", "param-count"}
	for _, name := range expected {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected check %q to be registered", name)
		}
	}
}
```

**Step 2: Run to verify failure**

Run: `go test ./internal/astcheck/ -run TestDefaultRegistry -v`
Expected: FAIL

**Step 3: Implement**

Create `internal/astcheck/defaults.go`:

```go
package astcheck

// DefaultRegistry returns a registry with all built-in AST checks.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&FunctionLength{})
	r.Register(&NestingDepth{})
	r.Register(&EmptyHandler{})
	r.Register(&ParamCount{})
	return r
}
```

**Step 4: Run tests**

Run: `go test ./internal/astcheck/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/astcheck/defaults.go internal/astcheck/defaults_test.go
git commit -m "feat: add default AST check registry"
```

---

### Task 9: Add AST Rules to default_rules.yaml

**Files:**
- Modify: `internal/rules/default_rules.yaml`
- Modify: `internal/rules/embed_test.go`

**Step 1: Write/update the failing test**

Add to `internal/rules/embed_test.go`:

```go
func TestDefaultRules_ContainsASTRules(t *testing.T) {
	rules, err := DefaultRules()
	if err != nil {
		t.Fatalf("failed to load defaults: %v", err)
	}
	astCount := 0
	for _, r := range rules {
		if r.Type == RuleTypeAST {
			astCount++
		}
	}
	if astCount < 4 {
		t.Errorf("expected at least 4 AST rules, got %d", astCount)
	}
}
```

**Step 2: Run to verify failure**

Run: `go test ./internal/rules/ -run TestDefaultRules_ContainsAST -v`
Expected: FAIL — no AST rules in defaults yet

**Step 3: Append AST rules to default_rules.yaml**

Add to the end of `internal/rules/default_rules.yaml`:

```yaml

  # ===========================================================================
  # AST-BASED RULES (tree-sitter)
  # ===========================================================================

  - id: "AST001"
    name: "function-length"
    type: ast
    category: "maintainability"
    ast_check: "function-length"
    ast_config:
      max_lines: 50
    level: "note"
    confidence: 1.0
    message: "Function exceeds maximum length"
    explanation: "Long functions are harder to understand, test, and maintain. Consider decomposing into smaller, focused functions."
    remediation: "Extract helper functions to reduce complexity. Each function should do one thing well."
    source: "SonarQube"
    references:
      - "https://rules.sonarsource.com/go/RSPEC-138"

  - id: "AST002"
    name: "nesting-depth"
    type: ast
    category: "maintainability"
    ast_check: "nesting-depth"
    ast_config:
      max_depth: 4
    level: "warning"
    confidence: 0.9
    message: "Deeply nested code block"
    explanation: "Deeply nested code is difficult to follow and error-prone. It often indicates that the function is doing too much."
    remediation: "Use early returns, guard clauses, or extract to separate functions to reduce nesting."
    source: "SonarQube"
    references:
      - "https://rules.sonarsource.com/go/RSPEC-134"

  - id: "AST003"
    name: "empty-error-handler"
    type: ast
    category: "reliability"
    ast_check: "empty-handler"
    level: "warning"
    confidence: 0.95
    message: "Empty error handling block"
    explanation: "Checking for an error but not handling it defeats the purpose of error checking and hides potential issues."
    remediation: "Add error handling: log the error, return it, or take corrective action."
    source: "CWE"
    cwe: ["CWE-252"]
    references:
      - "https://cwe.mitre.org/data/definitions/252.html"

  - id: "AST004"
    name: "param-count"
    type: ast
    category: "maintainability"
    ast_check: "param-count"
    ast_config:
      max_params: 5
    level: "note"
    confidence: 1.0
    message: "Function has too many parameters"
    explanation: "Functions with many parameters are hard to call correctly, test, and maintain."
    remediation: "Group related parameters into a struct, use the options pattern, or decompose the function."
    source: "SonarQube"
    references:
      - "https://rules.sonarsource.com/go/RSPEC-107"
```

**Step 4: Run tests**

Run: `go test ./internal/rules/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/rules/default_rules.yaml internal/rules/embed_test.go
git commit -m "feat: add AST rules to default rules"
```

---

### Task 10: Integrate AST Rules into TieredAnalyzer

**Files:**
- Modify: `internal/analyzer/tiered.go` (lines 54-60, 239-297)
- Create: `internal/analyzer/tiered_ast_test.go`

**Step 1: Write the failing test**

Create `internal/analyzer/tiered_ast_test.go`:

```go
package analyzer

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/config"
	"github.com/chris-regnier/gavel/internal/input"
	"github.com/chris-regnier/gavel/internal/rules"
)

func TestTieredAnalyzer_ASTRules_FunctionLength(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}
	ta := NewTieredAnalyzer(mock)

	// Build a Go file with a long function (60 lines)
	source := "package main\n\nfunc longFunc() {\n"
	for i := 0; i < 57; i++ {
		source += "\tx := 1\n"
	}
	source += "}\n"

	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: source,
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check code", Enabled: true},
	}

	var instantResults []TieredResult
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierInstant {
			instantResults = append(instantResults, result)
		}
	}

	// Should find the AST001 function-length rule
	found := false
	for _, tr := range instantResults {
		for _, r := range tr.Results {
			if r.RuleID == "AST001" {
				found = true
				if ruleType, ok := r.Properties["gavel/rule-type"].(string); !ok || ruleType != "ast" {
					t.Errorf("expected rule-type 'ast', got %v", r.Properties["gavel/rule-type"])
				}
			}
		}
	}

	if !found {
		t.Error("expected to find AST001 function-length finding")
	}
}

func TestTieredAnalyzer_ASTRules_LanguageFilter(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}
	ta := NewTieredAnalyzer(mock)

	// A .csv file should not produce AST findings
	artifacts := []input.Artifact{{
		Path:    "data.csv",
		Content: "col1,col2\nval1,val2",
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierInstant {
			for _, r := range result.Results {
				if r.Properties != nil {
					if ruleType, ok := r.Properties["gavel/rule-type"].(string); ok && ruleType == "ast" {
						t.Error("unexpected AST finding for .csv file")
					}
				}
			}
		}
	}
}

func TestTieredAnalyzer_ASTRules_CustomConfig(t *testing.T) {
	mock := &tieredMockClient{findings: []Finding{}}

	// Override default AST rules with a custom threshold
	customRules := []rules.Rule{{
		ID:        "AST001",
		Name:      "function-length",
		Type:      rules.RuleTypeAST,
		ASTCheck:  "function-length",
		ASTConfig: map[string]interface{}{"max_lines": 3},
		Level:     "warning",
		Message:   "Function too long",
		Confidence: 1.0,
	}}

	ta := NewTieredAnalyzer(mock, WithInstantPatterns(customRules))

	source := `package main

func medium() {
	a := 1
	b := 2
	c := 3
	d := 4
	e := 5
}
`
	artifacts := []input.Artifact{{
		Path:    "test.go",
		Content: source,
		Kind:    input.KindFile,
	}}
	policies := map[string]config.Policy{
		"test": {Instruction: "Check", Enabled: true},
	}

	var found bool
	for result := range ta.AnalyzeProgressive(context.Background(), artifacts, policies, "persona") {
		if result.Tier == TierInstant {
			for _, r := range result.Results {
				if r.RuleID == "AST001" {
					found = true
				}
			}
		}
	}

	if !found {
		t.Error("expected AST001 finding with max_lines=3 threshold")
	}
}
```

**Step 2: Run to verify failure**

Run: `go test ./internal/analyzer/ -run "TestTieredAnalyzer_AST" -v`
Expected: FAIL — no AST dispatching in runPatternMatching

**Step 3: Add AST integration to TieredAnalyzer**

Modify `internal/analyzer/tiered.go`:

1. Add import for `astcheck` package:
```go
import (
	// ... existing imports
	"github.com/chris-regnier/gavel/internal/astcheck"
)
```

2. Add `astRegistry` field to `TieredAnalyzer` struct (after line 57):
```go
	astRegistry *astcheck.Registry
```

3. Initialize it in `NewTieredAnalyzer` (after line 126):
```go
	ta.astRegistry = astcheck.DefaultRegistry()
```

4. Refactor `runPatternMatching` to dispatch by rule type. Extract the existing regex logic into `runRegexRules`, add `runASTRules`:

```go
func (ta *TieredAnalyzer) runPatternMatching(art input.Artifact) []sarif.Result {
	var regexRules, astRules []rules.Rule
	for _, rule := range ta.instantPatterns {
		switch rule.Type {
		case rules.RuleTypeAST:
			astRules = append(astRules, rule)
		default:
			regexRules = append(regexRules, rule)
		}
	}

	var results []sarif.Result
	results = append(results, ta.runRegexRules(art, regexRules)...)
	if len(astRules) > 0 {
		results = append(results, ta.runASTRules(art, astRules)...)
	}
	return results
}
```

5. Rename the existing regex matching logic body to `runRegexRules(art, rules)` (extract from the current `runPatternMatching`).

6. Add `runASTRules`:

```go
func (ta *TieredAnalyzer) runASTRules(art input.Artifact, astRules []rules.Rule) []sarif.Result {
	lr := astcheck.NewLanguageRegistry()
	lang, langName, ok := lr.Detect(art.Path)
	if !ok {
		return nil // Unsupported language
	}

	// Parse once, shared across all checks
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(art.Content))
	if err != nil {
		return nil
	}

	var results []sarif.Result
	for _, rule := range astRules {
		// Check language filter
		if len(rule.Languages) > 0 && !matchesLanguage(art.Path, rule.Languages) {
			continue
		}

		check, ok := ta.astRegistry.Get(rule.ASTCheck)
		if !ok {
			continue // Unknown check — skip silently
		}

		matches := check.Run(tree, []byte(art.Content), langName, rule.ASTConfig)
		for _, m := range matches {
			msg := rule.Message
			if m.Message != "" {
				msg = m.Message
			}

			props := map[string]interface{}{
				"gavel/explanation": rule.Explanation,
				"gavel/confidence":  rule.Confidence,
				"gavel/tier":        "instant",
				"gavel/rule-type":   "ast",
				"gavel/rule-source": string(rule.Source),
			}
			if len(rule.CWE) > 0 {
				props["gavel/cwe"] = rule.CWE
			}
			if len(rule.OWASP) > 0 {
				props["gavel/owasp"] = rule.OWASP
			}
			if rule.Remediation != "" {
				props["gavel/remediation"] = rule.Remediation
			}
			if len(rule.References) > 0 {
				props["gavel/references"] = rule.References
			}
			if m.Extra != nil {
				for k, v := range m.Extra {
					props["gavel/"+k] = v
				}
			}

			results = append(results, sarif.Result{
				RuleID:  rule.ID,
				Level:   rule.Level,
				Message: sarif.Message{Text: msg},
				Locations: []sarif.Location{{
					PhysicalLocation: sarif.PhysicalLocation{
						ArtifactLocation: sarif.ArtifactLocation{URI: art.Path},
						Region:           sarif.Region{StartLine: m.StartLine, EndLine: m.EndLine},
					},
				}},
				Properties: props,
			})
		}
	}
	return results
}
```

Also add the tree-sitter import:
```go
sitter "github.com/smacker/go-tree-sitter"
```

**Step 4: Run all analyzer tests**

Run: `go test ./internal/analyzer/ -v`
Expected: ALL PASS (both new AST tests and existing regex/tiered tests)

**Step 5: Run full test suite**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/analyzer/tiered.go internal/analyzer/tiered_ast_test.go
git commit -m "feat: integrate AST rules into TieredAnalyzer instant tier"
```

---

### Task 11: Full Build and Integration Verification

**Files:** None (verification only)

**Step 1: Build**

Run: `task build`
Expected: Compiles without error

**Step 2: Run full test suite**

Run: `task test`
Expected: ALL PASS

**Step 3: Run linter**

Run: `task lint`
Expected: No issues

**Step 4: Manual smoke test**

Create a test file with a long function and verify AST rules fire:

```bash
mkdir -p /tmp/gavel-ast-test
cat > /tmp/gavel-ast-test/main.go << 'EOF'
package main

import "fmt"

func reallyLongFunction(a, b, c, d, e, f int) {
	// This function has too many params and is too long
	x := 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	x = x + 1
	if true {
		if true {
			if true {
				if true {
					if true {
						fmt.Println("deeply nested")
					}
				}
			}
		}
	}
	err := fmt.Errorf("test")
	if err != nil {
	}
	_ = x
}
EOF
```

Run (adjust for available provider):
```bash
./gavel analyze --dir /tmp/gavel-ast-test 2>&1 | head -50
```

Expected: SARIF output contains findings for AST001 (function-length), AST002 (nesting-depth), AST003 (empty-error-handler), AST004 (param-count).

Note: This requires a configured LLM provider. If no provider is available, the command will error on the LLM tier but the instant-tier AST findings will still have been produced. You can verify by checking the `.gavel/results/` output directory.

**Step 5: Commit (no changes expected)**

If any fixes were needed, commit them:
```bash
git add -A && git commit -m "fix: address AST integration issues from smoke test"
```

---

### Task 12: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Add AST analyzer documentation**

Add a section after "Vendable rules" in the Key Design Decisions:

```markdown
- **AST analyzer** (`internal/astcheck/`): Tree-sitter-based structural code analysis. `Check` interface with `Run(tree, source, lang, config)`. Language detection via file extension. Four built-in checks: function-length (AST001), nesting-depth (AST002), empty-handler (AST003), param-count (AST004). Rules use `type: ast` in YAML with `ast_check` and `ast_config` fields. Integrated into TieredAnalyzer's instant tier alongside regex rules.
```

**Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: add AST analyzer to CLAUDE.md"
```
