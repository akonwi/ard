# Contributing

## Commit messages

This repo uses **Conventional Commits**.

Format:

```text
<type>(<scope>)!: <description>
```

Scope and `!` are optional, so these are also valid:

```text
fix: handle nil formatter params
feat(formatter): support inferred closure params
refactor(parser)!: remove deprecated parsing path
```

Allowed types:

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

### Breaking changes

Use `!` after the type or scope when a commit introduces a breaking change:

```text
feat!: change the VM module loading API
fix(parser)!: reject legacy function syntax
```

### CI enforcement

Pull requests validate the **PR title** in CI.
That matches the squash-merge flow, where the PR title becomes the final commit message on `main`.

In practice, the PR title should use the same Conventional Commits format documented above.
