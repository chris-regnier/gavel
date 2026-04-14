# GitHub Actions Node 24 Upgrade

**Status:** Approved
**Date:** 2026-04-13
**Issue:** #93
**Scope:** Bump all GitHub Actions to Node-24-compatible versions to eliminate deprecation warnings before the 2026-06-02 forced cutover.

## Background

Every CI and release run emits deprecation warnings: Node.js 20 actions are deprecated. GitHub will force Node 24 by default on 2026-06-02 and remove Node 20 on 2026-09-16. This was deferred from the release pipeline cleanup (#91, see `docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`).

## Design

Mechanical version bump across all 5 workflows. No structural changes to any workflow. No new steps, no removed steps, no config changes.

### Version Upgrades

| Action | Current | Target | Affected workflows |
|--------|---------|--------|--------------------|
| `actions/checkout` | `@v4` | `@v5` | ci, release, docs, benchmark, gavel |
| `actions/setup-go` | `@v5` | `@v6` | ci, release, benchmark, gavel |
| `actions/upload-artifact` | `@v4` | `@v7` | release, benchmark |
| `actions/download-artifact` | `@v4` | `@v8` | release |
| `actions/configure-pages` | `@v5` | `@v6` | docs |
| `actions/upload-pages-artifact` | `@v3` | `@v5` | docs |
| `actions/deploy-pages` | `@v4` | `@v5` | docs |
| `go-task/setup-task` | `@v1` | `@v2` | ci, release, gavel |

### No-Change Actions

- `github/codeql-action/upload-sarif@v4` — already runs Node 24 (v4 was the Node 24 release).
- `.github/actions/install-tools` — composite action (`runs.using: composite`), not a Node action. No change needed.

### Files Changed

1. `.github/workflows/ci.yml` — checkout, setup-go, setup-task
2. `.github/workflows/release.yml` — checkout, setup-go, upload-artifact, download-artifact, setup-task
3. `.github/workflows/docs.yml` — checkout, configure-pages, upload-pages-artifact, deploy-pages
4. `.github/workflows/benchmark.yml` — checkout, setup-go, upload-artifact
5. `.github/workflows/gavel.yml` — checkout, setup-go, setup-task

### Breaking Change Assessment

All target versions are the latest stable major from their respective `actions/` orgs. The workflows use only standard inputs (`go-version`, `cache`, `path`, `name`, `retention-days`, `fetch-depth`) — none of which were removed in any of these major bumps. The primary breaking change across all of them is the Node runtime bump itself (requires runner ≥ v2.327.1, which GitHub-hosted runners already satisfy).

## Acceptance Criteria

- All actions upgraded per the table above.
- CI passes on both matrix legs (ubuntu-latest, macos-latest).
- No Node 20 deprecation warnings in run logs.

## Risks

- **Runner version floor**: Node 24 actions require runner ≥ v2.327.1. GitHub-hosted runners (`ubuntu-latest`, `macos-latest`) already meet this. Self-hosted runners would not — but this project does not use any.
- **upload-artifact v7 / download-artifact v8 compatibility**: These were released the same day (2026-02-26) and use the same underlying `@actions/artifact` library. The release workflow pairs them directly; this pairing is intentional and tested by the actions team.
