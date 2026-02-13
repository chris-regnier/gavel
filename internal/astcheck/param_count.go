package astcheck

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
)

const defaultMaxParams = 5

// ParamCount checks that functions do not have too many parameters.
type ParamCount struct{}

func (p *ParamCount) Name() string { return "param-count" }

func (p *ParamCount) Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match {
	maxParams := defaultMaxParams
	if config != nil {
		if v, ok := config["max_params"]; ok {
			maxParams = toInt(v, defaultMaxParams)
		}
	}

	nodeTypes := funcNodeTypes(lang)
	if nodeTypes == nil {
		return nil
	}

	var matches []Match
	findNodes(tree.RootNode(), nodeTypes, func(node *sitter.Node) {
		paramsNode := node.ChildByFieldName("parameters")
		if paramsNode == nil {
			return
		}

		count := countParams(paramsNode, lang, source)
		if count > maxParams {
			name := funcName(node, source)
			matches = append(matches, Match{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
				Message:   fmt.Sprintf("function %q has %d parameters (max %d)", name, count, maxParams),
				Extra: map[string]interface{}{
					"function":   name,
					"param_count": count,
					"max_params":  maxParams,
				},
			})
		}
	})

	return matches
}

func countParams(paramsNode *sitter.Node, lang string, source []byte) int {
	switch lang {
	case "go":
		return countGoParams(paramsNode)
	default:
		return countGenericParams(paramsNode, lang)
	}
}

// countGoParams handles Go's grouped parameter declarations (e.g. `a, b int`).
// Each parameter_declaration may contain multiple identifiers.
func countGoParams(paramsNode *sitter.Node) int {
	count := 0
	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		child := paramsNode.NamedChild(int(i))
		if child == nil || child.Type() != "parameter_declaration" {
			continue
		}
		// Count identifier nodes within each parameter_declaration
		idCount := 0
		for j := 0; j < int(child.NamedChildCount()); j++ {
			grandchild := child.NamedChild(int(j))
			if grandchild != nil && grandchild.Type() == "identifier" {
				idCount++
			}
		}
		// If no identifiers found (e.g. unnamed params like `int`), count as 1
		if idCount == 0 {
			count++
		} else {
			count += idCount
		}
	}
	return count
}

// countGenericParams counts parameter nodes for non-Go languages.
func countGenericParams(paramsNode *sitter.Node, lang string) int {
	paramTypes := paramNodeTypes(lang)
	if paramTypes == nil {
		// Fallback: count all named children
		return int(paramsNode.NamedChildCount())
	}

	count := 0
	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		child := paramsNode.NamedChild(int(i))
		if child != nil && paramTypes[child.Type()] {
			count++
		}
	}
	return count
}

func paramNodeTypes(lang string) map[string]bool {
	switch lang {
	case "python":
		return map[string]bool{
			"identifier":                 true,
			"default_parameter":          true,
			"typed_parameter":            true,
			"typed_default_parameter":    true,
		}
	case "javascript":
		return map[string]bool{
			"identifier":          true,
			"assignment_pattern":  true,
			"rest_pattern":        true,
		}
	case "typescript":
		return map[string]bool{
			"identifier":          true,
			"assignment_pattern":  true,
			"rest_pattern":        true,
			"required_parameter":  true,
			"optional_parameter":  true,
		}
	case "java":
		return map[string]bool{
			"formal_parameter": true,
			"spread_parameter": true,
		}
	case "c":
		return map[string]bool{
			"parameter_declaration": true,
		}
	case "rust":
		return map[string]bool{
			"parameter": true,
		}
	default:
		return nil
	}
}
