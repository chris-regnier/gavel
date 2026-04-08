package astcheck

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// ResolveLogicalLocation parses the source file (if the language is supported)
// and returns a LogicalLocation for the enclosing function at the given
// 1-indexed line. Returns nil when no enclosing function can be determined.
func ResolveLogicalLocation(path string, source []byte, line int) *sarif.LogicalLocation {
	lang, langName, ok := Detect(path)
	if !ok {
		return nil
	}

	parser := sitter.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	if err != nil {
		return nil
	}

	return resolveFromTree(tree, source, langName, line)
}

// ResolveLogicalLocationFromTree is like ResolveLogicalLocation but reuses an
// already-parsed tree (avoiding a redundant parse in the AST-tier path).
func ResolveLogicalLocationFromTree(tree *sitter.Tree, source []byte, lang string, line int) *sarif.LogicalLocation {
	return resolveFromTree(tree, source, lang, line)
}

func resolveFromTree(tree *sitter.Tree, source []byte, lang string, line int) *sarif.LogicalLocation {
	fc := FindEnclosingFunction(tree.RootNode(), source, lang, line)
	if fc == nil {
		return nil
	}

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
