package astcheck

// DefaultRegistry returns a Registry pre-loaded with all built-in AST checks.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&FunctionLength{})
	r.Register(&NestingDepth{})
	r.Register(&EmptyHandler{})
	r.Register(&ParamCount{})
	return r
}
