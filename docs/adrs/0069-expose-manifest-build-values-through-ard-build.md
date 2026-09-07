# 0069: Expose Manifest Build Values Through `ard/build`

## Status

Accepted

## Context

Applications commonly need release metadata such as a version, release channel,
or build number. Ard's Go target can inherit `go build` linker flags, but generated
Go package paths and symbols are compiler implementation details. Applications
currently need a Go FFI shim solely to provide a stable linker symbol.

Build metadata must be typed before target lowering, deterministic for checking,
running, tests, and editor tooling, and portable to future targets. It must not
implicitly depend on environment variables, Git state, timestamps, or host state.

ADR 0049 reserves exact compiler-owned module paths within `ard/*`. Providing
build values therefore also requires reserving a module path.

## Decision

Reserve `ard/build` as a compiler-owned synthetic module. The root manifest may
declare build values:

```toml
[build.values]
version = { type = "Str", default = "dev", release = true }
channel = { type = "Str", default = "local" }
build_number = { type = "Int", default = 0 }
experimental = { type = "Bool", default = false }
```

Every declaration has a valid Ard identifier, an explicit type, and a default of
that type. Initially supported types are exactly `Str`, `Int`, and `Bool`.
`release` is optional and defaults to `false`.

Root-package modules access values as immutable module symbols:

```ard
use ard/build

let version = build::version
```

The compiler constructs `ard/build` for each resolver invocation. Its symbols
are ordinary checked immutable globals initialized with target-neutral literals.
They lower through the existing checker → AIR → target pipeline. The module is
not source-backed and is never stored in the process-wide embedded-module cache.

Dependencies cannot consume the root application's build values. A dependency
may use its own build declarations when compiled independently as a root project.
Importing `ard/build` without root declarations is an error.

`ard build` accepts explicit overrides:

```sh
ard build main.ard --define version=v1.2.3 --define build_number=42
```

The text after the first `=` is the value. `Str` preserves it verbatim and may be
empty or contain additional `=` characters. `Bool` accepts exactly `true` or
`false`. `Int` accepts a signed decimal value in the current Ard `Int` range.
Unknown, malformed, or duplicate overrides are errors.

`ard build --release` requires an explicit override for every declaration with
`release = true`. An override equal to the default still counts as explicit.
The policy applies to unused values. `--release` does not alter optimization,
backend settings, or defaults beyond enforcing this requirement.

Commands without build options—`check`, `run`, `test`, and the LSP—use manifest
defaults. The compiler does not inspect ambient metadata automatically. Tooling
that wants Git or CI metadata must pass it explicitly with `--define`.

Build values are embedded public artifact data and must not contain secrets.

## Manifest parsing

`ard.toml` is parsed as one complete TOML document into a shared typed manifest
model. Project identity, compiler constraint, Go configuration, dependencies,
and build values all derive from that parser rather than independent regular
expressions. Known sections validate their supported value shapes; unknown
non-build fields remain available for forward-compatible manifest evolution.
Malformed TOML and malformed build declarations fail project loading even when
no build value is referenced.

Dependency-editing commands may continue to apply targeted textual edits so they
preserve comments and formatting, but semantic reads use the shared parser.

## Consequences

- Release tooling has a stable, typed, target-neutral metadata interface.
- Generated Go names and linker flags remain implementation details.
- `ard/build` becomes a permanently reserved compiler-owned path.
- Effective values are invocation-local and cannot leak between projects.
- Changing manifest defaults or declarations invalidates LSP analysis through
  the existing manifest signature.
- The Go backend may represent immutable Ard globals as Go package variables;
  immutability is enforced by Ard rather than exported as a Go API guarantee.

## Related

- `docs/adrs/0021-represent-module-level-lets-as-air-globals.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0049-overlay-ard-intrinsics-on-an-explicit-stdlib-package.md`
- GitHub issue #468
