# 0049: Overlay Ard Intrinsics on an Explicit Standard Library Package

## Status

Proposed

## Context

Ard currently treats the entire `ard/*` import namespace as compiler-owned standard library space. The compiler embeds ordinary Ard modules from `compiler/std_lib`, intercepts `ard/*` imports before normal package resolution, and gives that namespace priority over dependency aliases.

That model is useful for module-shaped operations whose behavior is intrinsic to the language or backend, such as `ard/async` and `ard/unsafe`. It is unnecessarily restrictive for ordinary Ard library code. Pure Ard modules can use the normal package graph, lockfile, cache, tests, and release process without being tied to compiler releases.

New libraries are already being developed as standalone Ard projects. Some of them may eventually belong to a coherent standard library with familiar imports such as:

```ard
use ard/decode
use ard/list
```

The namespace should support both compiler intrinsics and an independently versioned standard library package. Projects should opt into ordinary standard library modules explicitly instead of receiving a hidden compiler dependency.

Gleam provides a useful precedent: core language semantics remain compiler-owned, while the separately versioned `gleam_stdlib` package provides normal `gleam/*` modules and is declared as a project dependency.

## Decision

Reserve exact compiler-intrinsic module paths within `ard/*`, while allowing every other `ard/*` path to resolve from a normal, explicitly declared dependency package named or aliased `ard`.

The compiler will no longer treat the `ard/` prefix by itself as proof that a module is compiler-owned.

### Compiler intrinsics

The compiler maintains an explicit registry of intrinsic module paths, conceptually:

```go
var intrinsicModules = map[string]IntrinsicModule{
    "ard/async":  AsyncPkg,
    "ard/unsafe": UnsafePkg,
}
```

The exact registry is an implementation detail, but membership is by complete module path rather than prefix. Builtin types, primitive methods, collection operations, and other prelude behavior that do not require imports remain compiler-owned without becoming modules in this registry.

Intrinsic module paths are reserved. A dependency package cannot override them, and the standalone standard library project must not define modules at those paths. Adding a new intrinsic path is therefore a compatibility decision because it claims a path that could otherwise belong to the package.

### Standalone standard library package

Ordinary standard library code lives in a separate Ard project with manifest name `ard`:

```toml
name = "ard"
ard = ">= 0.26.0"
```

It may contain modules such as:

```text
decode.ard
list.ard
map.ard
testing.ard
```

The repository, release cadence, tests, documentation, and compiler compatibility constraint are independent of the compiler repository and release.

A consuming project declares the package explicitly through the normal dependency system:

```toml
[dependencies]
ard = { git = "https://github.com/akonwi/ard-stdlib", tag = "v0.1.0" }
```

The exact repository URL and first release version are not fixed by this ADR.

The compiler does not silently add, fetch, or select this dependency. Project creation tooling may include it in new manifests by default, but builds use only the dependency graph declared by the project and recorded in its lockfile.

### Import resolution

For an import path under `ard/*`, resolution is:

1. If the complete import path is a registered compiler intrinsic, resolve it as compiler-owned.
2. Otherwise, resolve `ard` through the importing package's normal package context:
   - as the importing package itself when its manifest name is `ard`; or
   - as a direct dependency alias named `ard`.
3. Resolve the remainder of the path as a module within that package.
4. If no `ard` package is visible, report that the requested module is not intrinsic and that no `ard` dependency is declared.
5. If the package is visible but the module is absent, report an ordinary missing-module diagnostic against that package.

Examples:

| Import | Owner |
| --- | --- |
| `ard/async` | compiler intrinsic |
| `ard/unsafe` | compiler intrinsic |
| `ard/decode` | explicit `ard` package |
| `ard/list` | explicit `ard` package |
| `ard/unknown` | missing module in the explicit `ard` package |

The standalone `ard` package can import its own modules with normal absolute imports and can import compiler intrinsics through the same overlay:

```ard
use ard/unsafe
use ard/decode
```

### Package identity and lowering

Non-intrinsic `ard/*` modules retain ordinary dependency package identity. Their module, type, method, visibility, AIR, generated package, and FFI ownership must derive from the resolved package ID, not from the `ard/` spelling alone.

Intrinsic modules retain compiler-owned identity and lowering. Backends and tooling must distinguish the two owners explicitly rather than treating all `ard/*` modules as embedded standard library modules.

### Tooling behavior

The checker, formatter import handling, LSP, source navigation, completion, and diagnostics apply ordinary package behavior to non-intrinsic `ard/*` modules.

Only registered intrinsic modules may use compiler-provided signatures, immutable process-wide intrinsic caches, or synthetic source locations. Source navigation for `ard/decode`, for example, should lead to the dependency cache or local path package that owns it.

### Migration

Existing compiler-shipped modules are evaluated individually:

- Module-shaped compiler operations remain intrinsic.
- Ordinary Ard implementations move to the standalone package when its dependency and release workflow are available.
- A module path must not exist as both an intrinsic and a package module.
- Removing an ordinary module from the compiler should be paired with a diagnostic or migration note explaining how to declare the standalone `ard` dependency.

This ADR does not require all current modules to move in one change.

## Non-goals

This decision does not:

- define the complete initial module set of the standalone standard library;
- choose its repository URL, governance model, or first stable version;
- add a central package registry;
- make the standard library dependency implicit;
- allow packages to override compiler intrinsics;
- move builtin types or primitive operations out of the compiler;
- require specialized ecosystem libraries to join the `ard/*` namespace.

## Consequences

- Ard keeps concise module-shaped intrinsics without reserving the entire `ard/*` namespace for compiler implementation.
- Ordinary standard library modules can release independently and use the same dependency machinery as other Ard packages.
- Projects explicitly control and lock their standard library version.
- Programs that use only builtin language facilities and intrinsics do not need the standalone package.
- The compiler and LSP must replace broad `ard/*` special cases with exact intrinsic lookup followed by package-context resolution.
- Package ownership can no longer be inferred from an `ard/` prefix.
- Intrinsic path additions require care because those paths are permanently unavailable to the standalone package.
- Offline builds of projects using ordinary standard library modules require the locked dependency to be present in the normal cache, like any other dependency.

This decision supersedes only the import-resolution rule in ADR 0025 that says all `ard/*` imports resolve to the standard library. Its package graph, lockfile, cache, and ownership decisions otherwise remain in force. It also refines ADR 0013's statement that standard library modules live under `ard/*`: the namespace is shared by exact compiler intrinsics and the explicit package described here.

## Related

- `docs/adrs/0013-use-file-based-modules-and-absolute-imports.md`
- `docs/adrs/0025-use-lockfile-backed-dependency-cache.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0033-async-is-goroutines-and-channels.md`
- `docs/adrs/0034-reset-go-backend-and-ffi-boundary.md`
- `docs/adrs/0036-define-any-casting-policy.md`
- `docs/adrs/0037-define-unsafe-nil-interop-policy.md`
- `compiler/checker/std_lib.go`
- `compiler/checker/module_resolver.go`
- `compiler/std_lib/embed.go`
- [Gleam standard library](https://github.com/gleam-lang/stdlib)
