package astcheck

import (
	"sort"

	sitter "github.com/smacker/go-tree-sitter"
)

// Check is the interface that all AST-based checks must implement.
type Check interface {
	// Name returns the unique identifier for this check (e.g. "function-length").
	Name() string
	// Run executes the check against a parsed tree-sitter tree.
	// lang is the language name (e.g. "go", "python").
	// config provides check-specific configuration (may be nil).
	Run(tree *sitter.Tree, source []byte, lang string, config map[string]interface{}) []Match
}

// Match represents a single finding from an AST check.
type Match struct {
	StartLine int
	EndLine   int
	Message   string
	Extra     map[string]interface{}
}

// Registry holds a set of named AST checks.
type Registry struct {
	checks map[string]Check
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{checks: make(map[string]Check)}
}

// Register adds a check to the registry, keyed by its Name().
func (r *Registry) Register(c Check) {
	r.checks[c.Name()] = c
}

// Get retrieves a check by name.
func (r *Registry) Get(name string) (Check, bool) {
	c, ok := r.checks[name]
	return c, ok
}

// Names returns all registered check names in sorted order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.checks))
	for name := range r.checks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
