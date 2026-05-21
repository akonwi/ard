# 0002: Use AIR as the Backend Boundary

## Status

Accepted

## Context

Ard supports multiple targets, including the Go/native backend and JavaScript targets. The checker is the semantic source of truth, but target implementations should not need to repeatedly inspect checker-internal type structures or encode target-specific guesses about language semantics.

Without a shared backend-facing representation, each target risks growing its own interpretation of checked programs. That makes the compiler harder to evolve and can accidentally make one runtime or host language the semantic center of Ard.

Ard Intermediate Representation (AIR) already exists as a typed representation between checking and target lowering. The Go backend consumes AIR directly today. JavaScript currently lowers from the checked program model separately and can migrate toward AIR over time.

## Decision

Use AIR as the durable backend-facing boundary after checking and before target-specific lowering.

The compiler pipeline should be understood as:

```text
Ard source
→ parse
→ checker
→ AIR
→ target-specific lowering/execution
```

AIR should remain:

- typed
- target-neutral
- runtime-independent
- explicit about resolved program semantics targets need
- structured enough to preserve source-oriented diagnostics and control flow

AIR should expose target-facing program structure such as modules, functions, concrete type identities, struct layouts, enum/union metadata, captures, closure shape, extern signatures, statements/expressions, and entry/script roots.

AIR models two execution roots:

- `Entry`: the normal program entrypoint
- `Script`: the synthetic root used for top-level executable statements

Keeping these roots explicit means entry selection is handled during lowering instead of requiring targets to infer behavior from source names such as `main`.

AIR owns compact type metadata for targets. Targets should work with explicit `TypeID`s and resolved type information rather than re-deriving language facts from checker structures. Executable AIR should use concrete specialized types; struct layout should be index-based; and result, maybe, union, function, and fiber shapes should be explicit in the AIR type table.

AIR uses structured statements and expressions rather than SSA. Expressions carry kind, type, and source location. Representative operations include constants, local loads, `let`, assignment, block expressions, control-flow forms, calls, extern calls, closure calls, construction and mutation of compound values, enum/result/maybe/union construction and matching, `try`, trait object operations, and fiber/async intrinsics.

AIR should not become:

- a parse AST
- a checker AST
- a Go-specific IR
- a universal boxed runtime ABI
- a serialized form of any one target's runtime values

Targets should lower from AIR rather than rediscovering semantic facts through repeated checker type assertions or target-specific metadata lookups. AIR validation should reject malformed lowered programs before target code generation proceeds.

The Go/native target is the reference implementation of this boundary. It lowers `air.Program` directly into Go AST, formats generated source with `go/format`, and invokes the Go toolchain for run/build/test workflows:

```text
Ard source
→ parse
→ checker
→ AIR
→ Go lowering
→ go/ast
→ go/format
→ generated Go source
→ go build / binary execution
```

The Go backend should:

- lower directly from AIR to Go AST without a separate Go-specific semantic IR
- assemble one generated Go package per Ard project
- emit one generated Go file per Ard module
- render production generated code through `go/format`, not string templates
- materialize generated workspaces under `ard-out/go/{run,build}` for inspection and debugging
- use statement-oriented Go lowering when Ard expressions require temporaries, setup statements, or explicit control flow
- keep generated helper functions small and backend-owned

The Go target should prefer plain Go representations where practical, while using shared runtime types where they encode Ard semantics more clearly. In particular, `Maybe<T>` lowers to `runtime.Maybe[T]`, `Result<T, E>` lowers to `runtime.Result[T, E]`, `Dynamic` and opaque extern values lower to `any`, and extern-backed standard library/host calls should lower as direct static Go calls wherever possible.

The Go backend should not reintroduce the deleted Go backend architecture, use a universal `runtime.Object` representation, model all values as `any`, or use registry-driven host adapter models.

## Consequences

- Backend implementations get a shared semantic input boundary.
- The checker remains responsible for language semantics, while targets remain responsible for target-specific lowering and execution.
- Go/native backend decisions do not define Ard semantics for other targets.
- Generated Go remains inspectable and deterministic while preserving Ard semantics through statement-oriented lowering where needed.
- Generated artifacts are debug outputs and should not be treated as stable checked-in source.
- New targets can be added without re-implementing checker internals.
- AIR must evolve carefully to stay target-neutral while still exposing enough resolved metadata for practical code generation.
- JavaScript lowering may need incremental migration work before it fully uses AIR.

## Related

- `docs/adrs/0015-use-esm-javascript-targets-with-explicit-runtime-semantics.md`
- `docs/adrs/0001-record-architecture-decisions.md`
