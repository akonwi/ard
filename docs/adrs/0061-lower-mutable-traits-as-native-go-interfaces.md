# 0061: Lower Mutable Traits as Native Go Interfaces

## Status

Accepted

## Context

ADR 0023 introduced forwarding tables for `mut Trait`, and ADR 0057 retained a
special comparable forwarding handle so a mutable trait reference could point
either at concrete storage or at a replaceable trait-typed slot. That design
made this program observe replacement of the trait slot:

```ard
mut current: View = Box{value: 1}
let reference = mut current

current = Other{value: 2}
reference.value() // 2 under ADR 0057
```

A Go interface does not behave that way. Copying an interface captures its
current dynamic type and value; it does not retain the address of the variable
that contained the interface:

```go
var current View = &Box{Value: 1}
reference := current

current = &Other{Value: 2}
reference.Value() // 1
```

Preserving trait-slot replacement therefore requires a representation that is
not a Go interface: a forwarding handle, adapter, function table, registry, or
pointer-to-interface protocol. The resulting Go is larger, less familiar, and
presents additional work to Go's inliner, escape analysis, interface
devirtualization, and generic implementation.

Two independent experiments replaced the forwarding-table implementation with
raw pointers under `any` and with a sealed wrapper interface. The raw-pointer
representation was compact and fast for known direct operations but could not
soundly cross arbitrary returned or writable Go generic positions. The wrapper
preserved those boundaries and avoided per-call allocations, but still added a
second dynamic-dispatch layer and a non-Go object model. Profiling did not show
a runtime benefit large enough to justify retaining semantics that differ from
Go's interface model.

Ard should add source-level safety and expressiveness where useful, but should
not reimplement Go's value, pointer, and interface semantics when the native Go
model is sufficient. Concrete Ard references already lower naturally to Go
pointers. Trait values should similarly lower naturally to Go interfaces.

## Decision

`Trait` and `mut Trait` lower to the **same canonical Go interface type**.
`mut` remains part of Ard's source type system and is erased when choosing the
Go type for a trait value. The backend does not emit a distinct `MutTrait`
interface, marker capability, forwarding wrapper, handle, vtable, registry, or
registration initializer.

Conceptually, mutable trait values use this interpretation:

```text
exists T implementing Trait. mut T
```

They do not mean:

```text
mut (exists T implementing Trait. T)
```

A `mut Trait` therefore captures the current dynamic object represented by the
interface. It is not a reference to the variable, field, global, or container
slot that held that interface.

### Concrete projection

Widening a concrete Ard reference preserves the concrete Go pointer directly:

```ard
let box = Box{value: 1}
let view: mut View = mut box
```

```go
var view View = &box
```

Copying the trait value copies the interface descriptor and preserves the same
`*Box`. Rebinding one trait binding does not retarget copies.

### Trait-valued operands

Applying `mut` to a trait-valued lvalue captures its current interface value:

```ard
mut current: View = Box{value: 1}
let captured = mut current
current = Other{value: 2}

captured.value() // 1
```

The generated operation is equivalent to ordinary Go interface assignment. It
does not take `&current` and does not observe later replacement of `current`.
The same rule applies to trait-valued fields, globals, and container elements.

### Mutable-to-ordinary trait conversion

`mut Trait` converts implicitly to `Trait`. This is a source-level loss of the
reference qualification and has no runtime representation change:

```ard
fn inspect(value: View) Int { value.value() }

let value: mut View = mut box
inspect(value)
let ordinary: View = value
```

Both source types lower to the same Go interface. Inferred bindings continue to
preserve the Ard source type; an ordinary destination or parameter performs the
qualification drop.

### Dereferencing

Postfix `.@` is not defined for `mut Trait`.

A Go interface has no generic pointee to materialize. Copying the interface
would preserve the same dynamic pointer rather than produce the shallow value
copy promised by ADRs 0057 and 0060, while cloning the dynamic pointee would
require reflection or a compiler-defined clone protocol. Neither behavior is a
faithful dereference.

Code that needs an independent dynamic object copy must request that operation
explicitly through a trait/library cloning API or narrow to a known concrete
reference before using `.@`.

This amends ADR 0060: `.@` remains the canonical one-layer dereference syntax,
but `mut Trait` is no longer an eligible operand because it has no Go pointer
layer in the target representation.

### Calls and method mutability

Ard continues to enforce binding, receiver, and reference rules in the source
checker. Go does not encode Ard's source-level mutability distinction, so Go
callers may perform any operation admitted by the generated interface. This is
intentional: Ard restrictions are additive source guarantees, not a replacement
runtime capability model.

Trait implementation mutability continues to determine whether an Ard concrete
implementation uses a Go value or pointer receiver. The canonical trait
interface itself is shared by ordinary and mutable trait values.

### Natural and fallback interfaces

Every representable Ard trait lowers to a native Go interface.

When all implementations can expose the natural Go method names, the interface
uses those names and Ard implementations participate naturally in matching Go
method sets.

When a legal Go selector cannot be emitted—for example, an Ard struct has both
a `name` field and a `name()` trait method—the compiler emits collision-proof
interface methods and receiver forwarding methods:

