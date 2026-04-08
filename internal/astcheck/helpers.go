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

// FunctionContext describes the enclosing function (and optional class) for a
// source line. It is used to populate SARIF logicalLocations.
type FunctionContext struct {
	FuncName  string // e.g. "HandleLogin"
	ClassName string // e.g. "AuthService" (empty if top-level)
}

// FindEnclosingFunction returns the innermost function/method that contains the
// given 1-indexed line number. It returns nil when no enclosing function is found.
func FindEnclosingFunction(root *sitter.Node, source []byte, lang string, line int) *FunctionContext {
	fnTypes := funcNodeTypes(lang)
	if fnTypes == nil {
		return nil
	}

	// Convert 1-indexed line to 0-indexed row used by tree-sitter.
	row := uint32(line - 1)

	var best *sitter.Node
	findNodes(root, fnTypes, func(node *sitter.Node) {
		if node.StartPoint().Row <= row && node.EndPoint().Row >= row {
			// Prefer the innermost (narrowest) enclosing function.
			if best == nil || nodeSpan(node) < nodeSpan(best) {
				best = node
			}
		}
	})

	if best == nil {
		return nil
	}

	ctx := &FunctionContext{
		FuncName: funcName(best, source),
	}

	// Look for an enclosing class/struct/impl to build a fully-qualified name.
	ctx.ClassName = findEnclosingClass(best, source, lang)

	return ctx
}

// nodeSpan returns the number of rows a node spans (used to pick the narrowest match).
func nodeSpan(n *sitter.Node) uint32 {
	return n.EndPoint().Row - n.StartPoint().Row
}

// classNodeTypes returns AST node types that represent class/struct containers.
func classNodeTypes(lang string) map[string]bool {
	switch lang {
	case "go":
		// Go methods use receiver types; we handle this via method_declaration's receiver.
		return nil
	case "python":
		return map[string]bool{"class_definition": true}
	case "javascript", "typescript":
		return map[string]bool{"class_declaration": true, "class": true}
	case "java":
		return map[string]bool{"class_declaration": true, "interface_declaration": true}
	case "rust":
		return map[string]bool{"impl_item": true}
	case "c":
		return nil
	default:
		return nil
	}
}

// findEnclosingClass walks up from node to find the nearest class/struct container.
// For Go, it extracts the receiver type from method_declaration nodes.
func findEnclosingClass(node *sitter.Node, source []byte, lang string) string {
	// Special handling for Go method receivers.
	if lang == "go" && node.Type() == "method_declaration" {
		if params := node.ChildByFieldName("receiver"); params != nil {
			// receiver is a parameter_list; extract the type name from within.
			return extractGoReceiverType(params, source)
		}
		return ""
	}

	ctypes := classNodeTypes(lang)
	if ctypes == nil {
		return ""
	}

	// Walk up the tree from the function node.
	for p := node.Parent(); p != nil; p = p.Parent() {
		if ctypes[p.Type()] {
			// Rust impl blocks use the "type" field, others use "name".
			if lang == "rust" {
				if typeNode := p.ChildByFieldName("type"); typeNode != nil {
					return typeNode.Content(source)
				}
			}
			if nameNode := p.ChildByFieldName("name"); nameNode != nil {
				return nameNode.Content(source)
			}
		}
	}
	return ""
}

// extractGoReceiverType extracts the type name from a Go method receiver parameter list.
// e.g., "(s *Server)" -> "Server", "(s Server)" -> "Server".
func extractGoReceiverType(params *sitter.Node, source []byte) string {
	for i := 0; i < int(params.NamedChildCount()); i++ {
		param := params.NamedChild(int(i))
		typeNode := param.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}
		// Handle pointer receivers: *Server -> Server
		if typeNode.Type() == "pointer_type" && typeNode.NamedChildCount() > 0 {
			return typeNode.NamedChild(0).Content(source)
		}
		return typeNode.Content(source)
	}
	return ""
}
