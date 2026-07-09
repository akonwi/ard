# 0047: Add Fixed-Size Array Types

## Status

Accepted

## Context

Ard currently has growable list types written as `[T]`. Lists are the right default for most Ard code, but they do not model fixed-size storage. This matters for two related reasons:

1. Some APIs naturally require a length in the type, such as hashes, keys, protocol markers, RGB triples, and other small fixed-width data.
2. Direct Go FFI needs a principled mapping for Go fixed arrays (`[N]T`). Mapping Go arrays to Ard lists would erase the fixed-length property and imply hidden allocation/copying or runtime length checks in places where the type system can carry the length.

Earlier discussion considered adapting Go fixed arrays to Ard lists. That approach is too imprecise: Ard lists are resizable, while Go arrays have length as part of their type. A direct list mapping also leaves unclear semantics for passing Ard lists to Go array parameters, reading mutable Go array fields, and preserving copy/value behavior.

Several languages were considered for syntax:

- Rust: `[T; N]`
- Zig: `[N]T`
- Odin: `[N]T`

Ard already uses `[T]` for lists, so a Rust-like spelling extends the existing container syntax while keeping the element type in the same position.

## Decision

Add a distinct fixed-size array type to Ard using Rust-like syntax:

```ard
[Byte; 32]
[Int; 3]
[Str; 2]
```

Fixed-size arrays are a core Ard type, not a Go-only interop artifact. The Go backend is the initial lowering target and Direct Go FFI is the motivating use case, but `[T; N]` participates in ordinary Ard type identity, local variables, fields, function signatures, and generic type arguments where other sized value types are allowed.

This type is not a growable list. The length is part of the type, so `[Byte; 32]` and `[Byte; 16]` are different types, and `[Byte; 32]` is distinct from `[Byte]`.

The array length grammar is intentionally narrow in the initial version: `N` must be a non-negative integer literal representable as an Ard `Int`. Named constants, arithmetic expressions, inferred lengths, and generic/const parameters are not accepted in array type syntax in this ADR. `[T; 0]` is valid and maps to Go `[0]T`.

Go fixed arrays map to this type at the Direct Go FFI boundary:

```go
func Sum32(data [32]byte) uint32
func Digest() [32]byte
```

are exposed to Ard as shapes equivalent to:

```ard
fn Sum32(data: [Byte; 32]) Uint32
fn Digest() [Byte; 32]
```

### Value semantics

Fixed-size arrays are value types. Binding, assignment, return, and parameter passing copy the array value, matching Go array value semantics. The copy is element-wise and has the same shallow/deep behavior as copying the element type elsewhere in Ard.

Whole-array assignment is valid when the target is an addressable mutable place and the source has the exact same fixed-array type.

### Literal construction

Uncontextualized list literals remain lists:

```ard
let xs = [1, 2, 3] // [Int]
```

A list-like literal may be contextually checked as a fixed-size array when the expected type is known:

```ard
let rgb: [Byte; 3] = [255, 0, 0]
use_rgb([255, 0, 0]) // if use_rgb expects [Byte; 3]
```

The checker validates the element count and element types. A literal with the wrong length is a compile-time error.

Contextual fixed-array literals use the same per-element contextual typing and range checking as list/argument literals. For example, `[300]` is invalid for `[Byte; 1]` if `300` is not a valid `Byte`. `[]` is valid as a fixed-array literal only when the expected type is `[T; 0]`.

This ADR does not introduce a length-inferred fixed-array type syntax such as `[Byte; _]` or `[Byte; ?]`. That can be considered later if needed.

### Operations

The initial operation set should be intentionally small:

- indexing, with the same index expression rules as lists;
- element assignment through an index only when the array expression is an addressable mutable place;
- `.size()` returning the fixed length as an `Int`;
- iteration in `for` loops.

Index bounds follow list indexing semantics. Compile-time diagnostics for statically out-of-bounds literal indexes are allowed but not required by this ADR.

