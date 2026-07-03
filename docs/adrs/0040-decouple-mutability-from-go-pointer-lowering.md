# 0040: Decouple Mutability from Go Pointer Lowering

## Status

Proposed

## Context

Ard uses `mut` to express mutable access. In earlier Go lowering, mutable parameters were often treated as Go pointer parameters. That is correct for some values, such as structs or scalars whose storage must be mutated through an address, but it is not correct as a universal rule.

Some Go values are already reference-like descriptors. Slices, maps, channels, pointers, and interfaces can allow mutation of the referenced contents without passing a pointer to the descriptor itself. For example, Go's `io.Writer` interface requires:

```go
Write([]byte) (int, error)
```

An Ard implementation may want mutable access to the byte slice contents:

```ard
impl io::Writer for Sink {
  fn mut write(mut bytes: [Byte]) Int!Str {
    // may mutate bytes' elements
  }
}
```

That method must still lower to Go's exact interface ABI:

```go
func (s *Sink) Write(bytes []byte) (int, error)
```

It must not lower to:

```go
func (s *Sink) Write(bytes *[]byte) (int, error)
```

because `*[]byte` does not satisfy `io.Writer`.

The same distinction applies outside interface implementations. An ordinary Ard function:

```ard
fn fill(mut xs: [Int]) {
  // mutate elements
}
```

should lower to:

```go
func Fill(xs []int)
```

not `func Fill(xs *[]int)`, because the Go slice descriptor already carries mutable access to the backing array. Mutability is an Ard access capability; it is not synonymous with a Go pointer.

## Decision

Decouple Ard mutable access from Go pointer lowering.

`mut T` means the callee has mutable access according to Ard semantics. The Go backend chooses the representation needed for that access based on the lowered representation of `T` and the ABI context.

### Mutable access kinds

The Go backend should distinguish at least these mutable access shapes:

1. **Pointer-required mutable access**
   - Used when Go needs an address to mutate caller-visible storage.
   - Examples: structs, scalars, and other inline value types.
   - `mut Person` lowers as `*Person` when passed as a mutable parameter.

2. **Descriptor mutable access**
   - Used when the Go value is already a reference-like descriptor.
   - Examples: slices, maps, channels, Go pointers, and Go interfaces.
   - `mut [T]` lowers as `[]T`.
   - `mut [K: V]` lowers as `map[K]V`.
   - `mut foreign::PointerType` lowers as its Go pointer type, not as a pointer to that pointer.

3. **Foreign ABI mutable access**
   - Used when implementing or calling a Go API whose signature dictates the exact ABI shape.
   - Ard mutability may be allowed only if it can be represented without changing the Go signature.
   - For example, `mut [Byte]` can implement a Go `[]byte` parameter because the slice contents are mutable through the descriptor.

### Lists and slices

Ard lists lower to Go slices. A mutable list parameter lowers to a Go slice parameter:

```ard
fn fill(mut xs: [Int])
```

```go
func Fill(xs []int)
```

This gives mutable access to the elements. Operations that rebind the slice header, such as `push` when it appends and assigns the resulting slice descriptor, update only the callee's local descriptor in this representation. Caller-visible growth requires returning the new list, assigning through an owning field/local, or a future explicit operation that writes back the descriptor.

The important rule for this ADR is that `mut [T]` does not automatically mean `*[]T`, and callers should not rely on mutable list parameters to rebind the caller's slice header.

### Maps and channels

Maps and channels are descriptor/reference types in Go. Mutable map or channel access should lower to the descriptor itself rather than a pointer to the descriptor:

```ard
fn insert(mut values: [Str: Int])
```

```go
func Insert(values map[string]int)
```

Likewise, channel send/receive/close operations should not require `*chan T` merely because the Ard operation is effectful.

### Structs and scalar values

Inline values still require pointers when mutation must affect caller-owned storage:

```ard
fn rename(mut user: User)
```

```go
func Rename(user *User)
```

The same applies to scalar values when reassignment through a mutable parameter is intended.

### Go interface implementations

Foreign Go interface implementations must match the Go interface ABI exactly.

An Ard method implementing a Go interface may mark a parameter `mut` only when the Go ABI representation still matches the interface method. For example:

```ard
impl io::Writer for Sink {
  fn mut write(mut bytes: [Byte]) Int!Str { ... }
}
```

is valid because both `bytes: [Byte]` and `mut bytes: [Byte]` lower to `[]byte` in that ABI position.

But a mutable struct parameter cannot satisfy a Go struct-value parameter unless the Go interface itself expects a pointer-compatible shape.

### Checker and AIR representation

The checker and AIR should preserve mutability as an access capability separately from the eventual Go ABI representation. A parameter should carry enough information for the backend to know:

- the Ard semantic type;
- whether mutable access is requested;
- whether the lowered representation is pointer-required or descriptor-based;
- whether the context is a foreign ABI that must match exactly.

This avoids encoding Go pointer decisions into the core type system.

## Consequences

- Ard's `mut` remains a language-level access concept rather than a Go pointer spelling.
- Mutable slices/maps/channels can interoperate with idiomatic Go signatures.
- Go interface implementations can use `mut` for descriptor-backed values without breaking interface satisfaction.
- Ordinary Ard functions become more idiomatic in generated Go for descriptor-backed values.
- The backend must distinguish element/content mutation from descriptor rebinding for lists and similar values.
- Existing lowering that maps every mutable parameter to `*T` must be revised.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0038-use-idiomatic-go-abi-for-result-and-maybe-returns.md`
- `docs/adrs/0039-support-explicit-go-interface-interop.md`
