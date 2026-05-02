# go_next from AIR

This backlog tracks the Go target rewrite after AIR and `vm_next` parity. The
new Go target should consume AIR directly and should not preserve the current Go
target architecture unless a small piece is independently useful.

## Status

Pending

## Context

AIR now gives Ard a typed, target-neutral representation that is independent of
the bytecode VM and independent of `runtime.Object`. `vm_next` proves that the
language can execute against AIR with signature-driven FFI and target-shaped
values.

The next Go target should use that same AIR input so Ard is not merely syntactic
sugar over Go, while still generating idiomatic Go where the target supports it.

## Goals

- Build the Go target from scratch against AIR.
- Generate native Go values for Ard scalars and containers.
- Generate Go structs for Ard structs crossing backend and FFI boundaries.
- Generate tagged representations for Ard unions.
- Generate native Go closures for ordinary Ard function values.
- Lower Ard fibers to goroutines plus typed fiber/result handles.
- Coalesce self-hosted Ard stdlib modules into generated Go.
- Emit direct Go calls or generated adapters for low-level externs.
- Preserve Ard's testing model and single-file executable workflow where
  possible.

## Non-goals

- Do not preserve the current Go target's implementation structure by default.
- Do not make `runtime.Object` part of generated Go.
- Do not model every Ard value as `any`.
- Do not force `vm_next` callback handles into generated Go when native Go
  closures are available.

## Type Mapping

Initial Go mapping should follow the AIR design:

```text
Int          -> int
Float        -> float64
Bool         -> bool
Str          -> string
[T]          -> []T
[K:V]        -> map[K]V for Ard-keyable K
struct       -> generated Go struct
enum         -> generated named integer type
T?           -> ardrt.Maybe[T]
T!E          -> ardrt.Result[T,E] or `(T, error)` where the binding says so
union        -> generated tagged representation
Dynamic      -> opaque dynamic representation behind decode APIs
extern type  -> host resource handle
fn(...) ...  -> native Go function
Fiber[T]     -> generated/runtime fiber handle
```

## Stdlib Model

Pure Ard stdlib modules should compile from AIR into Go. Host capability modules
should become direct Go calls or generated adapters.

Examples:

- `ard/list`, `ard/map`, `ard/result`, `ard/maybe`, decode helpers, routing, and
  pure protocol logic should compile from Ard where possible.
- filesystem, environment, process, clock, crypto, sockets, JSON parse/stringify,
  and low-level HTTP hooks remain target externs.

## FFI Model

The Go target should reuse the generated FFI contract:

- generated structs are the default boundary type
- opaque externs represent host resources that do not fit Ard's type system
- callback boundaries become native Go closures in generated Go
- explicit adapters can support host-native structs later

## Checklist

- [ ] Define the `go_next` package layout.
- [ ] Lower AIR scalar expressions and direct calls to Go.
- [ ] Generate package/module structure from AIR modules.
- [ ] Generate structs, enums, `Maybe`, and `Result`.
- [ ] Generate tagged unions.
- [ ] Generate trait dispatch where dynamic trait objects are required.
- [ ] Lower fibers to goroutines and typed handles.
- [ ] Generate native Go closures and captures.
- [ ] Integrate generated/project FFI adapters.
- [ ] Compile self-hosted stdlib modules from AIR.
- [ ] Add Go target parity tests using the existing `vm_next` parity corpus and
  sample programs.
- [ ] Add runtime benchmark coverage for `go_next`.
