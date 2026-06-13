---
name: commit-format
description: Ard commit and PR title format. Use before creating commits, rewriting commit messages, or opening/updating PRs.
---

# Commit Format

Ard enforces Conventional Commit headers for commit messages and PR titles.

## Required header format

```text
<type>(<scope>)!: <description>
```

The scope and breaking-change marker are optional, so these are valid:

```text
feat(lsp): add member completions
fix(checker): continue after invalid call arity
docs: document release flow
refactor(parser)!: simplify function parsing
```

## Allowed types

- `build`
- `chore`
- `ci`
- `docs`
- `feat`
- `fix`
- `perf`
- `refactor`
- `revert`
- `style`
- `test`

## Issue references

When a commit fully fixes a tracked GitHub issue, include a closing reference in the commit body, e.g.:

```text
Fixes #218
```

Use a non-closing reference such as `Refs #218` when the commit is related but does not fully resolve the issue.

## Validation

Before pushing commits or creating/updating a PR title, validate the header with:

```bash
scripts/validate-commit-message.sh --message "feat(lsp): add member completions"
```

For commit message files, use:

```bash
scripts/validate-commit-message.sh .git/COMMIT_EDITMSG
```

## PR titles

PR titles are checked with the same format. Use a Conventional Commit title such as:

```text
feat(lsp): add navigation and editing features
```
