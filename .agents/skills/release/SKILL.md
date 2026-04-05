---
name: release
description: Cut a new Ard version release end-to-end. Creates and pushes a new version tag, waits for the Release Binaries workflow to build and publish the GitHub release, then drafts and updates release notes. Use when a batch of changes is ready to ship as a new version.
---

# Release (Ard)

## Overview

Cut a new version of the Ard compiler and publish release notes. This skill handles the full flow: tag → CI → release notes. For notes-only updates to an existing release, use the `release-notes` skill instead.

## Prerequisites

- On `main`, working tree clean, up to date with `origin/main`
- All PRs intended for this release are already merged
- CI is currently green on `main`

Sanity-check first:

```bash
git status --short --branch
git log --oneline -5
```

## Workflow

### 1. Pick the next version

List existing tags and decide on the next semver:

```bash
git tag --sort=-v:refname | head -5
```

Version bump rules:
- **Patch** (`0.13.0` → `0.13.1`) — bug fixes only, no new APIs
- **Minor** (`0.13.0` → `0.14.0`) — new features, stdlib additions, backward-compatible changes
- **Major** (`0.x.y` → `1.0.0`) — breaking changes (Ard is pre-1.0, so minor bumps may include breaks; call them out clearly)

Quick survey of what's in the release before deciding:

```bash
git log <previous-tag>..HEAD --oneline
```

If ambiguous, ask the user to confirm the version.

### 2. Tag and push

```bash
git tag v<major>.<minor>.<patch>
git push origin v<major>.<minor>.<patch>
```

### 3. Wait for the release workflow

Pushing a `v*` tag triggers `.github/workflows/build.yml` ("Release Binaries"). It runs tests, builds darwin/linux (amd64/arm64) binaries, and creates a GitHub release with the assets attached.

Wait for it to finish before drafting notes (otherwise `gh release edit` may race with the workflow's `gh release create`):

```bash
gh run watch $(gh run list --workflow "Release Binaries" --limit 1 --json databaseId --jq '.[0].databaseId')
```

Or poll manually:

```bash
gh run list --workflow "Release Binaries" --limit 1
```

All jobs (`test`, `build (darwin/linux × amd64/arm64)`, `release`) must be green. If the workflow fails, stop and investigate — do not proceed to notes.

### 4. Draft release notes

Follow the `release-notes` skill for tone, structure, and publishing:

- Audience: Ard language users, not contributors
- Categories: New Features, Improvements, Bug Fixes, Breaking Changes, Migration Guide (only when relevant)
- Reference previous releases for tone: `gh release view <previous-tag>`
- End with version tag and commit hash

Generate the diff and classify:

```bash
git log <previous-tag>..v<new> --oneline
git diff <previous-tag>..v<new> --stat
```

Write notes to a temp file (e.g. `/tmp/ard-v<new>-notes.md`).

### 5. Publish notes

Update the release the workflow already created:

```bash
gh release edit v<new> --notes "$(cat /tmp/ard-v<new>-notes.md)"
```

Verify:

```bash
gh release view v<new>
```

## Notes

- This skill assumes the workflow creates the release. Do not call `gh release create` — it will conflict with the CI job.
- If the workflow races ahead and you need to rewrite notes later, that's what the `release-notes` skill is for.
- If the tag was pushed but the workflow hasn't started yet, give GitHub a few seconds and re-query `gh run list`.
- Do not tag from a feature branch. Always tag from `main`.
