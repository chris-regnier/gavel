# Task Completion Checklist for Gavel

When completing a development task, follow these steps:

## 1. Code Quality
- [ ] Run `task lint` (go vet) and fix any issues
- [ ] Ensure code follows project conventions (see code_style_and_conventions.md)
- [ ] Check that interfaces are properly used (especially BAMLClient, Store)
- [ ] Verify SARIF extensions use `gavel/` namespace prefix

## 2. BAML Changes
If you modified BAML templates:
- [ ] Run `task generate` to regenerate the Go client
- [ ] Never manually edit `baml_client/` directory
- [ ] Test that generated code compiles and works

## 3. Testing
- [ ] Run `task test` (all tests)
- [ ] Add/update unit tests for new functionality
- [ ] Consider integration test impact
- [ ] Mock external dependencies (LLM calls, filesystem where appropriate)

## 4. Build
- [ ] Run `task build` to ensure binary builds successfully
- [ ] Test the binary manually if changes affect CLI behavior

## 5. Documentation
- [ ] Update README.md if user-facing behavior changed
- [ ] Update CLAUDE.md if architecture or key decisions changed
- [ ] Add code comments for complex logic (but avoid over-commenting)

## 6. Git
- [ ] Commit with clear, descriptive message
- [ ] Ensure `.gitignore` is respected (don't commit generated files unless intended)
- [ ] Consider squashing WIP commits before final push

## Common Gotchas
- Don't forget `task generate` after BAML changes
- SARIF properties must use `gavel/` prefix
- Config merging has specific rules (non-zero strings override)
- Rego policies receive SARIF JSON only, not source code
