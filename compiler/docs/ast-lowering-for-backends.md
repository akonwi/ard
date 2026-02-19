# AST Lowering: Decouple Backends from checker.Type

## Motivation

Enable multiple backends (bytecode VM, Go transpiler, WASM, etc.) by decoupling backend execution from checker type introspection.

This makes it easier to:
1. Generate different targets from the same checked program
2. Keep backends simple and predictable
3. Evolve the type system without breaking runtime backends
4. Add new compilation targets without re-implementing checker internals

## Architecture Goal

The checker acts as an IR producer by enriching nodes with pre-computed metadata.

```
Source Code
    -> parse AST
    -> checker AST (enriched metadata)
    -> backend (bytecode / Go / WASM)
    -> runtime artifact
```

Backends should consume enriched fields directly, not perform repeated `checker.Type` assertions.

## Concrete Changes Implemented

### 1) MapLiteral metadata

- Added pre-computed key/value metadata on map literal nodes.
- Backend logic reads pre-computed fields directly instead of asserting map type each time.

### 2) StructInstance metadata

- Added pre-computed field-type map on struct instance nodes.
- Backend logic iterates the pre-computed field map instead of pulling fields from asserted struct type.

### 3) ModuleStructInstance metadata

- Added pre-computed field-type map for module struct instances.
- Backend logic uses node metadata directly, avoiding module symbol type assertions.

### 4) OptionMatch metadata

- Added pre-computed inner type for option/maybe matches.
- Backend match evaluation no longer derives maybe inner type dynamically.

## Why this matters

Even when type assertions are not a hotspot, this improves:

1. Clarity: backend code becomes direct execution over lowered metadata
2. Decoupling: backend changes are less coupled to checker internals
3. Extensibility: new backends can reuse the same lowered model
4. Maintainability: fewer assertion-heavy code paths and fewer hidden invariants

## Scope

- Keep parse/check phases as-is conceptually
- Enrich checker nodes with backend-ready fields
- Keep behavior identical; this is architectural simplification

## Completed Phases

### Phase 0: Package rename

- Renamed `ast` package to `parse`.
- Updated imports and callers across the codebase.

### Phase 1: Core node enrichment

- Implemented metadata enrichment for `MapLiteral`, `StructInstance`, and `ModuleStructInstance`.

### Phase 2: Additional node enrichment

- Implemented metadata enrichment for `OptionMatch`.

### Phase 3: Backend simplification

- Bytecode backend consumes lowered checker metadata.
- Legacy interpreter runtime has been removed.
- `ard run` executes through the bytecode backend.

## Current State

- Bytecode backend is the runtime path for `ard run` and `ard build` outputs.
- Checker metadata lowering is in place for high-impact node types.
- Future backends should build on the same lowered checker AST contract.

## Follow-ups

- Continue replacing remaining backend-side type introspection with pre-computed metadata where practical.
- Keep this document focused on architecture and status; implementation details belong in package-level docs and tests.
