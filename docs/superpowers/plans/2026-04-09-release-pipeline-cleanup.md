# Release Pipeline Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the v0.6.0 `linux/arm64` release failure, prevent the class of arch-specific breakage from reaching release tags again, and give developers a single `task check` command that mirrors CI.

**Architecture:** Isolate the `syscall.Dup2` platform quirk into tiny per-OS helper files. Add Taskfile entries that cross-compile release targets with no archiving, and wire those tasks into a new 2-OS CI matrix (`ubuntu-latest` + `macos-latest`). Lock merges to `main` behind the new checks via `gh api` branch protection. Defer the remaining release-hardening wishlist to tracked GitHub issues.

**Tech Stack:** Go 1.25 (CGO enabled), Taskfile v3, GitHub Actions, `golang.org/x/sys/unix`, `gh` CLI.

**Spec:** `docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`

---

## File Structure

**New files:**
- `internal/analyzer/stdoutredirect_linux.go` — `dup2` helper using `unix.Dup3`, build-tagged `linux`.
- `internal/analyzer/stdoutredirect_darwin.go` — `dup2` helper using `syscall.Dup2`, build-tagged `darwin`.

**Modified files:**
- `internal/analyzer/stdoutredirect.go` — swap two `syscall.Dup2` call sites for `dup2`.
- `internal/analyzer/stdoutredirect_test.go` — swap six `syscall.Dup2` call sites for `dup2` so the test file also builds on `linux/arm64`.
- `go.mod`, `go.sum` — `golang.org/x/sys` promotes from indirect to direct.
- `Taskfile.yml` — three new tasks: `check`, `check:cross`, `check:cross:arch`.
- `.github/workflows/ci.yml` — `test` job becomes a 2-OS matrix with a cross-compile step.
- `CLAUDE.md` — single-line addition pointing at `task check`.

**Untouched:**
- `internal/analyzer/stdoutredirect_other.go` — existing `!linux && !darwin` no-op stub stays as-is.
- `.github/workflows/release.yml` — unchanged; the implicit win is that CI now catches arch breakage before the release workflow ever fires.

---

## Task 1: Fix `syscall.Dup2` on `linux/arm64`

**Files:**
- Create: `internal/analyzer/stdoutredirect_linux.go`
- Create: `internal/analyzer/stdoutredirect_darwin.go`
- Modify: `internal/analyzer/stdoutredirect.go`
- Modify: `internal/analyzer/stdoutredirect_test.go`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Create feature branch**

Run:
```bash
git checkout -b release-pipeline-cleanup
```
Expected: switched to new branch.

- [ ] **Step 2: Reproduce the compile failure (no toolchain needed)**

The failure is a compile-time type-check error, not a link error, so `go vet` reproduces it on any host without needing an aarch64 C toolchain:
```bash
GOOS=linux GOARCH=arm64 go vet ./internal/analyzer/
```
Expected output contains:
```
internal/analyzer/stdoutredirect.go:33:20: undefined: syscall.Dup2
internal/analyzer/stdoutredirect.go:41:21: undefined: syscall.Dup2
```
The analyzer package has no direct `import "C"`, so this works on macOS and Linux alike.

- [ ] **Step 3: Create `stdoutredirect_linux.go`**

Write `internal/analyzer/stdoutredirect_linux.go`:
```go
//go:build linux

package analyzer

import "golang.org/x/sys/unix"

// dup2 duplicates oldfd onto newfd with POSIX dup2 semantics.
// syscall.Dup2 is not exposed on linux/arm64 (the kernel only implements
// dup3 there), so this uses unix.Dup3 which is available on every linux arch.
func dup2(oldfd, newfd int) error {
	return unix.Dup3(oldfd, newfd, 0)
}
```

- [ ] **Step 4: Create `stdoutredirect_darwin.go`**

Write `internal/analyzer/stdoutredirect_darwin.go`:
```go
//go:build darwin

package analyzer

import "syscall"

// dup2 duplicates oldfd onto newfd with POSIX dup2 semantics.
// Darwin's syscall package exposes Dup2 directly on both amd64 and arm64.
func dup2(oldfd, newfd int) error {
	return syscall.Dup2(oldfd, newfd)
}
```

- [ ] **Step 5: Update `stdoutredirect.go` to call the helper**

