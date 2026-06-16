---
title: Dependencies
description: Add, lock, fetch, and use Ard package dependencies.
---

Ard dependencies are declared in `ard.toml`, resolved in `ard.lock`, and restored into a shared cache. Builds use the lockfile and cache; they do not fetch or update dependencies automatically.

## Declaring dependencies

A project can depend on Git packages or local path packages:

```toml
name = "my_app"
ard = ">= 0.13.0"

[dependencies]
vaxis = { git = "https://github.com/akonwi/vaxis-ard.git", commit = "76f7c1b" }
shared = { path = "../shared" }
```

The dependency key is the import root visible to the package that declares it:

```ard
use vaxis
use shared/strings
```

Dependency aliases are package-local. Your root project can import only its direct dependencies. A dependency's own dependencies are available to that dependency, but they are not automatically re-exported into your root project's import namespace.

## Adding a Git dependency

Use `ard add` to add or update a Git dependency:

```sh
ard add github.com/akonwi/vaxis-ard@v0.1.0
ard add github.com/akonwi/vaxis-ard@76f7c1b as tui
ard add github.com/akonwi/vaxis-ard@latest
```

`ard add`:

1. resolves the requested tag, commit, or `latest` ref,
2. updates `ard.toml`,
3. writes the resolved graph to `ard.lock`, and
4. fetches Git dependencies into Ard's shared cache.

To update an existing dependency, run `ard add` again with a new tag, commit, or `latest`.

## Lockfile and cache

`ard.lock` records the exact package graph used by the project, including resolved commits and cache integrity hashes. Commit `ard.lock` with your project.

Git dependencies are restored into Ard's shared cache:

```text
~/.ard/cache/git/<source-hash>/<commit>/
```

You can override the cache location with `ARD_CACHE_DIR`, which is useful for CI or tests.

Ordinary commands such as `ard check`, `ard run`, `ard build`, and `ard test` expect locked dependencies to already be present in the cache. If a cache entry is missing, run:

```sh
ard deps fetch
```

To verify that cached dependencies match `ard.lock`, run:

```sh
ard deps verify
```

## Removing a dependency

Remove a direct dependency with:

```sh
ard remove vaxis
```

This removes the dependency entry from `ard.toml` and prunes unreachable packages from `ard.lock`.

## Local path dependencies

Path dependencies point directly at a local package root:

```toml
[dependencies]
shared = { path = "../shared" }
```

They are resolved from the path in `ard.toml` and are not copied into the cache. Path dependencies are useful for local development across multiple packages.

## FFI companions

Dependency packages may own Go FFI companions, such as `ffi/*.go`. The compiler routes extern calls to the package that declares them, so a dependency can provide both its Ard API and host-language implementation.

## No default vendoring

Ard no longer uses project-local `.ard/vendor` as the normal dependency source. Optional vendoring may return later as an export/offline workflow, but the default build input is `ard.lock` plus the shared cache.
