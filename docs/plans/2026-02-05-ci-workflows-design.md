# CI/CD Workflows Design

**Date:** 2026-02-05
**Status:** Approved

## Overview

Implement GitHub Actions workflows for Gavel using a two-workflow approach:
1. **PR Workflow** (ci.yml) - Standard validation on PRs to main
2. **Release Workflow** (release.yml) - Tag-triggered releases with GoReleaser

## Requirements

- Test on PR to main
- Build and release when cutting a tag
- Standard validation only (no self-gating with gavel yet - requires solving model access in CI)

## Design Decisions

### Approach: Separate PR and Release Workflows

**Rationale:**
- Clear separation of concerns
- Faster PR feedback (no release overhead)
- Easier to troubleshoot and evolve independently
- Standard pattern in the ecosystem

### PR Workflow (ci.yml)

**Triggers:**
- Pull requests targeting `main`
- Pushes to `main` (post-merge verification)

**Steps:**
1. Checkout with full history
2. Set up Go 1.25
3. Install BAML CLI
4. Install Task
5. Generate BAML client and verify no uncommitted changes
6. Run linter (`task lint`)
7. Run tests (`task test`)
8. Build binary (`task build`)

**Key features:**
- Go module caching via `actions/setup-go@v5`
- BAML generation verification prevents stale generated code
- Sequential execution for clarity

### Release Workflow (release.yml)

**Updates to existing workflow:**
1. Add BAML generation step before tests
2. Uncomment GoReleaser step

**Triggers:**
- Version tags (`v*`)

**Behavior:**
- Runs full test suite
- Generates BAML client code
- GoReleaser builds multi-platform binaries
- Creates GitHub release with changelog
- Uploads artifacts

## Future Enhancements

Not implementing now, but documented for future consideration:

1. **Matrix testing** - Test against multiple Go versions
2. **Coverage reporting** - Upload to Codecov/Coveralls
3. **Self-gating** - Run gavel analysis on own PRs (requires model access solution)
4. **Dependabot** - Auto-update GitHub Actions versions

## Estimated CI Runtime

- PR workflow: ~2-3 minutes (with caching)
- Release workflow: ~5-10 minutes (includes multi-platform builds)

## Secrets Required

- PR workflow: None
- Release workflow: `GITHUB_TOKEN` (auto-provided by GitHub Actions)