Iterating a fixed-size array yields elements using the same binding semantics as list iteration. The initial design does not introduce mutable element iteration; mutation should use indexing on an addressable mutable array.

Until a general equality rule for list-like containers exists, fixed-size arrays do not support `==`, ordering comparisons, or structural equality, except where an existing generic equality rule already admits the element/container type.

Fixed-size arrays do not support resizing operations such as `push`, `pop`, or `clear`.

### Type identity, generics, and formatting

Generic unification treats `[T; N]` structurally: element types must unify and lengths must be exactly equal. Diagnostics and formatter output use canonical `[Element; N]` spelling.

Type aliases, when available for a position, may name fixed-size array types the same way they may name other concrete types.

### Conversion and coercion

There is no implicit coercion between `[T]` and `[T; N]`.

If a program needs a growable list from a fixed-size array, or a fixed-size array from a list, that should be an explicit operation. The list-to-array direction needs a length-mismatch design and is not part of this ADR.

There is also no recursive scalar coercion through fixed arrays. For example, `[Int; 3]` does not implicitly become `[Byte; 3]`; user code must transform elements explicitly.

### Direct Go FFI mapping

A Go array `[N]E` is representable only when `E` is representable by the existing Direct Go type mapping. The Ard element type is the mapped Ard type for `E`.

The first implementation should prefer value/copy semantics at FFI boundaries. Reading a Go array value or field into Ard produces a fixed-size array value. Passing an Ard fixed-size array to a Go `[N]T` parameter passes the corresponding Go array value.

Named Go array types remain named Direct Go types rather than being erased to plain structural arrays. For example:

```go
type Digest [32]byte
```

should be exposed as a direct-Go named type with `[Byte; 32]` as its underlying representation. Assignment and call compatibility should follow the existing named Go type rules, with the fixed array type used as the representation for literals, field access, and lowering.

Mutable writeback through Go array fields is deferred. In particular, mutating an Ard value obtained from a Go array field should not silently mutate the original Go object unless a future design explicitly supports that aliasing model.

### Not included

This ADR does not add slicing, concatenation, spread/rest syntax, destructuring, const generics, named length constants in type syntax, length-inferred fixed-array type syntax, mutable element iteration, or implicit list/array conversions.

## Consequences

Benefits:

- Ard gains a first-class way to represent fixed-width data.
- Direct Go FFI can expose Go `[N]T` precisely instead of erasing it to `[T]`.
- Length mismatch for literals can be caught at compile time.
- The syntax is consistent with Ard's existing `[T]` list type by adding `; N`.

Costs and follow-up work:

- Parser, formatter, checker, AIR, Go lowering, LSP, and tree-sitter support are required.
- The type system must carry array length as part of type identity.
- Runtime and lowering behavior for indexing, iteration, and method dispatch must be defined separately from lists where needed.
- Explicit list ↔ fixed-array conversion APIs remain future work.
- Syntax for inferred-length fixed arrays remains undecided and is not included here.
- The highest-risk implementation point is contextual literal typing: `[1, 2, 3]` must remain a list without an expected fixed-array type, while the same syntax must type-check as `[T; N]` under an expected type. The parser should not introduce a separate fixed-array literal syntax.

## Alternatives Considered

### Map Go fixed arrays to Ard lists

Rejected. This erases the fixed-length property, hides copies, and makes parameter passing require runtime length checks or surprising conversions.

### Zig/Odin-like `[N]T`

Rejected for Ard. It is compact and familiar in systems languages, but Ard already uses `[T]` for lists. `[T; N]` is a smaller extension of the existing Ard type grammar and keeps the element type in the same position.

### Infer fixed arrays from all bracket literals

Rejected. Ard list literals should continue to produce growable lists unless context requires a fixed-size array. This preserves existing code and avoids surprising default fixed-size values.
