# AIR Architecture

This document describes Ard Intermediate Representation (AIR), the typed backend-facing representation used between checking and target lowering.

## Purpose

AIR exists to make Ard a multi-target language without making any one runtime or host language the semantic center of the compiler.

AIR gives Ard:

- a typed, target-neutral representation after checking
- a shared input boundary for multiple backends
- a way to keep backend logic independent from checker-internal type shaping
- a semantic layer that is not tied to generated Go code or any future runtime layout

AIR is the architectural boundary that lets Ard targets consume the same checked program.

## Pipeline position

```text
Ard source
→ parse
→ checker
→ AIR
→ target-specific lowering/execution
```

Today, `compiler/go` lowers AIR into Go AST and formatted Go source. JavaScript targets lower from the checked program model separately and can migrate toward AIR over time.

## Design intent

AIR is meant to be:

- typed
- target-neutral
- runtime-independent
- lowered enough for target consumption
- structured enough to preserve source-oriented diagnostics and control flow

AIR is not:

- parse AST
- checker AST
- Go-specific IR
- a universal boxed runtime ABI

## Architectural boundaries

The checker remains the semantic source of truth for Ard programs.

AIR's job is to carry checked program semantics forward in a target-friendly shape so targets do not have to rediscover meaning through repeated `checker.Type` assertions or target-specific metadata lookups.

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

This keeps entry selection in lowering rather than requiring targets to guess based on source names like `main`.

## Type model

AIR owns compact type metadata for targets.

Targets should work with explicit `TypeID`s and resolved type information rather than re-deriving language facts from checker structures.

Important implications:

- executable AIR uses concrete specialized types
- struct layout is index-based
- field access should lower through resolved layout metadata
- result/maybe/union/function/fiber shapes are explicit in the AIR type table

This allows targets to lower operations like field access, matching, and call validation using shared semantic metadata instead of target-specific heuristics.

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

The goal is to preserve Ard semantics in a shape that is still straightforward for targets to lower into their own models.

## Validation role

AIR also defines a target-facing validation boundary.

The validator is responsible for rejecting malformed lowered programs before target codegen proceeds. Examples include:

- unresolved IDs
- invalid field indexes
- call signature mismatches
- illegal assignments
- invalid extern signatures
- target-facing async/capture invariant violations

This keeps target assumptions explicit and helps move correctness checks out of individual target implementations.

## Relationship to the Go backend

The rewritten Go backend consumes AIR directly.

This confirms the intended multi-target design:

- AIR is not Go-specific
- new targets can share the same semantic input boundary

For Go-target-specific architecture and current design decisions, see [`go-emission-architecture.md`](./go-emission-architecture.md).

## Key constraints

The following constraints should continue to guide AIR evolution:

- AIR should stay target-neutral
- AIR should not become a serialized form of any one target's runtime values
- AIR should expose resolved semantics that targets actually need
- AIR should prefer explicit metadata over target re-derivation
- AIR should support both runtime targets and codegen targets cleanly
