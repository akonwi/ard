# 0025: Use a Lockfile-Backed Dependency Cache

## Status

Accepted

## Context

ADR 0017 accepted project-local vendoring for Ard dependencies. That model was intentionally simple: dependencies are declared in `ard.toml`, materialized under `.ard/vendor/<alias>`, and the vendored tree is the source used by ordinary builds.

The implementation proved that Ard can resolve external packages, preserve package ownership for type and FFI purposes, and support reusable Ard packages without a central registry. However, making `.ard/vendor/` the source of truth has practical drawbacks:

- each project duplicates dependency source trees
- builds depend on mutable project-local vendor state rather than a resolved graph
- missing or stale vendor directories are easy to create
- it is unclear whether `.ard/vendor/` should be committed, restored, ignored, or regenerated
- transitive dependencies are difficult to model without either flattening aliases or recursively vendoring nested trees
- updates and removals mutate copied source trees instead of operating on a dependency graph
- dependency identity can accidentally drift toward filesystem paths instead of package identity

The alternative considered before ADR 0017 was closer to Zig-style dependency management: direct Git dependencies, lockfile-backed resolution, a shared cache, optional vendoring, and eventually multiple-version support. That approach is now a better fit for Ard because the compiler has clearer AIR boundaries, package-owned FFI routing, and enough module ownership tracking to support a package graph.

This decision must preserve the important constraints from earlier ADRs:

- ADR 0013: imports are absolute, file-based, and project/package-root oriented rather than relative
- ADR 0008: extern companions are target-aware and owned by stdlib, the root project, or a dependency package
- ADR 0015: JavaScript emits multi-file ESM and needs target-aware companion modules
- ADR 0002: AIR is the backend boundary and should receive an already-linked, target-neutral program graph

## Decision

Replace project-local vendoring as the default dependency source with a lockfile-backed package graph and a shared immutable dependency cache.

Ard projects continue to declare direct dependencies in `ard.toml`:

```toml
[dependencies]
ui = { git = "https://github.com/akonwi/vaxis-ard.git", tag = "v0.1.0" }
shared = { path = "../shared" }
```

The dependency key is the alias visible to the package that declares it. Aliases are local to the declaring package and are not flattened globally. A root package can import `ui/widgets`, while the dependency itself continues to import its own modules using its own manifest name and imports its own direct dependencies through its own dependency aliases.

Resolved dependency state lives in an `ard.lock` file. The lockfile records the complete package graph needed by the root project, including:

- package IDs
- manifest names
- dependency aliases visible from each package
- normalized Git source URLs
- requested refs, when useful for humans
- resolved commits for Git dependencies
- optional content/tree hashes for verification
- canonical paths for local path dependencies

Git dependencies are fetched into a shared cache instead of `.ard/vendor/`, for example:

```text
~/.ard/cache/git/<source-hash>/<commit>/
```

Cache entries are immutable build inputs. A build does not update, fetch, or rewrite dependencies. If the lockfile references a missing cache entry, build/check/run/test commands fail clearly and suggest an explicit restore command.

The dependency commands should become graph operations:

```bash
ard add github.com/akonwi/vaxis-ard@v0.1.0
ard add github.com/akonwi/vaxis-ard@76f7c1b as ui
ard deps fetch
ard deps verify
ard remove ui
```

`ard add` resolves the requested ref, updates `ard.toml`, updates `ard.lock`, and fetches the package into the cache. Existing dependencies are updated by running `ard add` again with a new hash, tag, or `latest`. `ard deps fetch` restores all locked Git dependencies into the cache. `ard deps verify` checks that cached content matches the lockfile. `ard remove` updates the manifest and prunes unreachable lockfile entries.

Project-local vendoring is no longer the default model. An optional future `ard deps vendor` command may export locked cache entries into a project-local directory for hermetic/offline workflows, but ordinary compiler resolution should use the lockfile and cache.

### Package graph and import resolution

The compiler should construct a package graph before checking user modules. A package graph entry should contain at least:

```go
type PackageID string

type PackageInfo struct {
    ID           PackageID
    Name         string
    RootPath     string
    Dependencies map[string]PackageID
}
```

Import resolution becomes package-context-aware:

```text
ResolveImport(importerPackageID, "ui/widgets")
```

Resolution rules are:

1. `ard/*` resolves to the standard library.
2. If the first path segment is the importing package's manifest name, resolve within that package root.
3. If the first path segment is a dependency alias declared by the importing package, resolve within that dependency package root.
4. Otherwise, report an unknown import root for the importing package.

The root package can only import direct dependency aliases it declares. Transitive aliases remain private to the packages that declared them unless the root also declares them directly.

### Module, type, and method identity

Compiler identity should be based on package ownership, not vendor paths or local aliases. A module identity is conceptually:

```text
(packageID, modulePathWithinPackage)
```

Named type owners, method owners, visibility checks, and AIR module metadata should derive from this canonical identity. Display paths may still use user-facing import aliases, but semantic identity must not depend on the alias chosen by the root project.

A transition implementation may encode canonical module identities as strings, but the long-term compiler model should carry structured `PackageID` / `ModuleID` information through checker and AIR metadata.

### Linking dependencies

Ard dependency linking means loading all reachable modules from the root package graph into one checked program graph, then lowering that checked graph to one AIR program.

The checker/module loader should:

- start from the root input module and root package ID
- resolve imports with the importing module's package context
- cache loaded modules by canonical module identity
- detect cycles across canonical module identities
- preserve package ownership on modules, types, methods, extern declarations, and globals

AIR lowering should receive an already-linked checked module graph and preserve package/module ownership in AIR metadata that targets need.

### Target FFI linking

Extern ownership is package-owned:

```text
ard/http       -> stdlib FFI package
root modules   -> root project FFI package
ui/widgets     -> dependency package FFI companion for package ui's PackageID
```

The Go backend should copy/read dependency Go FFI companions from the package root in the cache or local path dependency, not from `.ard/vendor`. It should also merge Go module requirements and replacements from dependency package roots when generated code actually needs them.

The JavaScript backend should route dependency-owned externs to dependency-owned target companion modules instead of treating all non-stdlib externs as root project FFI. Generated ESM imports should distinguish stdlib, root project, and dependency package companions.

### Version and conflict policy

The initial lockfile resolver should choose one version for each normalized Git source identity in a package graph and report clear conflicts when requirements cannot be unified. Multiple coexisting versions of the same package source are deferred until the core graph/cache model is proven.

A central package registry, package publication, trust policy, private repository credential management, mirrors, and global update policies are out of scope for this decision.

## Consequences

- Ard dependencies become reproducible build inputs described by `ard.lock`, not mutable copied source trees.
- Projects no longer need to commit or manage `.ard/vendor/` for ordinary builds.
- CI and offline workflows must restore the dependency cache explicitly with `ard deps fetch` or an equivalent cache restore step.
- Dependency aliases remain local and explicit, preserving Ard's absolute import style without flattening transitive dependency namespaces.
- The compiler must become package-graph aware before module checking, which is a larger architectural change than the current root-only resolver.
- Type and method identity must use package/module ownership so renamed dependencies and same-named modules from different packages remain distinct.
- Backends must link package-owned extern companions from package roots in the cache or path dependency roots.
- Optional vendoring can still be added later as an export mode, but it is not the source of truth.
- ADR 0017 should be superseded when this decision is accepted and implemented.

## Related

- `docs/adrs/0017-use-git-based-dependencies.md`
- `docs/adrs/0013-use-file-based-modules-and-absolute-imports.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0015-use-esm-javascript-targets-with-explicit-runtime-semantics.md`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
