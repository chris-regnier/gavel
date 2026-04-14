# GitHub Actions Node 24 Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade all GitHub Actions to Node-24-compatible versions to eliminate deprecation warnings before the 2026-06-02 forced cutover.

**Architecture:** Mechanical version bump across 5 workflow files. No structural changes.

**Tech Stack:** GitHub Actions YAML

---

### Task 1: Upgrade CI workflow

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Bump action versions in ci.yml**

Apply these version changes:

```yaml
# Line 18: actions/checkout@v4 → actions/checkout@v5
- uses: actions/checkout@v5

# Line 21: actions/setup-go@v5 → actions/setup-go@v6
- uses: actions/setup-go@v6

# Line 30: go-task/setup-task@v1 → go-task/setup-task@v2
- uses: go-task/setup-task@v2
```

- [ ] **Step 2: Verify YAML is valid**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: upgrade ci.yml actions to Node 24 (#93)"
```

### Task 2: Upgrade release workflow

**Files:**
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: Bump action versions in release.yml**

Apply these version changes:

```yaml
# build-linux job:
# Line 15: actions/checkout@v4 → actions/checkout@v5
- uses: actions/checkout@v5

# Line 21: actions/setup-go@v5 → actions/setup-go@v6
- uses: actions/setup-go@v6

# Line 34: go-task/setup-task@v1 → go-task/setup-task@v2
- uses: go-task/setup-task@v2

# Line 43: actions/upload-artifact@v4 → actions/upload-artifact@v7
- uses: actions/upload-artifact@v7

# build-macos job:
# Line 52: actions/checkout@v4 → actions/checkout@v5
- uses: actions/checkout@v5

# Line 58: actions/setup-go@v5 → actions/setup-go@v6
- uses: actions/setup-go@v6

# Line 65: go-task/setup-task@v1 → go-task/setup-task@v2
- uses: go-task/setup-task@v2

# Line 74: actions/upload-artifact@v4 → actions/upload-artifact@v7
- uses: actions/upload-artifact@v7

# release job:
# Line 85: actions/checkout@v4 → actions/checkout@v5
- uses: actions/checkout@v5

# Line 91: actions/download-artifact@v4 → actions/download-artifact@v8
- uses: actions/download-artifact@v8

# Line 96: actions/download-artifact@v4 → actions/download-artifact@v8
- uses: actions/download-artifact@v8
```

- [ ] **Step 2: Verify YAML is valid**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: upgrade release.yml actions to Node 24 (#93)"
```

### Task 3: Upgrade docs workflow

**Files:**
- Modify: `.github/workflows/docs.yml`

- [ ] **Step 1: Bump action versions in docs.yml**

Apply these version changes:

```yaml
# build job:
# Line 25: actions/checkout@v4 → actions/checkout@v5
- uses: actions/checkout@v5

# Line 27: actions/configure-pages@v5 → actions/configure-pages@v6
- uses: actions/configure-pages@v6

# Line 29: actions/upload-pages-artifact@v3 → actions/upload-pages-artifact@v5
- uses: actions/upload-pages-artifact@v5

# deploy job:
# Line 41: actions/deploy-pages@v4 → actions/deploy-pages@v5
- uses: actions/deploy-pages@v5
```

- [ ] **Step 2: Verify YAML is valid**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/docs.yml'))"`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/docs.yml
git commit -m "ci: upgrade docs.yml actions to Node 24 (#93)"
```

### Task 4: Upgrade benchmark workflow

**Files:**
- Modify: `.github/workflows/benchmark.yml`

- [ ] **Step 1: Bump action versions in benchmark.yml**

Apply these version changes:

```yaml
# Line 27: actions/checkout@v4 → actions/checkout@v5
- uses: actions/checkout@v5

# Line 30: actions/setup-go@v5 → actions/setup-go@v6
- uses: actions/setup-go@v6

# Line 57: actions/upload-artifact@v4 → actions/upload-artifact@v7
- uses: actions/upload-artifact@v7
```

- [ ] **Step 2: Verify YAML is valid**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/benchmark.yml'))"`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/benchmark.yml
git commit -m "ci: upgrade benchmark.yml actions to Node 24 (#93)"
```

### Task 5: Upgrade gavel workflow

**Files:**
- Modify: `.github/workflows/gavel.yml`

- [ ] **Step 1: Bump action versions in gavel.yml**

Apply these version changes:

```yaml
# gate job:
# Line 19: actions/checkout@v4 → actions/checkout@v5
- uses: actions/checkout@v5

# Line 119: github/codeql-action/upload-sarif@v4 — NO CHANGE (already Node 24)

# benchmark job:
# Line 143: actions/checkout@v4 → actions/checkout@v5
- uses: actions/checkout@v5

# Line 149: actions/setup-go@v5 → actions/setup-go@v6
- uses: actions/setup-go@v6

# Line 155: go-task/setup-task@v1 → go-task/setup-task@v2
- uses: go-task/setup-task@v2
```

- [ ] **Step 2: Verify YAML is valid**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/gavel.yml'))"`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/gavel.yml
git commit -m "ci: upgrade gavel.yml actions to Node 24 (#93)"
```

### Task 6: Verify no Node 20 actions remain

- [ ] **Step 1: Grep for old version references**

Run: `grep -rn '@v[0-9]' .github/workflows/ | grep -v 'codeql-action/upload-sarif@v4'`

Verify that no lines reference the old versions (`checkout@v4`, `setup-go@v5`, `upload-artifact@v4`, `download-artifact@v4`, `configure-pages@v5`, `upload-pages-artifact@v3`, `deploy-pages@v4`, `setup-task@v1`).

- [ ] **Step 2: Run local CI check**

Run: `task check`
Expected: all checks pass (generate, lint, test, cross-compile)
