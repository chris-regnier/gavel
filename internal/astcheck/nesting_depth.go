package astcheck

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
)

const defaultMaxDepth = 4

// NestingDepth checks that control-flow nesting does not exceed a configurable depth.
type NestingDepth struct{}

func (n *NestingDepth) Name() string { return "nesting-depth" }

func (n *NestingDepth) Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match {
	maxDepth := defaultMaxDepth
	if config != nil {
		if v, ok := config["max_depth"]; ok {
			maxDepth = toInt(v, defaultMaxDepth)
		}
	}

	nodeTypes := nestingNodeTypes(lang)
	if nodeTypes == nil {
		return nil
	}

	var matches []Match
	walkNesting(tree.RootNode(), nodeTypes, 0, maxDepth, &matches)
	return matches
}

func nestingNodeTypes(lang string) map[string]bool {
	switch lang {
	case "go":
		return map[string]bool{
			"if_statement":     true,
			"for_statement":    true,
			"switch_statement": true,
			"select_statement": true,
		}
	case "python":
		return map[string]bool{
			"if_statement":    true,
			"for_statement":   true,
			"while_statement": true,
			"with_statement":  true,
		}
	case "javascript", "typescript":
		return map[string]bool{
			"if_statement":     true,
			"for_statement":    true,
			"for_in_statement": true,
			"while_statement":  true,
			"switch_statement": true,
		}
	case "java":
		return map[string]bool{
			"if_statement":           true,
			"for_statement":          true,
			"enhanced_for_statement": true,
			"while_statement":        true,
			"switch_expression":      true,
		}
	default:
		return nil
	}
}

func walkNesting(node *sitter.Node, nodeTypes map[string]bool, depth, maxDepth int, matches *[]Match) {
	if node == nil {
		return
	}

	currentDepth := depth
	if nodeTypes[node.Type()] {
		currentDepth++
		if currentDepth > maxDepth {
			*matches = append(*matches, Match{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Message:   fmt.Sprintf("nesting depth %d exceeds maximum %d", currentDepth, maxDepth),
				Extra: map[string]interface{}{
					"depth":     currentDepth,
					"max_depth": maxDepth,
				},
			})
			// Don't recurse further into this subtree
			return
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkNesting(node.Child(int(i)), nodeTypes, currentDepth, maxDepth, matches)
	}
}