In `internal/analyzer/stdoutredirect.go`, replace the two `syscall.Dup2` call sites with `dup2`. The `syscall` import stays because `syscall.Dup`, `syscall.CloseOnExec`, and `syscall.Close` are still used.

Change line 33 from:
```go
	if err := syscall.Dup2(2, 1); err != nil {
```
to:
```go
	if err := dup2(2, 1); err != nil {
```

Change line 41 from:
```go
		if err := syscall.Dup2(savedFd, 1); err != nil {
```
to:
```go
		if err := dup2(savedFd, 1); err != nil {
```

The `slog.Warn` strings ("failed to dup2 stderr onto stdout for BAML redirect", "failed to restore stdout after BAML redirect") stay unchanged.

- [ ] **Step 6: Update `stdoutredirect_test.go` to call the helper**

The test file also uses `syscall.Dup2` directly, which means it cannot compile on `linux/arm64` either. Replace all six call sites with `dup2` so the package builds cleanly on every arch.

The `syscall` import stays because `syscall.Dup`, `syscall.Close`, and `syscall.Write` are still used.

Change line 25 from:
```go
		syscall.Dup2(origStdoutFd, 1)
```
to:
```go
		dup2(origStdoutFd, 1)
```

Change line 26 from:
```go
		syscall.Dup2(origStderrFd2, 2)
```
to:
```go
		dup2(origStderrFd2, 2)
```

Change line 46 from:
```go
	if err := syscall.Dup2(int(stdoutW.Fd()), 1); err != nil {
```
to:
```go
	if err := dup2(int(stdoutW.Fd()), 1); err != nil {
```

Change line 58 from:
```go
	if err := syscall.Dup2(int(stderrW.Fd()), 2); err != nil {
```
to:
```go
	if err := dup2(int(stderrW.Fd()), 2); err != nil {
```

Change line 82 from:
```go
	syscall.Dup2(origStdoutFd, 1)
```
to:
```go
	dup2(origStdoutFd, 1)
```

Change line 83 from:
```go
	syscall.Dup2(origStderrFd, 2)
```
to:
```go
	dup2(origStderrFd, 2)
```

Note: the test file return values on lines 25, 26, 82, 83 are intentionally discarded (teardown best-effort cleanup). `dup2` returns `error` just like `syscall.Dup2`, so the discards continue to compile. Go will not warn about this because the calls are expression statements, not assignments.

- [ ] **Step 7: Run `go mod tidy`**

Run:
```bash
go mod tidy
```
Expected: `go.mod` gains a direct `require golang.org/x/sys vX.Y.Z` (promoted from indirect). `go.sum` may gain lines. Verify with:
```bash
git diff go.mod
```
Expected: the existing `golang.org/x/sys v0.40.0 // indirect` line loses its `// indirect` comment, or a new direct `require` entry is created alongside.

- [ ] **Step 8: Verify the cross-arch compile error is gone**

Re-run the reproduction command from Step 2:
```bash
GOOS=linux GOARCH=arm64 go vet ./internal/analyzer/
```
Expected: clean exit, no output.

Also verify Darwin still type-checks:
```bash
GOOS=darwin GOARCH=arm64 go vet ./internal/analyzer/
GOOS=darwin GOARCH=amd64 go vet ./internal/analyzer/
```
Expected: all clean.

- [ ] **Step 9: Run existing tests on current host**

Run:
```bash
task test
```
Expected: full test suite passes, including `TestRedirectStdoutToStderr`. Look for `--- PASS: TestRedirectStdoutToStderr` in the output. No failures anywhere.

- [ ] **Step 10: Commit**

```bash
git add internal/analyzer/stdoutredirect.go \
        internal/analyzer/stdoutredirect_linux.go \
        internal/analyzer/stdoutredirect_darwin.go \
        internal/analyzer/stdoutredirect_test.go \
        go.mod go.sum
git commit -m "$(cat <<'EOF'
fix(analyzer): portable dup2 helper for linux/arm64

syscall.Dup2 is not exposed on linux/arm64 (kernel only implements
dup3 there). Split the dup2 call into per-OS helpers: Linux uses
unix.Dup3, Darwin keeps syscall.Dup2. Apply the same swap inside
stdoutredirect_test.go so the analyzer package builds on every arch,
not just the ones CI currently runs on.

Fixes the v0.6.0 release failure.
EOF
)"
```

