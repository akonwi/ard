# 0082: Select Top-Level Declarations by Platform

## Status

Accepted

## Context

Ard cannot include or exclude Ard source per target platform (Go's `GOOS` and
`GOARCH`). Each check loads one Go type universe for one platform (ADR 0044),
and every Ard declaration must type-check against it. A module that names
`syscall.SIGWINCH` therefore fails to compile for `GOOS=windows`, even inside a
run-time `if`. Code that touches a platform-only Go symbol has to live in a Go
bridge package with Go build constraints (#569).

The motivating case is Cooper's `ffi/signalwatch`. Its unix file
(`darwin || freebsd || linux || netbsd || openbsd`) watches `SIGHUP`, `SIGQUIT`,
`SIGTERM`, and `SIGWINCH`. Its Windows file and its fallback for every other
platform are identical and watch only `os.Interrupt`. Only two small functions
differ by platform.

Today the platform is never named inside the compiler. `go/packages` and the
generated `go build` both inherit `GOOS`/`GOARCH` from the environment, so
`GOOS=windows ard check` already checks against Windows Go packages.
`[go] build_tags` (ADR 0035) selects Go files only. Build values (ADR 0069) are
typed run-time constants whose branches must type-check on every platform.
ADR 0064 introduced `#name(...)` attributes for struct fields and deferred build
constraints because they need source selection before checking.

Prior art:

- Go selects whole files by filename suffix or `//go:build` expression. `unix`
  is a constraint name but not a suffix. gopls checks one `GOOS` at a time, so
  files for other platforms rot unless CI checks them.
- Rust `#[cfg]` and Zig comptime branches parse inactive code but do not check
  it. Rot is a known problem in both, handled with per-target checks in CI.
- Gleam moved from `@target` toward one shared, always-checked signature with
  a separate implementation per target.

## Decision

### `#platform` attribute

A top-level declaration or `use` line may carry one `#platform(...)`
attribute, written on its own line before the declaration:

```ard
use go:os
use go:syscall

fn watched() [os::Signal] { [os::Interrupt] }

#platform(unix)
fn watched() [os::Signal] {
  [os::Interrupt, syscall::SIGHUP, syscall::SIGQUIT, syscall::SIGTERM, syscall::SIGWINCH]
}
```

The attribute takes one or more positional platform names and matches when
any of them matches the current platform. Arguments are bare names, not
strings. `#platform` is not accepted on struct fields, methods inside an
`impl` block, enum variants, or anywhere other than the top level. It is not
repeatable, and an empty `#platform()` is an error.

The term is `platform`, not `target`: `target = "go"` in `ard.toml` already
names the compiler backend, and `--target` was the backend flag until ADR
0029. The CLI flags below use the same term.

### Platform names

- An operating system: any `GOOS` the compiler knows, such as `linux`,
  `darwin`, `windows`, or `wasip1`.
- An operating system and architecture: `<os>_<arch>`, such as `linux_amd64`
  or `darwin_arm64`.
- `unix`: Go's unix set (`aix`, `android`, `darwin`, `dragonfly`, `freebsd`,
  `hurd`, `illumos`, `ios`, `linux`, `netbsd`, `openbsd`, `solaris`).

Names match the same platforms as Go's build constraints, including Go's
implied operating systems: `linux` also matches `android`, `darwin` also
matches `ios`, and `solaris` also matches `illumos`. This keeps Ard selection
consistent with the Go files compiled for the same platform.

The compiler owns the list of known platforms (the `go tool dist list` pairs
for the supported Go release). An unknown name is an error, so a typo such as
`#platform(linx)` cannot silently exclude a declaration everywhere.

### Selection and fallback

Declarations that bind the same top-level name form a group. A `use` line
binds its alias. An `impl` block is identified by its header (`impl Type` or
`impl Trait for Type`).

- At most one declaration in a group has no `#platform`. It is the
  **fallback** and is active on every platform where no annotated sibling
  matches.
- Annotated siblings must not overlap: two of them may not match the same
  known platform. This is checked against the whole platform list, not just
  the current platform.
- Without a fallback, the name does not exist on platforms none of the
  siblings match. Using it there is an ordinary unresolved-name error on that
  platform.
- Two declarations of the same name without `#platform` remain a duplicate
  declaration error.

Selection happens once per module, right after parsing and before any other
analysis. Inactive declarations and `use` lines are removed from the program
that the Go import scan, the checker, and AIR see. Inactive Go imports are
therefore never loaded, so a package that does not build on the current
platform (such as `golang.org/x/sys/unix` on Windows) causes no error. The
formatter and the sibling contract below see the full source.

Importers see only the active declaration for a name. AIR and the Go backend
are unchanged.

### Sibling contract

