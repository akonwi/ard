# AIR Architecture

This document describes Ard Intermediate Representation (AIR), the typed
backend-facing representation used between checking and code generation/runtime
execution.

## Purpose

AIR exists to make Ard a multi-target language without making any one runtime or
host language the semantic center of the compiler.

In particular, AIR gives Ard:

- a typed, target-neutral representation after checking
- a shared input boundary for multiple backends
- a way to keep backend logic independent from checker-internal type shaping
- a semantic layer that is not tied to VM object layouts or generated Go code

AIR is the architectural boundary that lets both `vm_next` and the Go backend
consume the same checked Ard program.

## Pipeline position

```text
Ard source
→ parse
→ checker
→ AIR
→ backend-specific lowering/execution
```

Today that means:

- `vm_next` lowers AIR into its own bytecode/runtime execution model
- `compiler/go` lowers AIR into Go AST and formatted Go source

## Design intent

AIR is meant to be:

- typed
- target-neutral
- runtime-independent
- lowered enough for backend consumption
- structured enough to preserve source-oriented diagnostics and control flow

AIR is not:

- parse AST
- checker AST
- bytecode
- Go-specific IR
- a VM object model
- a universal boxed runtime ABI

## Architectural boundaries

The checker remains the semantic source of truth for Ard programs.

AIR's job is to carry checked program semantics forward in a backend-friendly
shape so backends do not have to rediscover meaning through repeated
`checker.Type` assertions or runtime-specific metadata lookups.

That means AIR should expose concrete program structure such as:

- modules and functions
- concrete type identities
- struct field layouts
- enum/union metadata
- captures and closure shape
- extern signatures
- explicit statement/expression structure
- entry and script execution roots

## Program model

At a high level, AIR models a checked Ard program as:

- modules
- functions
- concrete type metadata
- traits and impls
- extern declarations
- entrypoint/script roots

Two execution roots matter:

- `Entry`: the normal program entrypoint
- `Script`: the synthetic root used for top-level executable statements

This keeps entry selection in lowering rather than requiring backends to guess
based on source names like `main`.

## Type model

AIR owns compact type metadata for backends.

Backends should work with explicit `TypeID`s and resolved type information
rather than re-deriving language facts from checker structures.

Important implications:

- executable AIR uses concrete specialized types
- struct layout is index-based
- field access should lower through resolved layout metadata
- result/maybe/union/function/fiber shapes are explicit in the AIR type table

This allows backends to lower operations like field access, matching, and call
validation using shared semantic metadata instead of target-specific heuristics.

## Statement and expression model

AIR uses structured statements and expressions rather than SSA.

Expressions carry:

- kind
- type
- source location

Representative AIR operations include:

- constants and local loads
- `let` and assignment
- block expressions
- `if`, `while`, and loop forms
- direct calls, extern calls, and closure calls
- struct/list/map construction and mutation
- enum/result/maybe/union construction and matching
- `try`
- trait object operations
- fiber/async intrinsics

The goal is to preserve Ard semantics in a shape that is still straightforward
for backends to lower into their own target models.

## Validation role

AIR also defines a backend-facing validation boundary.

The validator is responsible for rejecting malformed lowered programs before
backend execution/codegen proceeds. Examples include:

- unresolved IDs
- invalid field indexes
- call signature mismatches
- illegal assignments
- invalid extern signatures
- backend-facing async/capture invariant violations

This keeps backend assumptions explicit and helps move correctness checks out of
individual backend implementations.

## Relationship to vm_next

`vm_next` was the first major backend built around AIR.

That work established several important architectural outcomes:

- Ard execution no longer needs to be centered on `runtime.Object`
- the VM execution value model is a runtime implementation detail, not the
  language ABI
- FFI can be shaped from Ard signatures rather than old object-wrapper patterns
- collections, closures, traits, async, and matching can all lower from the
  same shared IR

## Relationship to the Go backend

The rewritten Go backend also consumes AIR directly.

This confirms the intended multi-target design:

- AIR is not VM-specific
- AIR is not Go-specific
- new backends can share the same semantic input boundary

For Go-target-specific architecture and current design decisions, see
[`go-emission-architecture.md`](./go-emission-architecture.md).

## Key constraints

The following constraints should continue to guide AIR evolution:

- AIR should stay target-neutral
- AIR should not become a serialized form of any one backend's runtime values
- AIR should expose resolved semantics that backends actually need
- AIR should prefer explicit metadata over backend re-derivation
- AIR should support both runtime backends and codegen backends cleanly

## Historical note

AIR emerged from the work that introduced `vm_next` and enabled the later Go
backend rewrite. That migration work is complete; this document captures the
enduring architectural intent rather than the original backlog plan.