---

## Task 2: Add Taskfile cross-compile tasks

**Files:**
- Modify: `Taskfile.yml`

- [ ] **Step 1: Add `check:cross` and `check:cross:arch` tasks**

In `Taskfile.yml`, insert the following two task blocks directly after the existing `build:release:arch` block (so the cross-compile verification lives next to the cross-compile release logic it mirrors):

```yaml
  check:cross:
    desc: Cross-compile release targets for current OS (compile-only, no archiving)
    deps: [generate]
    cmds:
      - task: check:cross:arch
        vars: {ARCH: amd64}
      - task: check:cross:arch
        vars: {ARCH: arm64}

  check:cross:arch:
    internal: true
    cmds:
      - |
        CGO_ENABLED=1 GOOS={{.GOOS}} GOARCH={{.ARCH}} CC={{.CC}} go build \
          -o /dev/null \
          ./cmd/gavel
    vars:
      GOOS:
        sh: go env GOOS
      CC:
        sh: |
          if [ "$(go env GOOS)" = "linux" ] && [ "{{.ARCH}}" = "arm64" ]; then
            echo "aarch64-linux-gnu-gcc"
          else
            echo "gcc"
          fi
```

Key choices (matching spec):
- `-o /dev/null` because this is a compile check, not an artifact build.
- No `-ldflags` — version stamping is irrelevant when the binary is thrown away.
- `CC` logic is copy-identical to `build:release:arch` so the two stay in sync.
- `deps: [generate]` ensures `baml_client/` exists before the cross-compile attempts to import it.

- [ ] **Step 2: Add umbrella `check` task**

In `Taskfile.yml`, insert the following block directly after the new `check:cross:arch` block:

```yaml
  check:
    desc: Run all CI checks locally (generate, lint, test, cross-compile)
    deps: [generate]
    cmds:
      - task: lint
      - task: test
      - task: check:cross
```

`deps: [generate]` is necessary because `lint` (`go vet ./...`) and `test` (`go test ./...`) both require `baml_client/` to exist and neither currently triggers regeneration on its own. This mirrors the CI step order (generate → lint → test → cross-compile).

- [ ] **Step 3: List tasks to verify discovery**

Run:
```bash
task --list-all
```
Expected output contains (among others):
```
* check:              Run all CI checks locally (generate, lint, test, cross-compile)
* check:cross:        Cross-compile release targets for current OS (compile-only, no archiving)
```
`check:cross:arch` should NOT appear in the list because it is `internal: true`.

- [ ] **Step 4: Run the cross-compile task**

Run:
```bash
task check:cross
```
Expected on macOS: compiles `darwin/amd64` then `darwin/arm64`, both succeed. Total runtime roughly 3-5 seconds on a warm cache. No tarballs or binaries are left behind (they go to `/dev/null`).
Expected on Linux (with `gcc-aarch64-linux-gnu` installed): compiles `linux/amd64` then `linux/arm64`, both succeed. On Linux without the aarch64 toolchain, the `arm64` leg will fail with `aarch64-linux-gnu-gcc: command not found` — install `gcc-aarch64-linux-gnu` via your package manager before running.

- [ ] **Step 5: Run the full `task check`**

Run:
```bash
task check
```
Expected: `generate` runs, then `lint` (clean), then `test` (all passing), then `check:cross` (both arches). Total runtime dominated by `task test`. Exit code 0.

- [ ] **Step 6: Commit**

```bash
git add Taskfile.yml
git commit -m "$(cat <<'EOF'
build: add task check and task check:cross for local CI parity

task check:cross compiles all release targets for the current host OS
(amd64 + arm64) without producing artifacts, catching arch-specific
breakage at PR time instead of release time. task check wraps lint,
test, and check:cross into a single pre-push command that mirrors CI.
EOF
)"
```

---

## Task 3: Update CI workflow to a 2-OS matrix with cross-compile

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Replace `.github/workflows/ci.yml`**

