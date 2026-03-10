---
name: release-notes
description: Generate release notes for a new Ard version tag. Compares the diff between two tags, summarizes user-facing changes (features, fixes, breaking changes), and publishes via `gh release edit`. Use when a new version tag has been pushed and needs release notes.
---

# Release Notes

## Overview

Write release notes for a new Ard compiler/language version. Notes target **end-users** of the language and compiler — not internal contributors.

## Workflow

### 1. Identify the version range

Determine the new tag and the previous tag:

```bash
git tag --sort=-creatordate | head -5
```

### 2. Generate the diff

Get the commit log and diff summary between the two tags:

```bash
git log <previous-tag>..<new-tag> --oneline
git diff <previous-tag>..<new-tag> --stat
```

For detailed changes when needed:

```bash
git diff <previous-tag>..<new-tag> -- compiler/std_lib compiler/ffi compiler/checker compiler/bytecode/vm compiler/parse
```

### 3. Classify changes

Organize changes into these categories (omit empty categories):

- **New Features** — new language features, stdlib additions, CLI commands
- **Improvements** — enhancements to existing features, performance, ergonomics
- **Bug Fixes** — corrected behavior, resolved crashes or type errors
- **Breaking Changes** — anything requiring user code updates
- **Migration Guide** — only if there are breaking changes; show before/after examples

### 4. Write the notes

Follow this template:

```markdown
## Overview

One or two sentences summarizing the release theme.

## <Category>

### <Change Title>

Brief description of what changed and why it matters to users.

Code examples showing the new usage (when applicable):

\`\`\`ard
// example
\`\`\`
```

#### Style guidelines

- **Audience**: Ard language users, not compiler contributors
- **Tone**: Present features positively; explain what users can now do
- **Code examples**: Show practical usage, not internal implementation
- **Tests**: Don't describe test additions unless they demonstrate confidence in a fix
- **Internal refactors**: Omit unless they have user-visible impact
- **Keep it concise**: A few sentences per change is usually enough
- **Version and commit**: End with the version tag and commit hash

### 5. Publish

Update the existing GitHub release with the notes:

```bash
gh release edit <tag> --notes "$(cat notes.md)"
```

Or if no release exists yet:

```bash
gh release create <tag> --title "<tag>" --notes "$(cat notes.md)"
```

## Reference

Look at previous releases for tone and structure:

```bash
gh release view <previous-tag>
```
