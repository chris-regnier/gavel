package astcheck

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// EmptyHandler checks for empty error/exception handling blocks.
type EmptyHandler struct{}

func (e *EmptyHandler) Name() string { return "empty-handler" }

func (e *EmptyHandler) Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match {
	switch lang {
	case "go":
		return e.checkGo(tree.RootNode(), source)
	case "python":
		return e.checkPython(tree.RootNode(), source)
	case "javascript", "typescript", "java":
		return e.checkCatchClause(tree.RootNode(), source)
	default:
		return nil
	}
}

// checkGo finds `if err != nil { }` blocks with empty bodies.
func (e *EmptyHandler) checkGo(root *sitter.Node, source []byte) []Match {
	var matches []Match
	nodeTypes := map[string]bool{"if_statement": true}

	findNodes(root, nodeTypes, func(node *sitter.Node) {
		condNode := node.ChildByFieldName("condition")
		if condNode == nil {
			return
		}
		condText := strings.TrimSpace(condNode.Content(source))
		if condText != "err != nil" {
			return
		}

		consNode := node.ChildByFieldName("consequence")
		if consNode == nil {
			return
		}
		if consNode.NamedChildCount() == 0 {
			matches = append(matches, Match{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Message:   fmt.Sprintf("empty error handler at line %d", node.StartPoint().Row+1),
				Extra: map[string]interface{}{
					"pattern": "if err != nil {}",
				},
			})
		}
	})

	return matches
}

// checkPython finds `except: pass` blocks.
func (e *EmptyHandler) checkPython(root *sitter.Node, source []byte) []Match {
	var matches []Match
	nodeTypes := map[string]bool{"except_clause": true}

	findNodes(root, nodeTypes, func(node *sitter.Node) {
		// The except_clause's body is typically a block child.
		// We look for a body that contains only a pass_statement.
		bodyNode := findChildBlock(node)
		if bodyNode == nil {
			return
		}

		if bodyNode.NamedChildCount() == 1 {
			child := bodyNode.NamedChild(0)
			if child != nil && child.Type() == "pass_statement" {
				matches = append(matches, Match{
					StartLine: int(node.StartPoint().Row) + 1,
					EndLine:   int(node.EndPoint().Row) + 1,
					Message:   fmt.Sprintf("empty except handler (pass) at line %d", node.StartPoint().Row+1),
					Extra: map[string]interface{}{
						"pattern": "except: pass",
					},
				})
			}
		}
	})

	return matches
}

// findChildBlock finds the block child node within an except_clause.
func findChildBlock(node *sitter.Node) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil && child.Type() == "block" {
			return child
		}
	}
	return nil
}

// checkCatchClause finds catch blocks with empty bodies (JS/TS/Java).
func (e *EmptyHandler) checkCatchClause(root *sitter.Node, source []byte) []Match {
	var matches []Match
	nodeTypes := map[string]bool{"catch_clause": true}

	findNodes(root, nodeTypes, func(node *sitter.Node) {
		bodyNode := node.ChildByFieldName("body")
		if bodyNode == nil {
			return
		}
		if bodyNode.NamedChildCount() == 0 {
			matches = append(matches, Match{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Message:   fmt.Sprintf("empty catch handler at line %d", node.StartPoint().Row+1),
				Extra: map[string]interface{}{
					"pattern": "catch {}",
				},
			})
		}
	})

	return matches
}