Overwrite the file with the following content. The changes vs. the current file are: (a) add `strategy.fail-fast: false` and `strategy.matrix.os: [ubuntu-latest, macos-latest]`, (b) change `runs-on` to `${{ matrix.os }}`, (c) add a conditional "Install cross-compile toolchain" step, (d) add a final "Cross-compile release targets" step. All other steps are preserved verbatim.

```yaml
name: CI

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

jobs:
  test:
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache: true

      - name: Install BAML CLI
        run: go install github.com/boundaryml/baml/baml-cli@latest

      - name: Install goimports
        run: go install golang.org/x/tools/cmd/goimports@latest

      - name: Install Task
        uses: go-task/setup-task@v1

      - name: Install cross-compile toolchain
        if: matrix.os == 'ubuntu-latest'
        run: |
          sudo apt-get update
          sudo apt-get install -y gcc-aarch64-linux-gnu

      - name: Generate BAML Client
        run: |
          task generate
          if [ -n "$(git status --porcelain baml_client/)" ]; then
            echo "Error: BAML client code is out of sync. Run 'task generate' locally and commit changes."
            git status baml_client/
            exit 1
          fi

      - name: Run Linter
        run: task lint

      - name: Run Tests
        run: task test

      - name: Build Binary
        run: task build

      - name: Cross-compile release targets
        run: task check:cross
```

- [ ] **Step 2: Validate YAML parses**

Run:
```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo OK
```
Expected: `OK`. If Python/yaml isn't available, open the file visually and verify indentation is consistent (two spaces per level throughout).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "$(cat <<'EOF'
ci: matrix over ubuntu+macos and run task check:cross

CI now runs on both ubuntu-latest and macos-latest with
fail-fast disabled, and verifies cross-compilation of all
release targets via task check:cross. This catches
arch-specific breakage (like the linux/arm64 Dup2 issue that
blocked v0.6.0) on PR instead of at tag push time.

The Linux leg installs gcc-aarch64-linux-gnu for arm64
cross-compilation, matching what the release workflow does.
EOF
)"
```

---

## Task 4: Document `task check` in CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add a line to the Build & Development Commands block**

In `CLAUDE.md`, update the first code fence under `## Build & Development Commands`. Change:
```
task build          # builds dist/gavel for current platform
task test           # go test ./... -v
task lint           # go vet ./...
task generate       # baml-cli generate (regenerates baml_client/ from baml_src/)
```
to:
```
task build          # builds dist/gavel for current platform
task test           # go test ./... -v
task lint           # go vet ./...
task generate       # baml-cli generate (regenerates baml_client/ from baml_src/)
task check          # full CI parity: generate + lint + test + cross-compile (run before pushing)
```

No other part of CLAUDE.md changes.

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): document task check as pre-push CI parity"
```

---

## Task 5: Open PR and verify CI is green

**Files:** none

- [ ] **Step 1: Push the branch**

```bash
git push -u origin release-pipeline-cleanup
```

- [ ] **Step 2: Create the PR**

```bash
gh pr create \
  --title "fix(release): cross-compile safety for linux/arm64 + CI parity" \
  --body "$(cat <<'EOF'
## Summary
- Fix `syscall.Dup2` compile failure on `linux/arm64` that blocked the v0.6.0 release (`internal/analyzer/stdoutredirect.go`).
- Add `task check:cross` + `task check` so developers can reproduce CI locally.
- Add cross-compile verification to CI via a 2-OS matrix (`ubuntu-latest` + `macos-latest`). This catches arch-specific breakage on PR instead of at tag push.

## Design
`docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`

## Deferred (tracked separately)
The spec enumerates seven additional release-hardening improvements (tool pinning, smoke tests, Windows, Node 24 upgrade, host-OS-independent builds, partial-failure handling, dry-run). Those are being filed as separate issues and are out of scope for this PR.

## Test plan
- [x] `task check` passes locally on macOS
- [ ] CI `test (ubuntu-latest)` leg green (proves Linux cross-compile works, including linux/arm64)
- [ ] CI `test (macos-latest)` leg green (proves darwin/amd64 + darwin/arm64 cross-compile)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
Expected: prints the PR URL. Capture the PR number for the next step.

- [ ] **Step 3: Wait for CI to finish**

```bash
gh pr checks --watch
```
Expected: both `test (ubuntu-latest)` and `test (macos-latest)` report `pass`. If either fails, stop and debug — do NOT proceed to subsequent tasks until CI is green.

