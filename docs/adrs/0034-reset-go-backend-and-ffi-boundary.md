# 0034: Reset Go Backend and FFI Boundary

## Status

Proposed

## Context

The parser, formatter, core checker semantics, and AIR are relatively stable. Most upcoming work on this branch is focused on Go code lowering and generation, with some checker work needed for target types and interop validation.

The current branch already improves Go interop significantly compared with `main`, but legacy concepts still blur the boundary between Ard's core language and external target concepts. Examples include checker-level `ExternType` modeling, legacy `extern fn` / `extern type` machinery, generated FFI binding contracts, and Go package imports that are not cleanly isolated from Ard module/type semantics.

The goal of this branch is intentionally breaking: strip Ard down to its core language model, then reintroduce the FFI story deliberately. The standard library may shrink during this reset. That is acceptable because rebuilding stdlib modules on top of the new interop model will provide clearer validation than preserving old wrappers for compatibility.

## Decision

Reset the Go backend and FFI boundary around a cleaner separation between Ard core semantics and target interop.

Ard's core language should be defined without depending on Go-specific concepts. Go remains the default and reference target, and Ard should interoperate with Go well, but Go types and Go package metadata should enter the compiler through an explicit interop boundary rather than being mixed into the core language model.

### Stable core

Preserve these areas unless a specific backend/FFI decision exposes a real language issue:

- parser and syntax for core Ard;
- formatter behavior for core Ard;
- AIR as the frontend/backend boundary;
- core checker semantics for Ard-owned types and values;
- primitives such as `Bool`, `Str`, `Int`, and `Float64`;
- `Maybe` for explicit nullability;
- `Result` for errors as values;
- explicit mutability and immutable access;
- structs, enums, functions, methods, traits, generics, modules, and visibility.

### Primary work area

Focus this branch on the Go backend:

- package/module layout;
- identifier lowering;
- value and type lowering;
- function, method, trait, and generic lowering;
- runtime representation choices;
- generated project structure;
- direct Go interop lowering;
- host Go shim package integration.

Generated Go must preserve Ard semantics first. It should be idiomatic where practical, but it is compiler output, not the user-facing source of truth.

### Checker boundary

The checker may use target-aware information to validate interop, but that support must be isolated.

Target-specific concepts must not become core Ard concepts. In particular:

- legacy `ExternType`-style modeling should be removed or replaced;
- Go package imports should not be treated as ordinary Ard modules;
- Go named types should not leak into the core type system as if they were Ard-defined types;
- target metadata should be queried through a narrow interop abstraction;
- core Ard type checking should remain understandable without knowing Go.

The checker can validate that an Ard expression is compatible with a target boundary, but the target owns how that boundary lowers.

### FFI direction

The intended FFI model is:

1. direct Go imports for APIs that are naturally representable in Ard;
2. ordinary Go shim packages for APIs that need semantic adaptation;
3. explicit Ard-facing wrappers when Go concepts need to become Ard concepts.

Examples of semantic adaptation include:

- converting Go `(T, error)` into Ard `Result`;
- converting Go comma-ok or absence-like APIs into `Maybe`;
- hiding nil-prone or panic-prone APIs behind safer Ard functions;
- choosing defaults for Go APIs with many configuration parameters;
- adapting callback, interface, opaque `Any`, reflection, or lifecycle-heavy APIs.

Direct Go interop should make simple cases simple, but it should not force arbitrary Go concepts into Ard's core semantics.

### Remove or reset legacy machinery

Remove, rewrite, or quarantine these during the reset:

- legacy `extern fn` and `extern type` machinery;
- checker `ExternType` concepts that model host types as core Ard types;
- old FFI companion binding tables or generated host contracts;
- standard-library wrappers that only exist because older FFI required ceremony;
- Go import/package handling that is tangled with Ard module resolution;
- runtime/helper types that exist only to paper over incomplete Go lowering.

Removal is acceptable even if it temporarily reduces working stdlib coverage.

### Standard library reset policy

The standard library may shrink significantly during this work.

Restored modules should prove one of the following:

- they are essential to using or testing the language;
- they exercise the new direct Go interop model;
- they demonstrate appropriate Go shim adaptation;
- they provide stable Ard semantics that should not be left to raw Go APIs.

Prefer deleting legacy wrappers first, then reintroducing APIs deliberately as direct Go calls, pure Ard code, or Go shim-backed Ard wrappers.

### Rebuild order

Use this order as a guide:

1. minimize core runtime and stdlib to what the compiler/tests require;
2. remove legacy extern modeling from parser/checker/AIR/backend where possible;
3. define the new target interop abstraction for Go package metadata;
4. rebuild direct `use go:` imports through that abstraction;
5. rebuild Go lowering around the accepted backend contract;
6. add ordinary host Go shim package support;
7. restore stdlib modules incrementally, using each one as a test case for either direct interop or semantic adaptation.

## Consequences

- This branch may intentionally break compatibility with `main` branch FFI syntax and behavior.
- Existing standard-library coverage may temporarily shrink.
- The compiler should become easier to reason about because core Ard checking is separated from target interop.
- Go lowering can be rebuilt around preserving Ard semantics rather than preserving legacy backend artifacts.
- Direct Go interop and shim Go packages become the two main ways to use Go libraries from Ard.
- Future non-Go backends remain plausible because Go-specific concepts are kept behind an explicit boundary.

## Related

- `docs/language-philosophy.md`
- `docs/adrs/0030-use-direct-go-struct-values-and-fields.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `compiler/checker`
- `compiler/go`
