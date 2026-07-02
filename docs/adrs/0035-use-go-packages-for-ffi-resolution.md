# 0035: Use go/packages for Go FFI Resolution

## Status

Accepted

## Context

Ard's direct Go interop currently resolves Go package metadata well enough for simple standard-library imports such as `use go:fmt`. The next FFI milestone is resolving packages the same way `go build` would for a real Ard project: third-party modules, local shim packages, `replace` directives, workspaces, and configured build tags.

The old `go/importer`-style resolution is not sufficient for module-aware interop. It does not reliably model the user's Go module graph, local project packages, replace directives, or build constraints.

## Decision

Use `golang.org/x/tools/go/packages` as the implementation behind Ard's Go FFI package resolver.

The checker must not discover project roots or depend broadly on `go/packages` details. Project loading owns the Ard project root and config, then passes a configured resolver into checking. The checker consumes a narrow resolver abstraction that returns Ard's Go-package metadata model.

### Project root

The Go resolver is configured with the Ard project root discovered by frontend/project loading.

`use go:` resolution must be relative to that project root, not the compiler process working directory and not the generated output directory.

### `go.mod` policy

Ard projects do not require a `go.mod` for Ard-only code or Go-standard-library-only direct imports.

A Go module context is required when direct Go interop uses third-party packages or project-local shim packages. Normal compiler commands must not mutate the source project's `go.mod` or `go.sum`. Missing packages should produce actionable diagnostics that tell the user to use ordinary Go tools such as `go mod init`, `go get`, or `go mod tidy`.

### Generated Go module policy

When generating Go output:

- if the source project has a `go.mod`, copy it to the generated output;
- if the source project has a `go.sum`, copy it too;
- rewrite relative local `replace` targets in the copied `go.mod` to absolute paths resolved from the source project root;
- if the source project has no `go.mod`, generate a synthetic minimal `go.mod` using the Ard project name as the module path.

Relative local `replace` paths are valid in Go, but they are relative to the `go.mod` file that contains them. Rewriting them to absolute paths keeps generated output buildable no matter where it is placed.

The compiler should use Go's module-file tooling, such as `golang.org/x/mod/modfile`, instead of string editing when rewriting `go.mod`.

### Project-local shim packages

Project-local Go code callable from Ard lives under:

```text
<project-root>/ffi/
```

The `ffi/` directory is the whitelist boundary for project-owned Go code exposed to Ard. Subpackages are allowed:

```text
ffi/
  print.go
  http/
    server.go
```

Valid local imports use the source module path and the `ffi` prefix:

```ard
use go:my_app/ffi
use go:my_app/ffi/http
```

Project-local Go packages outside `ffi/` are not part of Ard's local FFI surface. If Ard imports a package from the current module that resolves outside `<project-root>/ffi`, the compiler should reject it with a diagnostic telling the user to move Ard-callable Go code under `./ffi`.

When generating output, copy the whole `ffi/` directory recursively into the generated module if it exists. Third-party packages are not copied; they resolve through the copied `go.mod`. Local replacement modules outside the project root are also not copied; they remain referenced by rewritten absolute `replace` directives.

### Build tags

Go build tags are configured only in `ard.toml`:

```toml
[go]
build_tags = ["sqlite", "debug"]
```

There is no CLI override, no forced default tag set, and no compiler-owned registry of known tags. Go build tags are open-ended and package-defined, so Ard should not reject unknown valid tags.

Ard should only validate basic syntax for individual tag entries and reject malformed values such as empty strings, values with whitespace, or build-constraint expressions like `linux && cgo`. The configured tags are passed through unchanged and consistently to both `go/packages` resolution and generated Go build/run/test commands.

## Implementation Plan

1. **Add project config support**
   - Parse optional `[go].build_tags` from `ard.toml`.
   - Validate only basic individual-tag syntax.
   - Carry build tags in the project/load result used by checking and Go generation.

2. **Introduce a resolver abstraction**
   - Define a narrow Go package resolver interface consumed by the checker.
   - Keep checker-facing metadata in Ard's existing Go-package/foreign-type model.
   - Avoid leaking `packages.Package` through core checker code.

3. **Implement a `go/packages` resolver**
   - Configure it with `Dir = <project-root>`.
   - Pass configured build tags through `packages.Config.BuildFlags`.
   - Load enough metadata for exported functions, constants, named types, methods, struct fields, callbacks, aliases, and future map/interface/channel support.
   - Cache successful and failed resolutions by import path.

4. **Preserve stdlib-only behavior**
   - Ensure projects without `go.mod` can still import Go standard-library packages such as `fmt`.
   - Diagnose non-stdlib or local shim imports that require a missing module context.

5. **Enforce the local FFI boundary**
   - Determine the source module path from `go.mod` when present.
   - If a `use go:` import resolves to the current source module, require its package directory to be under `<project-root>/ffi`.
   - Produce actionable diagnostics for current-module imports outside `ffi/`.

6. **Generate module files**
   - Copy source `go.mod`/`go.sum` when present.
   - Rewrite relative local `replace` targets to absolute paths in the generated copy.
   - Generate a synthetic minimal `go.mod` when the source has none.

7. **Copy shim code**
   - If `<project-root>/ffi` exists, copy it recursively to generated output.
   - Preserve subpackages.
   - Exclude obvious generated-output directories if needed to avoid recursion.

8. **Use consistent build tags for generated commands**
   - Pass configured tags to generated Go build/run/test commands using `-tags`.
   - Ensure the checker and backend always use the same effective tag set.

9. **Add tests**
   - Resolve a standard-library package without `go.mod`.
   - Resolve a third-party or temp module package with `go.mod`.
   - Resolve a local `ffi` package under the project module path.
   - Reject a current-module package outside `ffi/`.
   - Verify generated output copies `go.mod`, `go.sum`, and `ffi/`.
   - Verify relative `replace` directives are rewritten to absolute paths.
   - Verify configured build tags reach both package resolution and generated Go commands.

## Consequences

- Ard's Go interop follows normal Go module resolution instead of a compiler-specific approximation.
- Users manage Go dependencies with normal Go tools; Ard compiler commands do not mutate source module files.
- Project-local Go interop has a clear whitelist boundary: `./ffi`.
- Generated Go output is more likely to build consistently in temporary or external output directories.
- Checker and backend behavior stay aligned because both use the same project root and build-tag configuration.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0034-reset-go-backend-and-ffi-boundary.md`
- `docs/language-philosophy.md`