Common failure modes to expect:
- `task generate` reports `baml_client/` drift: rerun `task generate` locally, commit, push.
- `aarch64-linux-gnu-gcc: command not found`: the toolchain install step is missing or mistyped.
- `go vet` failure from a new dep: check `go.mod` / `go.sum` drift.

- [ ] **Step 4: Merge the PR**

```bash
gh pr merge --squash --delete-branch
```
Expected: squash-merged into `main`. Branch deleted.

- [ ] **Step 5: Pull `main` locally**

```bash
git checkout main
git pull origin main
```
Expected: local `main` now contains the merged commit.

---

## Task 6: Open Option C tracking issues

**Files:** none

Each issue uses the `ci` label (already exists in the repo) so they are discoverable together.

- [ ] **Step 1: Issue 1 — Pin tooling versions**

```bash
gh issue create \
  --title "release: pin baml-cli and goimports versions" \
  --label ci \
  --body "$(cat <<'EOF'
## Problem
Both `.github/workflows/ci.yml` and `.github/workflows/release.yml` install tooling via `@latest`:

```
go install github.com/boundaryml/baml/baml-cli@latest
go install golang.org/x/tools/cmd/goimports@latest
```

This means release builds are not reproducible — a new BAML CLI version can silently break a cut, and CI can go red without any repo change.

## Deferred from
Release pipeline cleanup (see `docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`). The prevent-regression work intentionally kept this out so the Dup2 fix could land fast.

## Acceptance criteria
- `baml-cli` installed at a pinned version (same version across `ci.yml` and `release.yml`, matching what `baml_src/` was generated against)
- `goimports` installed at a pinned version
- A documented upgrade path (e.g., a single constant or a dependabot-ish workflow) so bumping the pin is a one-line change
EOF
)"
```

- [ ] **Step 2: Issue 2 — Post-build smoke test**

```bash
gh issue create \
  --title "release: run built binary as a post-build smoke test" \
  --label ci \
  --body "$(cat <<'EOF'
## Problem
The release workflow produces `gavel_<OS>_<ARCH>` binaries and ships them without executing them. A successful compile does not prove the binary runs — link errors, missing CGO shared libraries, and runtime init panics can all slip through.

## Deferred from
Release pipeline cleanup (see `docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`).

## Acceptance criteria
- For every arch whose native runner is available (`linux/amd64` on `ubuntu-latest`, `darwin/arm64` on `macos-latest`), run `./dist/gavel_<OS>_<ARCH> --version` after build and before upload.
- Fail the release if the smoke test fails.
- arm64 Linux smoke test requires either a native runner (`ubuntu-24.04-arm`) or qemu — pick one and document the trade-off.
EOF
)"
```

- [ ] **Step 3: Issue 3 — Windows release target**

```bash
gh issue create \
  --title "release: add windows/amd64 (and optionally arm64) target" \
  --label ci \
  --body "$(cat <<'EOF'
## Problem
`task build:release` and `.github/workflows/release.yml` only produce Linux and macOS artifacts. Windows users have no supported distribution path.

## Deferred from
Release pipeline cleanup (see `docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`).

## Acceptance criteria
- `task build:release` on Windows produces `gavel_Windows_x86_64.exe` and packages it into `gavel_<version>_Windows_x86_64.zip` (zip, not tar.gz, for Windows convention).
- A `build-windows` job is added to `release.yml` running on `windows-latest`.
- BAML CGO dependency is verified to build cleanly on Windows (or an alternative distribution path — e.g., WSL-only — is documented).
- Optional stretch: `windows/arm64`.
EOF
)"
```

- [ ] **Step 4: Issue 4 — Node 20 action deprecation**

```bash
gh issue create \
  --title "ci: upgrade GitHub Actions off Node 20 before forced cutover" \
  --label ci \
  --body "$(cat <<'EOF'
## Problem
Every CI and release run currently emits deprecation warnings:

> Node.js 20 actions are deprecated. The following actions are running on Node.js 20 and may not work as expected: actions/checkout@v4, actions/setup-go@v5, actions/upload-artifact@v4, go-task/setup-task@v1.

GitHub will force Node 24 by default on 2026-06-02 and remove Node 20 on 2026-09-16.

## Deferred from
Release pipeline cleanup (see `docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`).

## Acceptance criteria
- Upgrade each of the four actions to a Node-24-compatible version (typically a minor/major bump).
- Verify CI and release workflows both pass after the upgrade.
- No more Node 20 deprecation warnings in the run logs.
EOF
)"
```

