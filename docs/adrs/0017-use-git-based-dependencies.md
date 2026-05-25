# 0017: Use Vendored Git-Based Dependencies

## Status

Accepted

## Context

Ard currently uses file-based modules and absolute imports rooted at the current project. Standard library modules live under the `ard/*` namespace, and project modules are resolved from the project root. ADR 0013 notes that future dependency management must preserve or intentionally extend this absolute import model.

Ard now primarily emits Go for native execution, but the language also supports JavaScript targets. Because dependencies may contain Ard source, target-specific extern companions, and target-specific stdlib or runtime expectations, Ard needs a dependency model that is not solely delegated to Go modules. Go modules remain important for generated Go builds and Go FFI companions, but they cannot be the only package-resolution mechanism for a multi-target Ard project.

The earlier dependency-system sketch proposed a Zig-inspired package model with direct Git dependencies, lockfile-backed resolution, a global cache, multiple-version support, and vendoring. After review, the initial Ard dependency model should be simpler: dependencies should be materialized into the project, and vendored dependency content should be the source of truth used by builds.

Ard needs a dependency model that fits the language's explicit module system without introducing a registry requirement or hiding dependency identity behind implicit global resolution.

## Decision

Adopt a separate Ard dependency system based on direct Git and local path dependencies with project-local vendoring.

Project dependencies are declared explicitly in `ard.toml`:

```toml
[dependencies]
vaxis = { git = "https://github.com/ard/vaxis.git", tag = "v0.1.0" }
local_utils = { path = "../shared-utils" }
```

The dependency key in the root project's manifest is the import root used by Ard source:

```ard
use vaxis
use local_utils/strings
```

The dependency package's own manifest name does not override the root project's chosen alias. This keeps imports explicit from the depending project's point of view and lets users rename dependencies locally.

Dependencies are materialized under the project directory:

```text
.ard/vendor/
```

Vendored dependencies are the source used by the module resolver for external packages. The initial model does not require a separate `ard.lock`: the pinned version is the source declared in `ard.toml`, and the actual build input is the vendored content in `.ard/vendor/`.

Vendored dependencies should not auto-update during ordinary builds. `ard build` should use existing vendored dependencies and fail clearly when a required dependency is missing. Dependencies can be added and removed with:

```bash
ard add github.com/akonwi/vaxis-ard@76f7c1b
ard add github.com/akonwi/vaxis-ard@latest
ard remove vaxis
```

`ard add` writes a pinned dependency entry to `ard.toml` and materializes it into `.ard/vendor/`. By default, the dependency alias starts as the repository name and is overridden by the dependency's own `name` property in its `ard.toml` when present. `latest` is resolved to the current remote commit before writing the dependency; branch pins are not part of the model because vendored content should be tied to a stable commit or tag. `ard remove` deletes the dependency entry and removes the matching `.ard/vendor/<alias>` directory.

The explicit command for rematerializing existing dependencies is:

```bash
ard deps fetch
```

This command reads `ard.toml`, fetches Git or local path dependencies, and writes the selected dependency contents into `.ard/vendor/`. Changing dependency versions is an explicit act: edit `ard.toml` or rerun `ard add`, then rerun `ard deps fetch`.

A central registry is explicitly out of scope. The initial dependency system should support Git sources and local paths, not package publication, registry lookup, or registry trust policy.

Transitive dependencies are resolved and vendored, but their aliases are private to the dependency that declared them. The root project can import only its direct dependency aliases unless it declares additional direct dependencies. This avoids flattening all transitive aliases into the root namespace.

Vendored dependencies may include target-specific companion files, such as Go FFI companions. Dependency resolution must preserve package ownership so externs can be routed to the companion package that owns them instead of flattening all non-stdlib externs into the root project's FFI package.

For Go output, extern ownership should route calls along these lines:

```text
ard/http        -> stdlib FFI package
main.ard        -> root project FFI package
vaxis           -> dependency-owned FFI package for alias vaxis
```

Generated Go builds should incorporate Go module requirements needed by dependency FFI companions, such as a vendored Ard package's `go.mod`, while Ard package resolution remains controlled by `ard.toml` and `.ard/vendor/`.

Multiple versions of the same dependency are deferred. The initial model should resolve one version per dependency identity in a project and report clear conflicts when resolution cannot choose a single version. Coexisting multiple-version graphs may be revisited after the core dependency model is proven.

The first implementation should prioritize:

- parsing dependency declarations in `ard.toml`
- adding Git dependencies through `ard add`
- resolving Git and local path dependencies through an explicit fetch command
- materializing dependencies into `.ard/vendor/`
- resolving imports through direct manifest aliases
- preserving dependency owner identity through module resolution and lowering
- routing dependency-owned FFI companions separately from root project FFI
- failing clearly when vendored dependencies are missing or stale relative to `ard.toml`

More advanced features, such as rich conflict analysis, update impact reports, private repository credential management, mirrors, lazy fetching, per-dependency metadata files, integrity hashes, global shared caches, and multi-version import namespaces, can be designed after the core model is proven.

## Consequences

- Ard has a target-independent dependency system that can serve both Go and JavaScript builds.
- Ard does not need a central package registry to support reusable packages.
- Projects can depend directly on Git repositories pinned by tag or commit, plus local paths.
- Project-local `.ard/vendor/` keeps dependency state close to the project and avoids requiring a global cache or lockfile.
- Vendored content is authoritative, so ordinary builds are reproducible as long as `.ard/vendor/` is checked in or otherwise restored.
- Updating dependencies is explicit and does not happen as a side effect of building.
- Import resolution must distinguish project modules, standard library modules, and direct dependency aliases while preserving Ard's absolute import style.
- Module resolution and AIR lowering must preserve whether a module belongs to the root project, stdlib, or a dependency alias.
- Transitive dependencies remain implementation details unless promoted to direct dependencies by the root project.
- Deferring multiple-version support keeps the initial resolver and type identity model simpler.
- The compiler and tooling need new commands for fetching, verifying, listing, and updating dependencies.
- Generated Go projects still need to interoperate with Go modules for Go runtime support and Go FFI companion dependencies, including dependency-owned FFI companions, but Go modules are not the source of truth for Ard package resolution.

## Related

- `docs/adrs/0013-use-file-based-modules-and-absolute-imports.md`
- `docs/adrs/0015-use-esm-javascript-targets-with-explicit-runtime-semantics.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
