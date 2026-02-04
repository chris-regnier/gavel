# Task Completion Checklist

When completing a task involving code changes, follow this checklist:

## 1. Code Generation (if BAML changes)
If you modified any `.baml` files:
```bash
task generate
```

## 2. Linting
```bash
task lint
```

## 3. Testing
```bash
task test
```

## 4. Build Verification
```bash
task build
```

## 5. Integration Test (if applicable)
If changes affect the full pipeline:
```bash
go test -run TestIntegration -v
```

## Before Committing
- Ensure all tests pass
- Verify build succeeds
- Check that generated BAML client is up to date
- Verify no hardcoded credentials in code
