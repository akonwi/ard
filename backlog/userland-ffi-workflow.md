# Userland FFI Workflow

This backlog tracks the post-`vm_next` work for making Ard FFI generation
available to ordinary Ard projects, not only the compiler's internal stdlib
project.

## Status

Pending

## Context

The internal stdlib project now uses generated Go adapter shapes under
`compiler/std_lib/ffi`. Userland projects should get the same signature-driven
contract, with adapters generated from Ard `extern` declarations instead of
discovered from handwritten Go.

The intended project shape is:

```text
project/
  ard.toml
  go.mod
  main.ard
  ffi/
    host.go
    ard.gen.go
```

`go.mod` lives at the Ard project root. `ffi/ard.gen.go` is generated, while
`ffi/host.go` is where project authors provide host implementations.

## Goals

- Generate Go FFI code into `PROJECT_ROOT/ffi/ard.gen.go` for projects with a
  root `go.mod`.
- Keep `ard.gen.go` generated from Ard extern signatures.
- Generate target structs, callback handles, extern handle types, type aliases,
  enums, and host function declarations needed by project FFI.
- Keep user-authored Go code in separate files under `PROJECT_ROOT/ffi`.
- Provide CLI commands that make the workflow discoverable and repeatable.

## Non-goals

- Do not discover bindings from old bytecode VM FFI packages.
- Do not generate `runtime.Object` based adapters.
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
- Output file is always `ffi/ard.gen.go`.
- Generated package name is `ffi`.
- `Maybe[T]` and `Result[T,E]` remain generated support types until there is a
  shared public runtime package for generated Go bindings.
- Function bindings use explicit target bindings where present and otherwise
  follow the same fallback rules as the compiler.
- Unsupported generic externs should be reported clearly instead of producing
  incomplete Go.

## Open Design Points

- Whether `ard ffi init` should run `go mod tidy` automatically or leave that as
  an explicit user step.
- How project FFI packages should be referenced by `ard run --target vm_next`
  and other execution/build flows now that the Go target rewrite exists.
- Whether generated project FFI should eventually share a package with generated
  Go target output or remain a stable companion package.

## Checklist

- [ ] Add project-root discovery for FFI generation.
- [ ] Generate `PROJECT_ROOT/ffi/ard.gen.go`.
- [ ] Add `ard ffi init`.
- [ ] Add `ard ffi`.
- [ ] Add `ard ffi check`.
- [ ] Add tests for project-level generated structs, callbacks, extern handles,
  `Maybe`, `Result`, lists, and maps.
- [ ] Document the userland FFI workflow.
