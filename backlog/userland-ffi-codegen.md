# Userland FFI Codegen

This backlog tracks code generation for userland Ard FFI projects. It is about
generating a stable Go signature/contract file from project `extern`
declarations; it is not about the already-supported ability to write project FFI
companion files by hand.

## Status

Pending design. The compiler already supports project Go FFI companions, but there is not yet a user-facing generator for project host signatures.

## Context

The internal stdlib project uses generated Go host signatures and Go-target
lowering metadata under `compiler/std_lib/ffi`. Userland projects should get a
similar signature-driven contract generated from Ard `extern` declarations.

Current project Go FFI support copies user-authored companion files from
`PROJECT_ROOT/ffi.go` and `PROJECT_ROOT/ffi/*.go`. These files must use
`package ffi`; the generated Go target imports them as a project FFI package.
Project extern types can bind to concrete Go host types, e.g.
`extern type Terminal = "*vaxis.Vaxis"`, so companion functions can use typed
host values instead of `any` casts. The `examples/tic-tac-toe` project is the
current real-world reference example.

The intended project shape is:

```text
project/
  ard.toml
  go.mod
  main.ard
  ffi.go              # optional user-authored companion, package ffi
  ffi/
    host.go          # optional user-authored companion, package ffi
    ard.gen.go       # future generated signatures, package ffi
```

`go.mod` lives at the Ard project root. User-authored Go code can live in root
`ffi.go` or under `ffi/*.go`. The proposed generator would add a generated
`ffi/ard.gen.go` file without overwriting user-authored companions.

## Goals

- Generate Go FFI code into `PROJECT_ROOT/ffi/ard.gen.go` for projects with a
  root `go.mod`.
- Keep `ard.gen.go` generated from Ard extern signatures.
- Generate target structs, callback handles, extern handle types, type aliases,
  enums, and host function declarations needed by project FFI.
- Keep user-authored Go code separate from generated code.
- Provide CLI commands that make the workflow discoverable and repeatable.

## Non-goals

- Do not generate dynamic runtime-object adapter layers.
- Do not require projects without FFI to have a `go.mod` or `ffi/` package.
- Do not support implicit structural matching against arbitrary host structs in
  the first version.

## CLI

### `ard ffi init`

Creates the initial Go FFI project shape:

- creates `go.mod` at the project root when missing
- creates `ffi/`
- creates a starter `ffi/host.go`
- generates `ffi/ard.gen.go`

The command should be conservative. If files already exist, it should not
overwrite user-authored files without an explicit future force flag.

### `ard ffi`

Regenerates `ffi/ard.gen.go` from the project's Ard source.

This should be the normal command after adding or changing `extern`
declarations.

### `ard ffi check`

Regenerates into memory or a temporary file and fails if `ffi/ard.gen.go` is out
of date.

This is intended for CI.

## Generation Rules

- Source of truth is Ard source, especially `extern fn`, `extern type`, public
  structs crossing FFI, aliases, enums, and callback signatures.
- Preferred output file is `ffi/ard.gen.go`, unless implementation discovers a
  better layout that coexists more cleanly with root `ffi.go` companions.
- Generated package name is `ffi`.
- `Maybe[T]` and `Result[T,E]` should use Ard's public Go runtime support types
  so generated project FFI matches stdlib FFI conventions.
- Function bindings use explicit target bindings where present and otherwise
  follow the same fallback rules as the compiler.
- Unsupported generic externs should be reported clearly instead of producing
  incomplete Go.

## Open Design Points

- Whether `ard ffi init` should run `go mod tidy` automatically or leave that as
  an explicit user step.
- Whether generated project FFI should eventually share a package with generated
  Go target output or remain a stable companion package.
- Whether to keep supporting both root `ffi.go` and `ffi/*.go` for generated-code
  workflows, or recommend `ffi/` for new projects while preserving root `ffi.go`
  for compatibility.

## Checklist

- [ ] Add project-root discovery for FFI generation.
- [ ] Generate `PROJECT_ROOT/ffi/ard.gen.go`.
- [ ] Add `ard ffi init`.
- [ ] Add `ard ffi`.
- [ ] Add `ard ffi check`.
- [ ] Add tests for project-level generated structs, callbacks, extern handles,
  `Maybe`, `Result`, lists, and maps.
- [ ] Document the userland FFI codegen workflow.
- [x] Keep at least one real project FFI example in-tree (`examples/tic-tac-toe`).
