# Release Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the release workflow recoverable when one OS build leg fails, without deleting/re-pushing tags.

**Architecture:** Add `workflow_dispatch` trigger with actor gate and bump artifact retention. All changes in one file.

**Tech Stack:** GitHub Actions YAML

---

### Task 1: Add `workflow_dispatch` trigger and tag resolution

**Files:**
- Modify: `.github/workflows/release.yml:1-10`

**Spec ref:** Sections 1, 3 of the design spec.

- [ ] **Step 1: Add `workflow_dispatch` trigger with tag input**

Replace the current `on:` block (lines 3-6):

```yaml
on:
  push:
    tags:
      - 'v*'
```

With:

```yaml
on:
  push:
    tags:
      - 'v*'
  workflow_dispatch:
    inputs:
      tag:
        description: 'Git tag to release (e.g. v0.6.0)'
        required: true
        type: string
```

- [ ] **Step 2: Add top-level `env` block for tag resolution**

Add after the `permissions:` block (after line 9), before `jobs:`:

```yaml
env:
  TAG: ${{ inputs.tag || github.ref_name }}
```

This resolves to the `workflow_dispatch` input when present, or the pushed tag ref otherwise.

- [ ] **Step 3: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"`

Expected: no output (valid YAML).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci(release): add workflow_dispatch trigger with tag input (#92)"
```

---

### Task 2: Add actor gate to all jobs

**Files:**
- Modify: `.github/workflows/release.yml` (all three job definitions)

**Spec ref:** Section 2 of the design spec.

- [ ] **Step 1: Add `if` condition to `build-linux` job**

Add immediately after the `build-linux:` job key, before `runs-on:`:

```yaml
  build-linux:
    if: github.event_name != 'workflow_dispatch' || github.actor == 'chris-regnier'
    runs-on: ubuntu-latest
```

- [ ] **Step 2: Add `if` condition to `build-macos` job**

Add immediately after the `build-macos:` job key, before `runs-on:`:

```yaml
  build-macos:
    if: github.event_name != 'workflow_dispatch' || github.actor == 'chris-regnier'
    runs-on: macos-latest
```

- [ ] **Step 3: Add `if` condition to `release` job**

Add immediately after the `release:` job key, before `needs:`. Note: when a job has both `if` and `needs`, `if` must come first for readability but the order doesn't matter to GitHub Actions. Place it before `needs:`:

```yaml
  release:
    if: github.event_name != 'workflow_dispatch' || github.actor == 'chris-regnier'
    needs: [build-linux, build-macos]
```

- [ ] **Step 4: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"`

Expected: no output (valid YAML).

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci(release): restrict workflow_dispatch to chris-regnier (#92)"
```

---

### Task 3: Update checkout steps to use resolved tag

**Files:**
- Modify: `.github/workflows/release.yml` (all three checkout steps)

**Spec ref:** Section 4 of the design spec.

- [ ] **Step 1: Update `build-linux` checkout to use TAG**

Change the checkout step in `build-linux` from:

```yaml
      - name: Checkout
        uses: actions/checkout@v5
        with:
          fetch-depth: 0
```

To:

```yaml
      - name: Checkout
        uses: actions/checkout@v5
        with:
          ref: ${{ env.TAG }}
          fetch-depth: 0
```

- [ ] **Step 2: Update `build-macos` checkout to use TAG**

Same change — add `ref: ${{ env.TAG }}` to the checkout step in `build-macos`:

```yaml
      - name: Checkout
        uses: actions/checkout@v5
        with:
          ref: ${{ env.TAG }}
          fetch-depth: 0
```

- [ ] **Step 3: Update `release` checkout to use TAG**

Same change — add `ref: ${{ env.TAG }}` to the checkout step in `release`:

```yaml
      - name: Checkout
        uses: actions/checkout@v5
        with:
          ref: ${{ env.TAG }}
          fetch-depth: 0
```

- [ ] **Step 4: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"`

Expected: no output (valid YAML).

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci(release): checkout explicit tag ref for dispatch support (#92)"
```

---

### Task 4: Update release step and bump artifact retention

**Files:**
- Modify: `.github/workflows/release.yml` (upload-artifact steps and gh release create)

**Spec ref:** Sections 5, 6 of the design spec.

- [ ] **Step 1: Update `gh release create` to use TAG env var**

In the `release` job, change the `Create Release` step from:

```yaml
          gh release create ${{ github.ref_name }} \
            --title "Gavel ${{ github.ref_name }}" \
```

To:

```yaml
          gh release create ${{ env.TAG }} \
            --title "Gavel ${{ env.TAG }}" \
```

- [ ] **Step 2: Bump Linux artifact retention from 1 to 5 days**

In `build-linux`, change:

```yaml
          retention-days: 1
```

To:

```yaml
          retention-days: 5
```

- [ ] **Step 3: Bump macOS artifact retention from 1 to 5 days**

In `build-macos`, change:

```yaml
          retention-days: 1
```

To:

```yaml
          retention-days: 5
```

- [ ] **Step 4: Validate YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"`

Expected: no output (valid YAML).

- [ ] **Step 5: Final review — read the complete file and verify all changes are consistent**

Run: `cat .github/workflows/release.yml`

Verify:
- `on:` block has both `push.tags` and `workflow_dispatch.inputs.tag`
- Top-level `env.TAG` references `inputs.tag || github.ref_name`
- All three jobs have the actor gate `if` condition
- All three checkout steps have `ref: ${{ env.TAG }}`
- `gh release create` uses `${{ env.TAG }}`
- Both upload-artifact steps have `retention-days: 5`

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci(release): use resolved tag and bump artifact retention to 5 days (#92)"
```