Siblings must present the same API, so code that uses them type-checks the
same way on every platform. The contract is syntactic and checked on every
run for every loaded module, whatever the current platform:

- Functions, static functions, and `test fn`: the same kind and an identical
  header (visibility, name, parameters with their names, mutability, and
  types, and return type). Bodies may differ.
- `let` and `mut`: the same binding (`let` or `mut`) and an identical
  explicit type annotation, which is required.
- `impl` blocks: the same header and the same set of method headers.
- `type` aliases: the same name and type parameters; the aliased type may
  differ.
- `use` lines: the same alias; the import paths may differ.
- Structs, enums, unions, and traits may be platform-restricted but may not
  have siblings. Per-platform variants of a nominal type are deferred.

Headers are compared in their canonical formatted form, so layout and
comments do not matter, but spelling does: `os::Signal` and an alias of it
are different.

### The current platform

Each invocation fixes one platform before loading:

- By default it is the platform `go env GOOS GOARCH` reports for the project,
  which honours the environment and `go env -w`.
- `ard check` and `ard build` accept `--platform <os>/<arch>`, which must be a
  known platform. `ard run` and `ard test` execute on the host and do not
  accept it.

An explicit platform is passed as `GOOS` and `GOARCH` to `go/packages`
resolution and to the generated `go build`. Without one, both use the same go
tool defaults that selection used. Either way, Ard selection, Go resolution,
and the built binary agree. The platform's architecture also sets the width of
`Int`, `Uint`, and `Uintptr` for checking, as it does for Go's `int`, so
`--platform linux/386` rejects an `Int` literal that does not fit in 32 bits.

### Checking every platform

`ard check` checks the current platform by default.
`ard check --all-platforms` checks one representative platform for each distinct
selection in the program:

1. Parse the union of modules reachable under any selection, and evaluate each
   known platform's selection over it.
2. Group platforms that select the same declarations.
3. Check one platform per group, preferring the host, then `linux/amd64`,
   `darwin/arm64`, and `windows/amd64`, then list order.

Each checked platform has its own Go type universe. Diagnostics name the
platform they were found on. For Cooper this checks two platforms: one unix
platform and one other.

### Editor support

The LSP analyzes each file under the current platform. When a file contains
declarations that are inactive there, it also analyzes the file under a
representative platform that activates them, chosen as above. Each analysis
uses a Go resolver session for its platform. Diagnostics from a secondary
analysis are published only for the ranges of declarations that are inactive
on the current platform. Hover, definition, references, and completion use the
analysis whose active declarations contain the cursor. Hover on a fallback
lists its platform-specific siblings.

## Alternatives Considered

- **Platform variants of a module chosen by filename** (`signals.ard` and
  `signals.unix.ard`). This keeps selection per file and needs no syntax, but
  it splits small differences into separate modules, and every tool that lists
  files (test discovery, the LSP workspace walk) must learn variant rules.
- **Compile-time branches inside function bodies** (`comptime if
  platform::unix { ... }`). This is the most flexible option, but the untaken
  branch is not checked, it looks like an `if` on build values that *is*
  checked, and editors cannot analyze it.
- **An explicit `#platform(default)` fallback.** Every alternative would then
  be marked, but it adds syntax for the most common case. The identical-header
  contract already catches most accidental overrides of a general declaration.
- **Boolean expressions** (`any`, `all`, `not`). The fallback rule covers the
  common `not(unix)` case, and nested attribute values would extend ADR 0064's
  metadata grammar. They can be added later if a real case needs them.

## Consequences

- Platform-specific code that uses Go symbols can be written in Ard. Cooper's
  `signalwatch` no longer needs Go build constraints.
- A sibling's API cannot drift unnoticed: the contract is checked on every
  platform. Bodies are fully checked by `--all-platforms` and in the editor.
- `--all-platforms` loads one Go type universe per representative platform. The
  first check for a new platform cross-compiles its Go export data, which the
  Go build cache then keeps.
- Packages that need cgo usually fail to resolve for a foreign platform,
  because cross-compiling disables cgo by default.
- The compiler carries a platform list that must be updated when Go adds a
  port. Code can select a newer port only after the list is updated.
- The LSP keeps one Go resolver session per platform it analyzes.

## Related

- `docs/adrs/0035-use-go-packages-for-ffi-resolution.md`
- `docs/adrs/0043-rebuild-lsp-on-snapshot-analysis.md`
- `docs/adrs/0044-use-a-shared-go-type-universe.md`
- `docs/adrs/0064-add-json-struct-field-attributes.md`
- `docs/adrs/0069-expose-manifest-build-values-through-ard-build.md`
- `docs/adrs/0075-allow-only-declarations-at-module-top-level.md`
- GitHub issue #569