```go
type Named interface {
    ArdTraitMethod_Named_0() string
}

func (value User) ArdTraitMethod_Named_0() string {
    return value.Name
}
```

This remains ordinary Go interface dispatch. It does not use `any`, concrete
type switches, function tables, or adapters. Cross-package methods are exported
with collision-proof names where required.

If Go cannot legally attach even a collision-proof method to the implementing
type, the Go target reports a diagnostic rather than introducing a separate
runtime representation. Examples include implementations that would require
methods on nonlocal Go types or unsupported concrete generic instantiations.

### Equality and map keys

Trait equality and hashing use Go interface equality and hashing. Ard-produced
`mut Trait` values widened from concrete references contain pointers and are
therefore comparable. Trait values captured from existing interfaces follow the
comparability of their current dynamic Go value.

As in Go, comparing or hashing an interface whose dynamic value is not
comparable may panic. Ard does not add a second identity or hashing protocol for
trait values. Libraries that require a static comparability guarantee should
express that requirement explicitly rather than relying on a mutable-trait
handle invariant.

### `Any` and Go boundaries

Converting a trait value to `Any` uses ordinary Go interface conversion. A
`mut Trait` widened from `mut T` therefore exposes dynamic `*T` without a
projection hook. `unsafe::cast<mut Trait>` uses an ordinary checked Go interface
assertion and returns `Maybe<mut Trait>`; no forwarding metadata needs to be
reconstructed.

Ordinary and mutable Ard trait parameters have the same generated Go interface
ABI. External Go callers are governed by Go's interface rules and are not
prevented from passing implementations that Ard itself would not construct in a
mutable context. This is consistent with direct Go interoperability: Ard's
checker protects Ard source; it does not impose a second runtime type system on
Go callers.

### Generics, fields, returns, and containers

Because `Trait` and `mut Trait` share one named Go interface representation,
mutable trait values remain type-safe through:

- Go generic arguments and results;
- writable generic slices, maps, pointers, channels, and structs;
- Ard fields, returns, closures, and containers;
- Go method constraints compatible with the generated interface.

No `any` erasure or wrapper reconstruction is required.

## Consequences

- Generated Go uses one interface representation for each trait.
- Go assignment, copying, rebinding, equality, hashing, generics, and method
  dispatch retain their native semantics.
- The backend removes mutable-trait handles, vtables, registries, adapters,
  storage forwarding, projection hooks, and related reflection.
- Go has better opportunities to inline, devirtualize, and reason about escape
  behavior because dynamic values are the implementing values themselves.
- Generated APIs are easier for Go programmers to understand and adopt.
- Existing references to trait-typed storage no longer observe later slot
  replacement.
- `.@` on `mut Trait` becomes a compile-time error.
- `mut Trait -> Trait` becomes an implicit, representation-free conversion.
- Trait identity and comparability follow Go interfaces rather than a separate
  Ard storage-identity protocol.
- Some fallback implementations that cannot legally receive generated methods
  become Go-target errors instead of silently selecting a non-interface
  representation.

## Migration

Code depending on live trait-slot forwarding must retain and widen the concrete
reference before erasing its concrete type:

```ard
let box = Box{value: 1}
let box_reference = mut box
let view: mut View = box_reference
```

Code using `reference.@` only to pass a mutable trait to an ordinary trait
parameter should remove `.@`; the conversion is now implicit:

```ard
inspect(reference)
```

Code requiring an independent dynamic value copy must introduce an explicit
clone operation.

The compiler test matrix must replace forwarding-specific expectations with Go
interface expectations for copying, rebinding, equality, map keys, `Any`, Go
generics, fallback methods, cross-package private implementations, and method
constraints.

## Alternatives Considered

### Preserve ADR 0057 forwarding semantics

The forwarding-table, raw-pointer, and wrapper experiments all preserve live
trait-slot replacement. Each requires a runtime representation that is not a
plain Go interface. This conflicts with the decision to treat Go's interface
model as the target semantics for trait values.

### Emit a separate `MutTrait` capability interface

A marker or capability interface can statically require pointer-backed dynamic
values, but it gives `Trait` and `mut Trait` different Go ABIs and exposes a
capability model that Go itself does not use. Ard's source checker can enforce
its restrictions without introducing that target type.

### Use `*Trait`

A pointer to a Go interface observes slot replacement but does not implement the
interface, does not satisfy ordinary method constraints, exposes dynamic
`*Trait` through `any`, and gives concrete projections the wrong identity.
Pointers to interfaces are also non-idiomatic in Go.

### Keep `.@` as capability narrowing

Converting `mut Trait` to `Trait` while preserving the same interface value is
useful, but calling that operation dereferencing would conflict with `.@`'s
shallow-value materialization contract. The conversion is therefore ordinary
source assignability rather than a postfix operation.

## Related

- `docs/language-philosophy.md`
- `docs/go-backend-idiomatic-lowering.md`
- `docs/adrs/0023-represent-mutable-trait-references-with-forwarding-tables.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0056-preserve-ard-value-semantics-when-lowering-go-interfaces.md`
- `docs/adrs/0057-separate-binding-mutability-from-reference-values.md`
- `docs/adrs/0060-adopt-postfix-value-at-dereference-syntax.md`
