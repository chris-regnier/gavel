package astcheck

import sitter "github.com/smacker/go-tree-sitter"

// findNodes performs a recursive DFS and calls fn for every node whose Type()
// is in the nodeTypes set.
func findNodes(node *sitter.Node, nodeTypes map[string]bool, fn func(*sitter.Node)) {
	if node == nil {
		return
	}
	if nodeTypes[node.Type()] {
		fn(node)
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		findNodes(node.Child(int(i)), nodeTypes, fn)
	}
}

// funcNodeTypes returns the set of AST node types that represent function
// definitions for the given language.
func funcNodeTypes(lang string) map[string]bool {
	switch lang {
	case "go":
		return map[string]bool{
			"function_declaration": true,
			"method_declaration":   true,
		}
	case "python":
		return map[string]bool{
			"function_definition": true,
		}
	case "javascript", "typescript":
		return map[string]bool{
			"function_declaration": true,
			"method_definition":    true,
			"arrow_function":       true,
		}
	case "java":
		return map[string]bool{
			"method_declaration":      true,
			"constructor_declaration": true,
		}
	case "c":
		return map[string]bool{
			"function_definition": true,
		}
	case "rust":
		return map[string]bool{
			"function_item": true,
		}
	default:
		return nil
	}
}

// funcName extracts a human-readable function name from a function node.
func funcName(node *sitter.Node, source []byte) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return nameNode.Content(source)
	}
	return "<anonymous>"
}
