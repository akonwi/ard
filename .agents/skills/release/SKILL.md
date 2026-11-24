---
name: release
description: Cut a new Ard version release end-to-end. Prepares release notes, dispatches build/test jobs, waits for approval, then verifies the workflow-created tags, release, assets, and Homebrew update.
---

# Release (Ard)

## Overview

Releases are prepared and built before a tag exists. A manual `Release Binaries` workflow run tests and builds one exact `main` commit, then pauses at the protected `release` environment. Approval creates the tags and GitHub release together from those tested artifacts. Stable releases update `akonwi/homebrew-tap` afterward; release candidates do not. The Homebrew step refuses to replace a newer or equal formula version.

Each release has two tags at the same commit:

- `vX.Y.Z`: the release tag that the GitHub release and binaries use.
- `compiler/vX.Y.Z`: the Go module tag. The compiler is the Go module `github.com/akonwi/ard/compiler` in the `compiler/` subdirectory, so Go needs this prefixed tag before `go get github.com/akonwi/ard/compiler@vX.Y.Z` works. The Go module proxy caches a version once it is fetched, so this tag must never move.

Do not create or push release tags locally.

## Prerequisites

- Local branch is `main`, clean, and synchronized with `origin/main`.
- All intended changes are merged.
- CI on `main` is green.
- The GitHub `release` environment requires manual approval.
- No other `Release Binaries` run is active or waiting for approval; releases are intentionally serialized.

```bash
git status --short --branch
git log --oneline -5
git tag --sort=-version:refname | head -5
gh run list --workflow "Release Binaries" --limit 5
```

## 1. Pick the next version

Use SemVer:

- Patch: fixes only.
- Minor: backward-compatible features or APIs.
- Major: breaking changes after 1.0; before 1.0, use a minor bump and clearly document breaks.

Supported workflow versions are `vX.Y.Z` and `vX.Y.Z-rcN`.

Inspect the range from the latest stable tag:

```bash
git log <previous-tag>..HEAD --oneline
git diff <previous-tag>..HEAD --stat
```

## 2. Prepare final release notes

Follow the `release-notes` skill. Write final user-facing notes to a temporary file such as:

```text
/tmp/ard-vX.Y.Z-notes.md
```

The notes must be complete before dispatch because the workflow uses them when it creates the release. There should be no post-publication notes-edit step in the normal flow.

## 3. Dispatch from main

```bash
gh workflow run "Release Binaries" \
  --ref main \
  -f version=vX.Y.Z \
  -f notes="$(cat /tmp/ard-vX.Y.Z-notes.md)"
```

Find the run for that version:

```bash
gh run list --workflow "Release Binaries" --limit 10
```

The workflow validates that:

- it was dispatched from `main`;
- the version is valid;
- the `vX.Y.Z` tag, the `compiler/vX.Y.Z` tag, and the release do not already exist.

It then runs tests and builds darwin/linux archives for amd64/arm64. No tags exist yet.

## 4. Inspect and approve

Wait until the `release` job is waiting on the protected `release` environment. Before approving, inspect the successful jobs and, when desired, download/test the uploaded artifacts.

If anything is wrong:

1. Reject or cancel the run.
2. Fix and merge/amend `main`.
3. Dispatch the same version again from the new `main` commit.

Because no tags exist yet, the version remains reusable.

Approve only when the tested artifacts are ready to publish.

## 5. Verify publication

After approval, the release job creates the `vX.Y.Z` tag ref and then the `compiler/vX.Y.Z` tag ref at the exact tested `${{ github.sha }}`, verifying each, and then creates the GitHub release with its assets and final notes. Each ref is created only if it does not exist, so if another actor creates either tag during the approval wait, publication fails rather than using that tag. The two refs are separate API calls, not one atomic update; see Recovery if only the first exists.

Wait for completion:

```bash
gh run watch <run-id> --exit-status
```

Verify the release and compare the tag with the exact workflow-run commit:

```bash
gh release view vX.Y.Z
git fetch --tags
RUN_SHA="$(gh run view <run-id> --json headSha --jq .headSha)"
test "$(git rev-parse vX.Y.Z^{commit})" = "$RUN_SHA"
test "$(git rev-parse compiler/vX.Y.Z^{commit})" = "$RUN_SHA"
```

Optionally confirm the Go module resolves (this also warms the module proxy):

```bash
GOFLAGS=-mod=mod go list -m github.com/akonwi/ard/compiler@vX.Y.Z
```

All jobs must pass:

- `validate`
- `test`
- four platform `build` jobs
- `release`
- `update-homebrew-tap` for stable releases

Release candidates are marked prerelease and skip Homebrew.

## Recovery

Never move an existing release tag. Inspect the publication state before acting:

- **No tags and no release:** fix the cause and dispatch the whole workflow again.
- **Only one of `vX.Y.Z` and `compiler/vX.Y.Z` exists, at the tested run SHA:** create the missing tag at that same SHA, then continue with the next case if the release is also missing. The workflow creates `vX.Y.Z` first, so a failed run normally leaves only that tag; the reverse means another actor or a manual step created `compiler/vX.Y.Z`, which is still fine if it points at `RUN_SHA`:

  ```bash
  # Use refs/tags/vX.Y.Z instead if the release tag is the missing one.
  gh api --method POST repos/akonwi/ard/git/refs \
    --field ref=refs/tags/compiler/vX.Y.Z --field sha="$RUN_SHA"
  ```

- **Tags exist at the tested run SHA but no release:** download the original run artifacts with `gh run download <run-id>` and create the release for the existing `vX.Y.Z` tag. Do not rebuild from another commit.
- **Release exists but assets are incomplete:** download the original run artifacts and upload the missing assets to the same release.
- **Either tag exists at any other SHA:** stop; do not publish or move the tag. Investigate the conflicting publication.
- **Only Homebrew failed:** rerun or repair the Homebrew step without recreating the tag or release.

Always compare every existing tag (`vX.Y.Z` and `compiler/vX.Y.Z`) against the run's `headSha` before recovery.