- [ ] **Step 5: Issue 5 — Host-OS-independent release builds**

```bash
gh issue create \
  --title "release: eliminate host-OS coupling in task build:release" \
  --label ci \
  --body "$(cat <<'EOF'
## Problem
`task build:release` uses `go env GOOS` to pick the target OS, which means:
- A Mac can only build Darwin artifacts.
- A Linux host can only build Linux artifacts.
- The release workflow is forced to keep two parallel jobs (`build-linux`, `build-macos`) and there is no single-command way to rehearse a full release locally.

## Deferred from
Release pipeline cleanup (see `docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`).

## Options
1. Teach `task build:release` to cross the OS boundary using zig cc or osxcross (complex toolchain setup with CGO).
2. Switch to native arm64 runners (`ubuntu-24.04-arm`, `macos-14`) which sidesteps cross-compilation entirely.
3. Migrate to GoReleaser with `goreleaser-cross` Docker image.

## Acceptance criteria
- A single `task build:release` invocation (or a documented equivalent) produces artifacts for all supported OS/arch combinations.
- Or: native runners are adopted and the release workflow has one job per target instead of per host-OS.
- Decision between options is recorded in a new ADR or design doc.
EOF
)"
```

- [ ] **Step 6: Issue 6 — Partial-release failure handling**

```bash
gh issue create \
  --title "release: recover gracefully when one OS leg fails" \
  --label ci \
  --body "$(cat <<'EOF'
## Problem
When the `build-linux` leg failed during the v0.6.0 cut, the `build-macos` leg had already completed and uploaded artifacts — those were then thrown away because the `release` job depends on both. There is no retry path short of deleting the tag and re-pushing it, which rewrites release history.

## Deferred from
Release pipeline cleanup (see `docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`).

## Acceptance criteria
- A failed leg can be re-run without re-running the successful leg (e.g., via `gh workflow run` with job retry, or by caching artifacts across workflow runs tied to the tag SHA).
- Or: the release workflow tolerates partial failures and publishes whatever did build, clearly marked as partial.
- Or: a documented recovery runbook (delete tag, re-push, expected behavior) is added to the repo.
EOF
)"
```

- [ ] **Step 7: Issue 7 — Release dry-run task**

```bash
gh issue create \
  --title "release: add task release:dry-run to rehearse the pipeline" \
  --label ci \
  --body "$(cat <<'EOF'
## Problem
Today the only way to test changes to the release pipeline is to cut a real tag. That means a broken workflow gets discovered at exactly the worst possible moment, and every rehearsal wastes a version number.

## Deferred from
Release pipeline cleanup (see `docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md`).

## Acceptance criteria
- `task release:dry-run` runs everything the release workflow runs, locally, without tagging, pushing, or calling `gh release create`.
- The task produces the same `dist/` layout and checksum file the real release would produce.
- The task works on both macOS and Linux hosts (after Issue 5 lands, ideally both host OSes can produce a full multi-platform `dist/`).
EOF
)"
```

- [ ] **Step 8: Verify all 7 issues landed**

```bash
gh issue list --label ci --limit 20 --state open
```
Expected: 7 new issues visible, titles matching the steps above. Capture their numbers — the next task does not depend on them, but it is useful confirmation.

---

## Task 7: Enable branch protection on `main`

**Files:** none

- [ ] **Step 1: Confirm `main` is currently unprotected (sanity check)**

```bash
gh api repos/chris-regnier/gavel/branches/main/protection 2>&1
```
Expected: `{"message":"Branch not protected",...}` with HTTP 404. If protection already exists (someone set it up while we were working), STOP and fetch the current state with the same command, then merge-edit rather than overwrite.

- [ ] **Step 2: Confirm the required check names exist in the workflow**

