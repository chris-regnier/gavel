package astcheck

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// ParseTree parses the source file and returns a tree-sitter Tree, or nil if
// the language is unsupported or parsing fails. Callers can pass the result to
// ResolveLogicalLocationFromTree to resolve multiple locations without re-parsing.
func ParseTree(path string, source []byte) *sitter.Tree {
	lang, _, ok := Detect(path)
	if !ok {
		return nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil
	}
	return tree
}

// BuildIndex parses the source and pre-computes a FunctionIndex for fast
// lookups. Returns nil if the language is unsupported or parsing fails.
func BuildIndex(path string, source []byte) (*FunctionIndex, string) {
	tree := ParseTree(path, source)
	if tree == nil {
		return nil, ""
	}
	_, langName, _ := Detect(path)
	idx := BuildFunctionIndex(tree.RootNode(), source, langName)
	return idx, langName
}

// ResolveLogicalLocation parses the source file (if the language is supported)
// and returns a LogicalLocation for the enclosing function at the given
// 1-indexed line. Returns nil when no enclosing function can be determined.
// For multiple lookups in the same file, prefer BuildIndex + LogicalLocationFromIndex.
func ResolveLogicalLocation(path string, source []byte, line int) *sarif.LogicalLocation {
	tree := ParseTree(path, source)
	if tree == nil {
		return nil
	}
	_, langName, _ := Detect(path)
	return resolveFromTree(tree, source, langName, line)
}

// ResolveLogicalLocationFromTree is like ResolveLogicalLocation but reuses an
// already-parsed tree (avoiding a redundant parse in the AST-tier path).
func ResolveLogicalLocationFromTree(tree *sitter.Tree, source []byte, lang string, line int) *sarif.LogicalLocation {
	return resolveFromTree(tree, source, lang, line)
}

// LogicalLocationFromIndex resolves a LogicalLocation using a pre-built
// FunctionIndex, avoiding CGO overhead entirely.
func LogicalLocationFromIndex(idx *FunctionIndex, line int) *sarif.LogicalLocation {
	fc := idx.Lookup(line)
	if fc == nil {
		return nil
	}
	return toLogicalLocation(fc)
}

func resolveFromTree(tree *sitter.Tree, source []byte, lang string, line int) *sarif.LogicalLocation {
	if tree == nil {
		return nil
	}
	fc := FindEnclosingFunction(tree.RootNode(), source, lang, line)
	if fc == nil {
		return nil
	}
	return toLogicalLocation(fc)
}

func toLogicalLocation(fc *FunctionContext) *sarif.LogicalLocation {
	ll := &sarif.LogicalLocation{
		Name: fc.FuncName,
		Kind: "function",
	}
	if fc.ClassName != "" {
		ll.FullyQualifiedName = fc.ClassName + "." + fc.FuncName
	} else {
		ll.FullyQualifiedName = fc.FuncName
	}
	return ll
}
