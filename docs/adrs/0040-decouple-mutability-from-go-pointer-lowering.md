# 0040: Decouple Mutability from Go Pointer Lowering

## Status

Accepted

Amended: native Ard mutable list references include the list descriptor and lower
pointer-shaped on the Go target. Descriptor-only mutable access is retained at
foreign Go ABI boundaries whose signatures require a slice value.

Superseded in part by ADR 0057. Binding mutability must not select a reference
representation, native list pointer shape supports sanctioned interior methods
rather than whole-list assignment, and all references follow pointer-copy/rebind
behavior even when descriptor or trait forwarding requires a specialized
handle.

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

The distinction differs for ordinary Ard functions. A native mutable list reference must reach the caller's entire list value, including the slice descriptor, so operations that grow or replace the list remain caller-visible:

```ard
fn fill(xs: mut [Int]) {
  xs.push(1)
}
```

On the Go target this lowers approximately as:

```go
func Fill(xs *[]int) {
    *xs = append(*xs, 1)
}
```

Mutability remains an Ard access capability rather than a Go pointer spelling. The pointer is a backend representation for native list-reference semantics; it is omitted at foreign boundaries where the Go ABI fixes the parameter as a slice value.

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
   - Used when all Ard mutations are naturally visible through a copied Go descriptor, or when a foreign ABI fixes the descriptor shape.
   - Examples: maps, channels, Go pointers, Go interfaces, and slices in foreign ABI positions.
   - Native `mut [T]` is excluded because list growth and replacement must update the owner; it lowers as `*[]T`.
   - Foreign slice positions remain `[]T` and provide element-level mutable access only.
   - `mut [K: V]` lowers as `map[K]V` because map entry mutation does not replace the map descriptor.
   - `mut foreign::PointerType` lowers as its Go pointer type, not as a pointer to that pointer.

3. **Foreign ABI mutable access**
   - Used when implementing or calling a Go API whose signature dictates the exact ABI shape.
   - Ard mutability may be allowed only if it can be represented without changing the Go signature.
   - For example, `mut [Byte]` can implement a Go `[]byte` parameter because the slice contents are mutable through the descriptor.

### Lists and slices

Ard lists lower to Go slices, but native mutable list references lower as pointers to the slice descriptor:

```ard
fn fill(xs: mut [Int])
```

```go
func Fill(xs *[]int)
```

This makes element mutation, `push`, `prepend`, and whole-list assignment visible to the caller. Explicit aliases, nested forwarding, closures, function values, and generic functions preserve the same reference to the descriptor.

Foreign Go ABI positions remain exact. A Go parameter or interface method requiring `[]T` continues to receive `[]T`, not `*[]T`. Such a parameter may mutate elements, but the checker rejects descriptor-rebinding operations such as `push` and `prepend` because Go cannot propagate the replacement descriptor to its caller.

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
- Native mutable list references satisfy Ard's caller-visible alias semantics even though their generated Go signature uses `*[]T`.
- The checker and backend distinguish native list references from foreign slice descriptors.
- Descriptor-rebinding list operations are rejected only where a foreign Go ABI cannot carry the updated descriptor.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0038-use-idiomatic-go-abi-for-result-and-maybe-returns.md`
- `docs/adrs/0039-support-explicit-go-interface-interop.md`
