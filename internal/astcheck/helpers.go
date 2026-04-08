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
	if root == nil {
		return nil
	}
	fnTypes := funcNodeTypes(lang)
	if fnTypes == nil {
		return nil
	}

	best := findInnermostFunction(root, fnTypes, uint32(line-1))
	if best == nil {
		return nil
	}

	return &FunctionContext{
		FuncName:  funcName(best, source),
		ClassName: findEnclosingClass(best, source, lang),
	}
}

// findInnermostFunction locates the narrowest function node that contains the
// given 0-indexed row.
func findInnermostFunction(root *sitter.Node, fnTypes map[string]bool, row uint32) *sitter.Node {
	var best *sitter.Node
	findNodes(root, fnTypes, func(node *sitter.Node) {
		if node.StartPoint().Row <= row && node.EndPoint().Row >= row {
			if best == nil || nodeSpan(node) < nodeSpan(best) {
				best = node
			}
		}
	})
	return best
}

// nodeSpan returns the number of rows a node spans (used to pick the narrowest match).
func nodeSpan(n *sitter.Node) uint32 {
	return n.EndPoint().Row - n.StartPoint().Row
}

// funcEntry is a pre-computed function location for O(1) lookup.
type funcEntry struct {
	startRow  uint32
	endRow    uint32
	funcName  string
	className string
}

// FunctionIndex pre-computes function ranges from a tree-sitter tree so that
// repeated lookups avoid costly CGO traversals. Build one with BuildFunctionIndex
// and query with Lookup.
type FunctionIndex struct {
	entries []funcEntry
}

// BuildFunctionIndex traverses the AST once and caches all function ranges.
func BuildFunctionIndex(root *sitter.Node, source []byte, lang string) *FunctionIndex {
	if root == nil {
		return nil
	}
	fnTypes := funcNodeTypes(lang)
	if fnTypes == nil {
		return nil
	}

	var entries []funcEntry
	findNodes(root, fnTypes, func(node *sitter.Node) {
		entries = append(entries, funcEntry{
			startRow:  node.StartPoint().Row,
			endRow:    node.EndPoint().Row,
			funcName:  funcName(node, source),
			className: findEnclosingClass(node, source, lang),
		})
	})

	return &FunctionIndex{entries: entries}
}

// Lookup returns the FunctionContext for the innermost function containing the
// given 1-indexed line, or nil if no function encloses that line.
func (idx *FunctionIndex) Lookup(line int) *FunctionContext {
	if idx == nil {
		return nil
	}
	row := uint32(line - 1)
	var best *funcEntry
	for i := range idx.entries {
		e := &idx.entries[i]
		if e.startRow <= row && e.endRow >= row {
			if best == nil || (e.endRow-e.startRow) < (best.endRow-best.startRow) {
				best = e
			}
		}
	}
	if best == nil {
		return nil
	}
	return &FunctionContext{
		FuncName:  best.funcName,
		ClassName: best.className,
	}
}

// classContainerTypes maps languages to their class/struct/impl AST node types.
var classContainerTypes = map[string]map[string]bool{
	"python":     {"class_definition": true},
	"javascript": {"class_declaration": true, "class": true},
	"typescript": {"class_declaration": true, "class": true},
	"java":       {"class_declaration": true, "interface_declaration": true},
	"rust":       {"impl_item": true},
}

// findEnclosingClass walks up from node to find the nearest class/struct container.
// For Go, it extracts the receiver type from method_declaration nodes instead.
func findEnclosingClass(node *sitter.Node, source []byte, lang string) string {
	if lang == "go" {
		return findGoReceiverType(node, source)
	}

	ctypes := classContainerTypes[lang]
	if ctypes == nil {
		return ""
	}

	for p := node.Parent(); p != nil; p = p.Parent() {
		if !ctypes[p.Type()] {
			continue
		}
		return extractNodeName(p, source, lang)
	}
	return ""
}

// extractNodeName gets the name of a class/impl node.
func extractNodeName(node *sitter.Node, source []byte, lang string) string {
	// Rust impl blocks use the "type" field instead of "name".
	if lang == "rust" {
		if typeNode := node.ChildByFieldName("type"); typeNode != nil {
			return typeNode.Content(source)
		}
		return ""
	}
	if nameNode := node.ChildByFieldName("name"); nameNode != nil {
		return nameNode.Content(source)
	}
	return ""
}

// findGoReceiverType extracts the receiver type from a Go method_declaration.
// Returns "" for non-method functions.
func findGoReceiverType(node *sitter.Node, source []byte) string {
	if node.Type() != "method_declaration" {
		return ""
	}
	params := node.ChildByFieldName("receiver")
	if params == nil {
		return ""
	}
	return extractGoReceiverType(params, source)
}

// extractGoReceiverType extracts the type name from a Go method receiver parameter list.
// e.g., "(s *Server)" -> "Server", "(s Server)" -> "Server".
func extractGoReceiverType(params *sitter.Node, source []byte) string {
	for i := 0; i < int(params.NamedChildCount()); i++ {
		param := params.NamedChild(int(i))
		if param == nil {
			continue
		}
		typeNode := param.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}
		// Handle pointer receivers: *Server -> Server
		if typeNode.Type() == "pointer_type" {
			inner := typeNode.NamedChild(0)
			if inner == nil {
				continue
			}
			return inner.Content(source)
		}
		return typeNode.Content(source)
	}
	return ""
}
