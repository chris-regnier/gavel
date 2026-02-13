package astcheck

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func parseGo(t *testing.T, source string) *sitter.Tree {
	t.Helper()
	return parseWith(t, source, golang.GetLanguage())
}

func parsePython(t *testing.T, source string) *sitter.Tree {
	t.Helper()
	return parseWith(t, source, python.GetLanguage())
}

func parseJS(t *testing.T, source string) *sitter.Tree {
	t.Helper()
	return parseWith(t, source, javascript.GetLanguage())
}

func parseWith(t *testing.T, source string, lang *sitter.Language) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}
	return tree
}

// ---------------------------------------------------------------------------
// Registry tests
// ---------------------------------------------------------------------------

func TestRegistryBasics(t *testing.T) {
	r := NewRegistry()
	if len(r.Names()) != 0 {
		t.Fatal("new registry should be empty")
	}

	r.Register(&FunctionLength{})
	r.Register(&NestingDepth{})

	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(names))
	}
	// Names() returns sorted
	if names[0] != "function-length" || names[1] != "nesting-depth" {
		t.Fatalf("unexpected names: %v", names)
	}

	c, ok := r.Get("function-length")
	if !ok || c == nil {
		t.Fatal("expected to find function-length check")
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Fatal("should not find nonexistent check")
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	names := r.Names()
	expected := []string{"empty-handler", "function-length", "nesting-depth", "param-count"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d checks, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("expected names[%d]=%q, got %q", i, name, names[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Language detection tests
// ---------------------------------------------------------------------------

func TestDetect(t *testing.T) {
	tests := []struct {
		path     string
		wantName string
		wantOK   bool
	}{
		{"main.go", "go", true},
		{"script.py", "python", true},
		{"app.js", "javascript", true},
		{"app.jsx", "javascript", true},
		{"app.ts", "typescript", true},
		{"app.tsx", "typescript", true},
		{"Main.java", "java", true},
		{"lib.c", "c", true},
		{"lib.h", "c", true},
		{"main.rs", "rust", true},
		{"readme.md", "", false},
		{"data.json", "", false},
		{"/path/to/file.GO", "go", true}, // case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			lang, name, ok := Detect(tt.path)
			if ok != tt.wantOK {
				t.Errorf("Detect(%q) ok=%v, want %v", tt.path, ok, tt.wantOK)
			}
			if name != tt.wantName {
				t.Errorf("Detect(%q) name=%q, want %q", tt.path, name, tt.wantName)
			}
			if ok && lang == nil {
				t.Errorf("Detect(%q) returned nil language", tt.path)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FunctionLength tests
// ---------------------------------------------------------------------------

func TestFunctionLengthName(t *testing.T) {
	c := &FunctionLength{}
	if c.Name() != "function-length" {
		t.Errorf("expected name 'function-length', got %q", c.Name())
	}
}

func TestFunctionLengthShortFunc(t *testing.T) {
	src := `package main

func short() {
	return
}
`
	tree := parseGo(t, src)
	c := &FunctionLength{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for short function, got %d", len(matches))
	}
}

func TestFunctionLengthLongFunc(t *testing.T) {
	// Build a function with >50 lines
	src := "package main\n\nfunc longFunc() {\n"
	for i := 0; i < 55; i++ {
		src += "\t_ = 0\n"
	}
	src += "}\n"

	tree := parseGo(t, src)
	c := &FunctionLength{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Extra["function"] != "longFunc" {
		t.Errorf("expected function name 'longFunc', got %v", matches[0].Extra["function"])
	}
}

func TestFunctionLengthCustomThreshold(t *testing.T) {
	src := `package main

func medium() {
	a := 1
	b := 2
	c := 3
	d := 4
	e := 5
}
`
	tree := parseGo(t, src)
	c := &FunctionLength{}

	// With default (50), should not match
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches with default threshold, got %d", len(matches))
	}

	// With threshold of 3, should match
	matches = c.Run(tree, []byte(src), "go", map[string]interface{}{"max_lines": 3})
	if len(matches) != 1 {
		t.Errorf("expected 1 match with threshold 3, got %d", len(matches))
	}
}

func TestFunctionLengthGoMethod(t *testing.T) {
	src := `package main

type S struct{}

func (s S) longMethod() {
` + repeatLines("\tx := 1\n", 55) + `}
`
	tree := parseGo(t, src)
	c := &FunctionLength{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for long method, got %d", len(matches))
	}
	if matches[0].Extra["function"] != "longMethod" {
		t.Errorf("expected function name 'longMethod', got %v", matches[0].Extra["function"])
	}
}

func TestFunctionLengthPython(t *testing.T) {
	src := "def long_func():\n"
	for i := 0; i < 55; i++ {
		src += "    x = 1\n"
	}

	tree := parsePython(t, src)
	c := &FunctionLength{}
	matches := c.Run(tree, []byte(src), "python", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for long python function, got %d", len(matches))
	}
	if matches[0].Extra["function"] != "long_func" {
		t.Errorf("expected function name 'long_func', got %v", matches[0].Extra["function"])
	}
}

func TestFunctionLengthJSArrow(t *testing.T) {
	src := "const fn = () => {\n"
	for i := 0; i < 55; i++ {
		src += "  let x = 1;\n"
	}
	src += "};\n"

	tree := parseJS(t, src)
	c := &FunctionLength{}
	matches := c.Run(tree, []byte(src), "javascript", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for long arrow function, got %d", len(matches))
	}
	// Arrow functions are anonymous
	if matches[0].Extra["function"] != "<anonymous>" {
		t.Errorf("expected function name '<anonymous>', got %v", matches[0].Extra["function"])
	}
}

func TestFunctionLengthUnknownLang(t *testing.T) {
	tree := parseGo(t, "package main")
	c := &FunctionLength{}
	matches := c.Run(tree, []byte("package main"), "cobol", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for unknown language, got %d", len(matches))
	}
}

// ---------------------------------------------------------------------------
// NestingDepth tests
// ---------------------------------------------------------------------------

func TestNestingDepthName(t *testing.T) {
	c := &NestingDepth{}
	if c.Name() != "nesting-depth" {
		t.Errorf("expected name 'nesting-depth', got %q", c.Name())
	}
}

func TestNestingDepthShallow(t *testing.T) {
	src := `package main

func main() {
	if true {
		if true {
			return
		}
	}
}
`
	tree := parseGo(t, src)
	c := &NestingDepth{}
	matches := c.Run(tree, []byte(src), "go", nil) // default max 4
	if len(matches) != 0 {
		t.Errorf("expected no matches for shallow nesting, got %d", len(matches))
	}
}

func TestNestingDepthDeep(t *testing.T) {
	src := `package main

func main() {
	if true {
		if true {
			if true {
				if true {
					if true {
						return
					}
				}
			}
		}
	}
}
`
	tree := parseGo(t, src)
	c := &NestingDepth{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) == 0 {
		t.Fatal("expected at least 1 match for deep nesting")
	}
}

func TestNestingDepthCustomThreshold(t *testing.T) {
	src := `package main

func main() {
	if true {
		if true {
			return
		}
	}
}
`
	tree := parseGo(t, src)
	c := &NestingDepth{}

	// With max_depth=1, the second if should trigger
	matches := c.Run(tree, []byte(src), "go", map[string]interface{}{"max_depth": 1})
	if len(matches) == 0 {
		t.Fatal("expected match with max_depth=1")
	}

	// With max_depth=10, nothing should trigger
	matches = c.Run(tree, []byte(src), "go", map[string]interface{}{"max_depth": 10})
	if len(matches) != 0 {
		t.Errorf("expected no matches with max_depth=10, got %d", len(matches))
	}
}

func TestNestingDepthPython(t *testing.T) {
	src := `if True:
    if True:
        if True:
            if True:
                if True:
                    pass
`
	tree := parsePython(t, src)
	c := &NestingDepth{}
	matches := c.Run(tree, []byte(src), "python", nil) // default max 4
	if len(matches) == 0 {
		t.Fatal("expected at least 1 match for deep python nesting")
	}
}

func TestNestingDepthForLoop(t *testing.T) {
	src := `package main

func main() {
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			for k := 0; k < 10; k++ {
				for l := 0; l < 10; l++ {
					for m := 0; m < 10; m++ {
						_ = m
					}
				}
			}
		}
	}
}
`
	tree := parseGo(t, src)
	c := &NestingDepth{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) == 0 {
		t.Fatal("expected at least 1 match for deeply nested for loops")
	}
}

func TestNestingDepthUnknownLang(t *testing.T) {
	tree := parseGo(t, "package main")
	c := &NestingDepth{}
	matches := c.Run(tree, []byte("package main"), "unknown", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for unknown language, got %d", len(matches))
	}
}

// ---------------------------------------------------------------------------
// EmptyHandler tests
// ---------------------------------------------------------------------------

func TestEmptyHandlerName(t *testing.T) {
	c := &EmptyHandler{}
	if c.Name() != "empty-handler" {
		t.Errorf("expected name 'empty-handler', got %q", c.Name())
	}
}

func TestEmptyHandlerGoDetectsEmpty(t *testing.T) {
	src := `package main

import "errors"

func main() {
	err := errors.New("fail")
	if err != nil {
	}
}
`
	tree := parseGo(t, src)
	c := &EmptyHandler{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for empty error handler, got %d", len(matches))
	}
}

func TestEmptyHandlerGoNonEmpty(t *testing.T) {
	src := `package main

import "fmt"

func main() {
	var err error
	if err != nil {
		fmt.Println(err)
	}
}
`
	tree := parseGo(t, src)
	c := &EmptyHandler{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for non-empty error handler, got %d", len(matches))
	}
}

func TestEmptyHandlerGoOtherCondition(t *testing.T) {
	src := `package main

func main() {
	if x > 0 {
	}
}
`
	tree := parseGo(t, src)
	c := &EmptyHandler{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for non-error if block, got %d", len(matches))
	}
}

func TestEmptyHandlerPythonDetectsPass(t *testing.T) {
	src := `try:
    x = 1
except:
    pass
`
	tree := parsePython(t, src)
	c := &EmptyHandler{}
	matches := c.Run(tree, []byte(src), "python", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for except:pass, got %d", len(matches))
	}
}

func TestEmptyHandlerPythonNonEmpty(t *testing.T) {
	src := `try:
    x = 1
except Exception as e:
    print(e)
`
	tree := parsePython(t, src)
	c := &EmptyHandler{}
	matches := c.Run(tree, []byte(src), "python", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for non-empty except, got %d", len(matches))
	}
}

func TestEmptyHandlerJSDetectsEmpty(t *testing.T) {
	src := `try {
  throw new Error("fail");
} catch (e) {
}
`
	tree := parseJS(t, src)
	c := &EmptyHandler{}
	matches := c.Run(tree, []byte(src), "javascript", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for empty catch, got %d", len(matches))
	}
}

func TestEmptyHandlerJSNonEmpty(t *testing.T) {
	src := `try {
  throw new Error("fail");
} catch (e) {
  console.log(e);
}
`
	tree := parseJS(t, src)
	c := &EmptyHandler{}
	matches := c.Run(tree, []byte(src), "javascript", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for non-empty catch, got %d", len(matches))
	}
}

func TestEmptyHandlerUnknownLang(t *testing.T) {
	tree := parseGo(t, "package main")
	c := &EmptyHandler{}
	matches := c.Run(tree, []byte("package main"), "ruby", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for unsupported language, got %d", len(matches))
	}
}

// ---------------------------------------------------------------------------
// ParamCount tests
// ---------------------------------------------------------------------------

func TestParamCountName(t *testing.T) {
	c := &ParamCount{}
	if c.Name() != "param-count" {
		t.Errorf("expected name 'param-count', got %q", c.Name())
	}
}

func TestParamCountGoFewParams(t *testing.T) {
	src := `package main

func fewParams(a, b int) {
}
`
	tree := parseGo(t, src)
	c := &ParamCount{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for few params, got %d", len(matches))
	}
}

func TestParamCountGoTooManyParams(t *testing.T) {
	src := `package main

func tooMany(a, b, c int, d string, e float64, f bool) {
}
`
	tree := parseGo(t, src)
	c := &ParamCount{}
	matches := c.Run(tree, []byte(src), "go", nil) // default max 5
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for too many params, got %d", len(matches))
	}
	if matches[0].Extra["function"] != "tooMany" {
		t.Errorf("expected function name 'tooMany', got %v", matches[0].Extra["function"])
	}
	if matches[0].Extra["param_count"] != 6 {
		t.Errorf("expected param_count=6, got %v", matches[0].Extra["param_count"])
	}
}

func TestParamCountGoGroupedParams(t *testing.T) {
	// a, b, c are grouped in one parameter_declaration
	src := `package main

func grouped(a, b, c int) {
}
`
	tree := parseGo(t, src)
	c := &ParamCount{}

	// With max 2, should trigger (3 params)
	matches := c.Run(tree, []byte(src), "go", map[string]interface{}{"max_params": 2})
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Extra["param_count"] != 3 {
		t.Errorf("expected param_count=3 for grouped params, got %v", matches[0].Extra["param_count"])
	}
}

func TestParamCountCustomThreshold(t *testing.T) {
	src := `package main

func twoParams(a, b int) {
}
`
	tree := parseGo(t, src)
	c := &ParamCount{}

	// max_params=1 should trigger
	matches := c.Run(tree, []byte(src), "go", map[string]interface{}{"max_params": 1})
	if len(matches) != 1 {
		t.Fatalf("expected 1 match with max_params=1, got %d", len(matches))
	}

	// max_params=10 should not trigger
	matches = c.Run(tree, []byte(src), "go", map[string]interface{}{"max_params": 10})
	if len(matches) != 0 {
		t.Errorf("expected 0 matches with max_params=10, got %d", len(matches))
	}
}

func TestParamCountPython(t *testing.T) {
	src := `def too_many(a, b, c, d, e, f):
    pass
`
	tree := parsePython(t, src)
	c := &ParamCount{}
	matches := c.Run(tree, []byte(src), "python", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for python function with 6 params, got %d", len(matches))
	}
	if matches[0].Extra["function"] != "too_many" {
		t.Errorf("expected function name 'too_many', got %v", matches[0].Extra["function"])
	}
}

func TestParamCountPythonFew(t *testing.T) {
	src := `def ok_func(a, b):
    pass
`
	tree := parsePython(t, src)
	c := &ParamCount{}
	matches := c.Run(tree, []byte(src), "python", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for 2-param python function, got %d", len(matches))
	}
}

func TestParamCountJS(t *testing.T) {
	src := `function tooMany(a, b, c, d, e, f) {
}
`
	tree := parseJS(t, src)
	c := &ParamCount{}
	matches := c.Run(tree, []byte(src), "javascript", nil)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for JS function with 6 params, got %d", len(matches))
	}
}

func TestParamCountUnknownLang(t *testing.T) {
	tree := parseGo(t, "package main")
	c := &ParamCount{}
	matches := c.Run(tree, []byte("package main"), "haskell", nil)
	if len(matches) != 0 {
		t.Errorf("expected no matches for unknown language, got %d", len(matches))
	}
}

func TestParamCountNilConfig(t *testing.T) {
	src := `package main

func ok(a int) {
}
`
	tree := parseGo(t, src)
	c := &ParamCount{}
	matches := c.Run(tree, []byte(src), "go", nil)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches with nil config, got %d", len(matches))
	}
}

// ---------------------------------------------------------------------------
// Integration-style test: DefaultRegistry runs all checks
// ---------------------------------------------------------------------------

func TestDefaultRegistryRunsAllChecks(t *testing.T) {
	reg := DefaultRegistry()

	src := `package main

import "fmt"

func longFunc(a, b, c, d, e, f int) {
` + repeatLines("\tx := 1\n", 55) + `}

func nested() {
	if true {
		if true {
			if true {
				if true {
					if true {
						return
					}
				}
			}
		}
	}
}

func emptyErr() {
	var err error
	if err != nil {
	}
}
`
	tree := parseGo(t, src)
	sourceBytes := []byte(src)

	totalMatches := 0
	for _, name := range reg.Names() {
		check, _ := reg.Get(name)
		matches := check.Run(tree, sourceBytes, "go", nil)
		totalMatches += len(matches)
	}

	// We expect at least 1 match from function-length, nesting-depth, empty-handler, param-count
	if totalMatches < 4 {
		t.Errorf("expected at least 4 total matches across all checks, got %d", totalMatches)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func repeatLines(line string, n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += line
	}
	return s
}
