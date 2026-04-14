# Release Pipeline Cleanup

**Status:** Draft
**Date:** 2026-04-09
**Scope:** Prevent-regression fix for the v0.6.0 release failure, plus local-dev parity with CI.

## Background

The v0.6.0 tag failed to publish. The `build-linux` job errored during `linux/arm64` cross-compilation:

```
internal/analyzer/stdoutredirect.go:33: undefined: syscall.Dup2
internal/analyzer/stdoutredirect.go:41: undefined: syscall.Dup2
```

Root cause: `syscall.Dup2` is not exposed on `linux/arm64` (that kernel only implements `dup3`). The file uses `//go:build linux || darwin` with no arch discrimination. The file was added in 66c8eda for an LSP stdout-redirect fix and never built on arm64 because CI only runs on `ubuntu-latest` (amd64) — arch-specific breakage goes undetected until a tag is pushed.

The broader problem this exposes: **the release workflow is the only place cross-compilation happens.** A clean CI run does not imply a clean release.

## Goals

1. Fix the `Dup2` bug so `linux/arm64` builds again.
2. Catch any future arch-specific breakage on PR, not on tag push.
3. Let developers reproduce the full CI check suite locally via Task before pushing.
4. Require the new CI checks to pass before merges to `main`.
5. Preserve the list of deferred improvements (Option C) as tracked GitHub issues so they are not forgotten.

## Non-Goals

- Windows support, GoReleaser migration, native arm64 runners, Node-24 action upgrades, tool pinning, post-build smoke tests, dry-run mode — all deferred to tracked issues (see Issue List below). They are listed to ensure they are remembered; they are not part of this cleanup.
- Modifying the release workflow structure. The release workflow stays as-is except for the implicit win that CI will now catch arch breakage before it reaches the release job.

## Design

### 1. Fix `syscall.Dup2` on `linux/arm64`

The `redirectStdoutToStderr` function in `internal/analyzer/stdoutredirect.go` needs a single primitive that duplicates one fd onto another. The Go stdlib does not expose this portably, so we introduce a tiny per-OS helper and call it from the shared logic.

**File changes:**

- `internal/analyzer/stdoutredirect.go` — retains the full mutex/save/restore logic. Replaces the two `syscall.Dup2(...)` call sites with `dup2(...)`, an unexported helper.
- `internal/analyzer/stdoutredirect_linux.go` (new) — build tag `linux`. Defines `dup2(oldfd, newfd int) error` as a thin wrapper around `golang.org/x/sys/unix.Dup3(oldfd, newfd, 0)`. `unix.Dup3` is available on every linux arch.
- `internal/analyzer/stdoutredirect_darwin.go` (new) — build tag `darwin`. Defines `dup2` as a thin wrapper around `syscall.Dup2`, which IS available on Darwin.

The helper signature `dup2(oldfd, newfd int) error` matches POSIX `dup2` semantics so the shared code reads naturally. The platform quirk is isolated to one line per OS.

**Dependency:** `golang.org/x/sys` is already in `go.mod` as an indirect dependency (v0.40.0). Promoting it to a direct dep is automatic when the new file imports `golang.org/x/sys/unix` and `go mod tidy` runs.

**Testing:** `internal/analyzer/stdoutredirect_test.go` (if it does not already exist) gets a test that verifies fd 1 actually points at stderr's inode between `redirectStdoutToStderr()` and the returned restore, and is restored afterward. This test runs under `linux` and `darwin` build tags — no arch-specific guards — because the public behavior should be identical. The cross-compile matrix in CI (below) is what actually exercises the `linux/arm64` build path.

### 2. Cross-compile verification in CI

Two new Taskfile entries and a CI matrix update.

**Taskfile (`Taskfile.yml`):**

```yaml
check:cross:
  desc: Cross-compile all release targets for current OS (no archiving)
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

Key points:
- Output goes to `/dev/null` — this is a compile check, not an artifact build. Fast (~1.5s per arch on warm cache vs ~30s for the full release build pipeline).
- Reuses the same `GOOS` / `CC` logic as `build:release:arch` so the two stay in sync. If someone changes the cross-toolchain in the future, both tasks pick it up.
- On Darwin hosts, cross-compile stays within Darwin (amd64 + arm64). On Linux hosts, within Linux. This matches the current release workflow topology — we do not attempt to cross the OS boundary, which would require osxcross.

**CI workflow (`.github/workflows/ci.yml`):**

The existing `test` job becomes a matrix over `ubuntu-latest` and `macos-latest`:

```yaml
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

The existing steps (checkout through `task build`) are preserved verbatim; the only additions are the `strategy` block at the top, the `Install cross-compile toolchain` step (Linux only), and the final `Cross-compile release targets` step.

`fail-fast: false` so a failure on one OS does not mask a different failure on the other. Both legs must pass before merge (see Part 4).

### 3. Local dev workflow parity

One umbrella task that runs everything CI runs:

```yaml
check:
  desc: Run all CI checks locally (lint, test, cross-compile)
  deps: [generate]
  cmds:
    - task: lint
    - task: test
    - task: check:cross
```

