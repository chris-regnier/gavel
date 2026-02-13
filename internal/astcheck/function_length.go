package astcheck

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"
)

const defaultMaxLines = 50

// FunctionLength checks that functions do not exceed a configurable line count.
type FunctionLength struct{}

func (f *FunctionLength) Name() string { return "function-length" }

func (f *FunctionLength) Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match {
	maxLines := defaultMaxLines
	if config != nil {
		if v, ok := config["max_lines"]; ok {
			maxLines = toInt(v, defaultMaxLines)
		}
	}

	nodeTypes := funcNodeTypes(lang)
	if nodeTypes == nil {
		return nil
	}

	var matches []Match
	findNodes(tree.RootNode(), nodeTypes, func(node *sitter.Node) {
		startRow := int(node.StartPoint().Row)
		endRow := int(node.EndPoint().Row)
		lineCount := endRow - startRow + 1

		if lineCount > maxLines {
			name := funcName(node, source)
			matches = append(matches, Match{
				StartLine: startRow + 1, // 1-indexed
				EndLine:   endRow + 1,
				Message:   fmt.Sprintf("function %q is %d lines long (max %d)", name, lineCount, maxLines),
				Extra: map[string]interface{}{
					"function":   name,
					"line_count": lineCount,
					"max_lines":  maxLines,
				},
			})
		}
	})

	return matches
}

// toInt converts an interface{} to int, supporting int, float64, and int64.
func toInt(v interface{}, fallback int) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case int64:
		return int(val)
	default:
		return fallback
	}
}
