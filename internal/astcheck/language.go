package astcheck

import (
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	typescript "github.com/smacker/go-tree-sitter/typescript/typescript"
)

type langEntry struct {
	language *sitter.Language
	name     string
}

var extToLang map[string]langEntry

func init() {
	extToLang = map[string]langEntry{
		".go":  {language: golang.GetLanguage(), name: "go"},
		".py":  {language: python.GetLanguage(), name: "python"},
		".js":  {language: javascript.GetLanguage(), name: "javascript"},
		".jsx": {language: javascript.GetLanguage(), name: "javascript"},
		".ts":  {language: typescript.GetLanguage(), name: "typescript"},
		".tsx": {language: typescript.GetLanguage(), name: "typescript"},
		".java": {language: java.GetLanguage(), name: "java"},
		".c":   {language: c.GetLanguage(), name: "c"},
		".h":   {language: c.GetLanguage(), name: "c"},
		".rs":  {language: rust.GetLanguage(), name: "rust"},
	}
}

// Detect returns the tree-sitter Language, language name, and whether the
// file extension was recognized.
func Detect(path string) (*sitter.Language, string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	entry, ok := extToLang[ext]
	if !ok {
		return nil, "", false
	}
	return entry.language, entry.name, true
}