`deps: [generate]` is present because neither `lint` nor `test` currently trigger BAML regeneration on their own (only `build` does), and `go vet` / `go test` will fail on a fresh checkout without `baml_client/`. Running `generate` once at the top of `check` mirrors the order CI uses.

`task check` is the "am I ready to push?" command. It mirrors exactly one leg of the CI matrix — the one matching the developer's host OS. It does not attempt to run the other OS's leg (that would require a VM or remote runner) and does not try to run `task build` since `check:cross` already proves the current OS's native target builds.

CLAUDE.md gets a one-line update under "Build & Development Commands" pointing at `task check` as the pre-push equivalent of CI.

### 4. Required status checks on `main`

After the CI change is merged, branch protection on `main` needs to be updated to require:

- `test (ubuntu-latest)`
- `test (macos-latest)`

This is applied via `gh api`, run after the workflow change lands (so the check names exist in the repo's history and GitHub will accept them as required):

```bash
gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  repos/chris-regnier/gavel/branches/main/protection \
  -f "required_status_checks[strict]=true" \
  -f "required_status_checks[contexts][]=test (ubuntu-latest)" \
  -f "required_status_checks[contexts][]=test (macos-latest)" \
  -F "enforce_admins=false" \
  -f "required_pull_request_reviews[required_approving_review_count]=0" \
  -f "restrictions=null"
```

Notes:
- `enforce_admins=false` keeps the existing admin-override behavior (repo owner can push directly in an emergency). Adjustable if preferred.
- `required_pull_request_reviews[required_approving_review_count]=0` preserves auto-merge for self-authored PRs without adding a review requirement — the user asked for tests as the gate, not reviews. If there is an existing review requirement, it will be overwritten; the command above is explicit about the intended state.
- `strict=true` means PRs must be up-to-date with `main` before merging. This matches the goal: a green check against stale main does not prove anything.
- The exact command will be confirmed against current protection state before running (via `gh api repos/.../branches/main/protection`) so we do not clobber unrelated settings.

### 5. Deferred improvements — tracked as issues

Each of the following becomes a separate GitHub issue on `chris-regnier/gavel`, linking back to this spec. Labels are left off unless a matching one already exists in the repo (to be checked at issue-creation time).

1. **Pin tooling versions.** Replace `baml-cli@latest` and `goimports@latest` in both `ci.yml` and `release.yml` with exact versions. Reproducible builds.
2. **Post-build smoke test.** Run `./dist/gavel_<OS>_<ARCH> --version` on every produced binary to catch link errors or missing CGO shared libs before publishing.
3. **Windows release target.** Add `windows/amd64` (and optionally `windows/arm64`) to `build:release` and the release workflow.
4. **Node 20 action deprecation.** Upgrade `actions/checkout`, `actions/setup-go`, `actions/upload-artifact`, `go-task/setup-task` to Node-24-compatible versions before the forced cutover.
5. **Host-OS-independent release builds.** Either teach `task build:release` to build both OSes from one host (osxcross / zig cc), or migrate to native arm64 runners (`ubuntu-24.04-arm`) or GoReleaser so cross-compile pain disappears.
6. **Partial-release failure handling.** Currently when one OS leg fails, the other leg's artifacts are uploaded and then discarded. Define a retry path (re-run failed job) or a safe-restart story that does not require deleting and re-pushing the tag.
7. ~~**Release dry-run task.**~~ Implemented in #91. `task release:dry-run` runs tests, builds release binaries, generates a changelog, and shows what `gh release create` would run — all without tagging, pushing, or creating a release.

Each issue body includes: the problem, why we deferred it from this cleanup, a brief acceptance criterion, and a link to this spec.

## Implementation Order

1. Dup2 fix (unblocks v0.6.0 re-cut)
2. Taskfile tasks: `check:cross`, `check:cross:arch`, `check`
3. CI workflow matrix update
4. CLAUDE.md one-line note about `task check`
5. Open Option C issues
6. Merge, verify CI is green on both matrix legs
7. Run `gh api` to set branch protection
8. Re-cut the release tag (separate decision — not part of this cleanup)

## Risks & Open Questions

- **`gh api` branch protection is shared state.** The user has approved this action explicitly, but the command will be displayed and confirmed once more at runtime, with current protection state fetched first, to avoid clobbering any unrelated settings that may have been added since.
- **Check names are fragile.** `test (ubuntu-latest)` and `test (macos-latest)` are GitHub's auto-generated matrix display names. If the matrix key or values change, the required-check names change and branch protection silently stops enforcing. This risk is accepted for now; a follow-up could pin display names via `name:` on the job.
- **`task check` on an M-series Mac runs darwin/arm64 twice (native + cross) and darwin/amd64 once.** Fine — it is still catching breakage, just not the minimum possible work. Optimizing this is out of scope.
- ~~**Rehearsal before tag cut.**~~ Resolved — `task release:dry-run` now provides local rehearsal (#91).