Before setting branch protection, verify GitHub has actually seen the new check names on a recent CI run for `main`:
```bash
gh run list --branch main --workflow ci.yml --limit 1 --json databaseId,conclusion,name
gh run view $(gh run list --branch main --workflow ci.yml --limit 1 --json databaseId --jq '.[0].databaseId') --json jobs --jq '.jobs[].name'
```
Expected: the `jobs` output lists `test (ubuntu-latest)` and `test (macos-latest)`. These exact strings are what the next step references as required contexts.

If the check names are different (e.g., GitHub formats them differently), use the observed strings in Step 3 instead.

- [ ] **Step 3: Apply branch protection**

```bash
gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  repos/chris-regnier/gavel/branches/main/protection \
  -F "required_status_checks[strict]=true" \
  -F "required_status_checks[contexts][]=test (ubuntu-latest)" \
  -F "required_status_checks[contexts][]=test (macos-latest)" \
  -F "enforce_admins=false" \
  -F "required_pull_request_reviews[required_approving_review_count]=0" \
  -F "restrictions=null"
```
Expected: JSON response describing the new protection rule, including a `required_status_checks` block with both `contexts` entries, `strict: true`, and `enforce_admins.enabled: false`.

Note on flags: all use uppercase `-F` so `gh` sends them as proper form fields (not JSON strings). This matters for `enforce_admins=false` and `required_pull_request_reviews[required_approving_review_count]=0` because the GitHub API expects a boolean and an integer respectively, not strings.

If `gh` rejects the command with a nested-field error, fall back to piping a JSON body:
```bash
gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  repos/chris-regnier/gavel/branches/main/protection \
  --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["test (ubuntu-latest)", "test (macos-latest)"]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 0
  },
  "restrictions": null
}
EOF
```
This form is unambiguous and matches the GitHub REST API contract exactly.

- [ ] **Step 4: Verify protection is active**

```bash
gh api repos/chris-regnier/gavel/branches/main/protection \
  --jq '{strict: .required_status_checks.strict, contexts: .required_status_checks.contexts, enforce_admins: .enforce_admins.enabled}'
```
Expected output:
```json
{
  "strict": true,
  "contexts": [
    "test (ubuntu-latest)",
    "test (macos-latest)"
  ],
  "enforce_admins": false
}
```

- [ ] **Step 5: Smoke-test that the protection actually gates merges**

Open a trivial test PR to confirm the status-check gate engages. This is optional but recommended on first setup.

```bash
git checkout -b branch-protection-smoke
# Make a no-op change (e.g., trailing newline in a doc file)
echo "" >> docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md
git add docs/superpowers/specs/2026-04-09-release-pipeline-cleanup-design.md
git commit -m "test: verify branch protection gates merge on CI checks"
git push -u origin branch-protection-smoke
gh pr create --title "[TEST] branch protection smoke" --body "Smoke test — close without merging if protection gates as expected"
```

Watch the PR in the browser or via `gh pr view --web`. Confirm:
- "Merge pull request" is disabled until checks pass.
- After both `test (ubuntu-latest)` and `test (macos-latest)` go green, merge is enabled.

Then close the test PR without merging, delete the branch:
```bash
gh pr close branch-protection-smoke --delete-branch
git checkout main
```

Skip this step if you are confident in the protection settings and want to avoid the extra PR noise.

---

## Self-Review Notes

The plan covers every section of the spec:

- Spec §1 (Dup2 fix) → Task 1
- Spec §2 (cross-compile verification in CI) → Tasks 2 + 3
- Spec §3 (local dev workflow parity) → Task 2 Step 2 (`check` umbrella) + Task 4 (CLAUDE.md)
- Spec §4 (required status checks on `main`) → Task 7
- Spec §5 (Option C tracking issues) → Task 6
- Spec Implementation Order → Tasks 1-5 preserve the ordering, Task 6 runs after CI proves the checks are named as expected, Task 7 runs last and includes a fallback for check-name mismatch.

There are no TBD/TODO markers. Every step that writes code shows the exact code. Every command has an expected-output clause. The `dup2` helper signature `(oldfd, newfd int) error` is consistent across all files that reference it (new helpers, production file, test file). Branch-protection check names (`test (ubuntu-latest)`, `test (macos-latest)`) match the GitHub auto-generated matrix display name format, and Task 7 Step 2 verifies them against live API output before trusting them.
