# AST Analyzer Design

## Summary

Add tree-sitter-based AST analysis as a new rule type (`type: ast`) alongside the existing regex rules (`type: regex`). AST rules run within the existing TieredAnalyzer's instant tier, providing structural code checks that regex cannot express (function length, nesting depth, empty error handlers, parameter counts).

## Motivation

The existing regex rules system has 15 rules covering security, reliability, and maintainability. Some checks are inherently limited by regex:

- `defer-in-loop` (S2259): Comment in `default_rules.yaml` notes "A robust fix would require AST-based analysis"
- `unreachable-code` (S1144): Simple regex misses complex control flow
- Function length, nesting depth, cyclomatic complexity: Not expressible as regex at all

Tree-sitter provides fast, multi-language AST parsing that fills this gap while staying within the instant tier's <100ms budget.

## Approach

Embed tree-sitter via [`smacker/go-tree-sitter`](https://github.com/smacker/go-tree-sitter) directly in the Gavel binary. This was chosen over shelling out to [`ast-grep`](https://ast-grep.github.io/) because:

- **Single binary**: No external tool dependency. Critical for CI.
- **Already CGO**: BAML requires CGO, so tree-sitter's CGO adds no new build constraint.
- **In-process**: Integrates directly with the existing cache and async pipeline. No subprocess overhead.
- **Sufficient expressiveness**: Tree-sitter S-expression queries handle the structural checks Gavel needs.

## Design

### Rule Type Extension

The `Rule` struct in `internal/rules/rules.go` gets a `Type` field:

```go
type RuleType string

const (
    RuleTypeRegex RuleType = "regex"  // default
    RuleTypeAST   RuleType = "ast"    // tree-sitter
)
```

New fields on `Rule`:

| Field | Type | Purpose |
|-------|------|---------|
| `Type` | `RuleType` | `"regex"` (default) or `"ast"` |
| `ASTCheck` | `string` | Name of registered check (e.g., `"function-length"`) |
| `ASTConfig` | `map[string]interface{}` | Check-specific parameters (e.g., `max_lines: 50`) |

Existing regex rules are unaffected (Type defaults to `"regex"`, Pattern field still required for regex rules).

### AST Rules in YAML

AST rules use the same YAML format as regex rules but reference a registered check name instead of a regex pattern:

```yaml
rules:
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
    explanation: "Long functions are harder to understand, test, and maintain."
    remediation: "Extract helper functions to reduce complexity."

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
    explanation: "Deeply nested code is difficult to follow and error-prone."
    remediation: "Use early returns or extract to separate functions."

  - id: "AST003"
    name: "empty-error-handler"
    type: ast
    category: "reliability"
    ast_check: "empty-handler"
    level: "warning"
    confidence: 0.95
    message: "Empty error handling block"
    explanation: "Checking for an error but not handling it defeats the purpose of error checking."
    remediation: "Add error handling logic or explicitly document why it's safe to ignore."
    cwe: ["CWE-252"]

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
    explanation: "Functions with many parameters are hard to call correctly and indicate design issues."
    remediation: "Consider grouping related parameters into a struct or options pattern."
```

Users can override `ast_config` values in their project's `.gavel/rules/*.yaml` files using the same tiered merging that regex rules use.

### AST Check Registry

```
internal/astcheck/
├── registry.go        # Check interface + global registry
├── language.go        # tree-sitter grammar registry + language detection
├── function_length.go # AST001
├── nesting_depth.go   # AST002
├── empty_handler.go   # AST003
└── param_count.go     # AST004
```

**Check interface:**

```go
type Check interface {
    Name() string
    Run(tree *sitter.Tree, source []byte, lang string,
        config map[string]interface{}) []Match
}

type Match struct {
    StartLine int
    EndLine   int
    Message   string                 // optional override of rule message
    Extra     map[string]interface{} // e.g., {"actual_lines": 72}
}
```

Checks register via `init()`:

```go
func init() { Register(&FunctionLength{}) }
```

### Language Support

```go
type LanguageRegistry struct {
    grammars map[string]*sitter.Language
}
```

**V1 grammars:** Go, Python, JavaScript, TypeScript, Java, C, Rust.

Language detection is file-extension-based (reusing the existing `matchesLanguage` logic). Files in unsupported languages silently produce no AST findings.

The tree-sitter parse happens **once per file**, then all applicable AST checks share the parsed tree. This keeps analysis well within the instant tier's <100ms budget.

### Integration into TieredAnalyzer

The existing `runPatternMatching` method in `internal/analyzer/tiered.go` is extended:

```go
func (ta *TieredAnalyzer) runPatternMatching(art input.Artifact) []sarif.Result {
    var results []sarif.Result

    // Partition rules by type
    var regexRules, astRules []rules.Rule
    for _, rule := range ta.instantPatterns {
        switch rule.Type {
        case rules.RuleTypeAST:
            astRules = append(astRules, rule)
        default:
            regexRules = append(regexRules, rule)
        }
    }

    // Run regex rules (existing logic, unchanged)
    results = append(results, ta.runRegexRules(art, regexRules)...)

    // Run AST rules (new)
    if len(astRules) > 0 {
        results = append(results, ta.runASTRules(art, astRules)...)
    }

    return results
}
```

The `runASTRules` method:
1. Detects language from file extension
2. Parses once with tree-sitter
3. Runs each applicable AST check against the shared tree
4. Converts `Match` to `sarif.Result` with rule metadata (CWE, OWASP, remediation, etc.)

Results get `"gavel/tier": "instant"` and `"gavel/rule-type": "ast"` properties.

### Validation Changes

`ParseRuleFile` validation is updated:
- `type: regex` (or empty): `pattern` field required, `ast_check` ignored
- `type: ast`: `ast_check` field required, must exist in registry. `pattern` field ignored.

### Data Flow

```
Load Rules (default + user + project)
    ↓
TieredAnalyzer receives []rules.Rule
    ↓
runPatternMatching per artifact:
    ├── Regex rules → line-by-line regex matching (existing)
    └── AST rules → tree-sitter parse → run checks → Match → sarif.Result
    ↓
Results merge with LLM results → dedup → SARIF → Rego
```

## Initial AST Checks (V1)

| ID | Check Name | What It Detects | Languages |
|----|------------|-----------------|-----------|
| AST001 | `function-length` | Functions/methods exceeding configurable line threshold | Go, Python, JS/TS, Java, C, Rust |
| AST002 | `nesting-depth` | Deeply nested code blocks (if/for/switch) | Go, Python, JS/TS, Java |
| AST003 | `empty-handler` | Empty catch/except blocks, `_ = err` in Go | Go, Python, JS/TS, Java |
| AST004 | `param-count` | Functions with too many parameters | Go, Python, JS/TS, Java, C, Rust |

## Dependencies

- [`smacker/go-tree-sitter`](https://github.com/smacker/go-tree-sitter) — Go bindings for tree-sitter
- Tree-sitter language grammars (bundled as Go packages from `go-tree-sitter`)

## Build Impact

- Binary size: ~1-3 MB per language grammar (7 grammars = ~7-21 MB increase)
- Build time: Slightly longer due to CGO compilation of grammar C code
- No new external runtime dependencies

## Future Extensions

- **Cyclomatic complexity check**: Count branch points in function ASTs
- **Unreachable code detection**: Replace regex rule S1144 with AST-aware version
- **Defer-in-loop detection**: Replace regex rule S2259 with AST-aware version
- **Custom tree-sitter queries**: Allow users to define S-expression queries in YAML for advanced pattern matching
- **Additional languages**: Add grammars as needed (Ruby, PHP, Kotlin, Swift, etc.)
