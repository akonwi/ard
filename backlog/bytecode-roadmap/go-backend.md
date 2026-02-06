# Go Backend Plan (Runtime-Light, Ard-Semantic)

This target emits Go code that preserves Ard semantics while depending on a small runtime support library.

## Requirements

- Dynamic is fully supported.
- Async semantics match Ard exactly.
- Collections compile to native Go slices/maps, with runtime helpers to preserve Ard rules.

## Runtime Support Scope

- Result and Maybe structs with helper functions (`ok`, `err`, `none`, `some`, `expect`, `or`, `is_ok`, `is_err`, `is_none`, `is_some`).
- Dynamic wrapper with conversions and JSON encode/decode helpers.
- Trait dispatch tables for calls on trait-typed receivers.
- Async runtime that matches current scheduling and error propagation behavior.
- Map key coercion helpers to preserve Ard key semantics.

## Codegen Strategy

- Emit Go that calls runtime helpers for Ard semantics rather than relying on idiomatic Go behavior.
- Generate trait dispatch stubs keyed by runtime kind/type tags.
- Compile pattern matching to `switch` on discriminants or runtime match helpers.
- Emit native slices/maps for List/Map, with helper calls for mutating operations and key coercion.

## Implementation Phases

1) **Define runtime-lite API**
   - Establish Go interfaces/types needed by generated code.

2) **Lowered IR contract**
   - Ensure all nodes required for codegen carry explicit metadata (return types, dispatch info, discriminants).

3) **Emitter**
   - Translate lowered nodes into Go code with runtime helper calls.

4) **Build integration**
   - Add CLI target to emit Go and compile with `go build`.

5) **Parity testing**
   - Compare interpreter VM results to Go backend for shared test cases.

## Notes

- This is still a runtime-backed backend; it avoids the interpreter but keeps semantics stable.
- Do not map async to goroutines directly unless behavior matches the current runtime.
