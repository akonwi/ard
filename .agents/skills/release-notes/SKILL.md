---
name: release-notes
description: Draft final user-facing Ard release notes before dispatching the build-before-tag release workflow, or exceptionally repair notes on an existing release.
---

# Release Notes

## Overview

Write final release notes for an Ard compiler/language version. Notes target language users, not contributors.

In the normal release flow, notes are completed before the `Release Binaries` workflow is dispatched. The workflow receives them as its `notes` input and publishes them when it creates the tag and release after build approval.

## 1. Identify the version range

Use the latest stable release tag, ignoring prerelease tags when appropriate:

```bash
git tag --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -5
```

## 2. Generate the diff

```bash
git log <previous-tag>..HEAD --oneline
git diff <previous-tag>..HEAD --stat
```

Inspect detailed source diffs when commit subjects are insufficient.

## 3. Classify changes

Use applicable categories and omit empty ones:

- **New Features**
- **Improvements**
- **Bug Fixes**
- **Breaking Changes**
- **Migration Guide**

## 4. Write final notes

Suggested shape:

```markdown
## Overview

One or two sentences describing the release theme.

## <Category>

### <Change Title>

Explain what changed and why it matters to users.

\`\`\`ard
// practical example when useful
\`\`\`

---

**Release**: vX.Y.Z<br>
**Commit**: <short commit>
```

Guidelines:

- Explain user-visible behavior, not internal implementation work.
- Include practical examples where syntax or migration changed.
- Do not describe tests unless they communicate compatibility confidence.
- Clearly call out breaking changes and provide before/after migration examples.
- Keep notes concise and complete; normal releases do not edit notes after publication.

Write them to a temporary file:

```bash
cat > /tmp/ard-vX.Y.Z-notes.md <<'EOF'
...
EOF
```

The release skill passes this file to the workflow:

```bash
gh workflow run "Release Binaries" \
  --ref main \
  -f version=vX.Y.Z \
  -f notes="$(cat /tmp/ard-vX.Y.Z-notes.md)"
```

## Exceptional repair

If an already-published release has incorrect notes, repair them explicitly:

```bash
gh release edit vX.Y.Z --notes-file /tmp/ard-vX.Y.Z-notes.md
```

This is recovery, not the normal release sequence. Never recreate or move the tag merely to update notes.
