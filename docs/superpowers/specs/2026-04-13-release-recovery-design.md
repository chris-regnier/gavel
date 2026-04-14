# Release Recovery Design

**Issue:** #92 — release: recover gracefully when one OS leg fails
**Date:** 2026-04-13

## Problem

When one build leg (e.g. `build-linux`) fails during a release, the other leg's artifacts are uploaded but discarded because the `release` job depends on both. The only recovery path is deleting and re-pushing the tag.

## Solution

Two independent recovery paths, both achieved through changes to `release.yml`:

1. **Fast path — "Re-run failed jobs":** Increase artifact retention from 1 day to 5 days so GitHub's built-in "Re-run failed jobs" button works reliably within a comfortable window (covers weekends).

2. **Slow path — manual `workflow_dispatch`:** Add a `workflow_dispatch` trigger with a required `tag` input so the entire workflow can be re-triggered against any tag without deleting/re-pushing it. This covers the case where artifacts have already expired.

## Changes to `release.yml`

All changes are confined to `.github/workflows/release.yml`. No new files or jobs.

### 1. Add `workflow_dispatch` trigger

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

### 2. Actor gate on all jobs

Restrict manual dispatch to `chris-regnier`:

```yaml
if: github.event_name != 'workflow_dispatch' || github.actor == 'chris-regnier'
```

Applied to all three jobs (`build-linux`, `build-macos`, `release`).

### 3. Tag resolution

Compute a single `TAG` value that works for both trigger types:

```yaml
env:
  TAG: ${{ github.event.inputs.tag || github.ref_name }}
```

### 4. Checkout with explicit ref

All checkout steps use `ref: ${{ env.TAG }}` to ensure manual dispatch builds the correct commit.

### 5. Release step uses resolved tag

`gh release create` uses `${{ env.TAG }}` instead of `${{ github.ref_name }}`.

### 6. Artifact retention

Bump `retention-days` from 1 to 5 on both `upload-artifact` steps.

## Recovery Scenarios

| Scenario | Action |
|---|---|
| One leg fails, noticed within 5 days | Click "Re-run failed jobs" in GitHub UI |
| One leg fails, artifacts expired | Trigger workflow manually with the tag via Actions tab |
| Both legs fail | Either re-run all jobs or trigger manually |
| Unauthorized user tries manual dispatch | All jobs skip silently (actor gate) |
