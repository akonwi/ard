# Go Interface Semantics for Ard Interop

This note records the Go type-system behavior Ard must preserve or deliberately
adapt when crossing a Go interface boundary. It is supporting technical
reference for ADR 0056, not itself an architecture decision.

## Why interfaces are exceptional

An ordinary Go value parameter has one concrete destination type. A Go
interface is an existential container: its static interface type admits a set
of concrete types, while each interface value carries one observable dynamic
concrete type and value.

The Go specification says that interface variables have a distinct dynamic
type: the non-interface type of the value assigned at run time. The dynamic
type may change over time, but each stored value must be assignable to the
static interface type.

This means an interface parameter does not by itself answer whether its dynamic
value owns data or refers to existing storage. That depends on the concrete
dynamic value placed in the interface.

Primary references:

- [Go specification: Variables](https://go.dev/ref/spec#Variables)
- [Go specification: Interface types](https://go.dev/ref/spec#Interface_types)
- [The Laws of Reflection](https://go.dev/blog/laws-of-reflection)

## Static interface type and dynamic concrete value

An interface value can be modeled semantically as:

```text
(dynamic concrete type, dynamic concrete value)
```

For example:

```go
var value io.Reader = file
```

may contain:

```text
(*os.File, filePointer)
```

The static type determines which methods may be called through `value`. The
dynamic type remains the full concrete type and is observable through type
assertions, type switches, reflection, formatting such as `%T`, and interface
equality.

Assigning one interface to another does not nest the source interface as the
dynamic value:

```go
var reader io.Reader = file
var value any = reader
```

Both carry the same concrete dynamic type and value. As the official reflection
article states, interfaces do not hold interface values; they hold concrete
values.

## Interface implementation and method sets

A basic runtime interface defines a type set through its required methods:

```go
type Writer interface {
    Write([]byte) (int, error)
}
```

A concrete type implements that interface when its method set includes every
required method with the required signature.

For a defined non-interface type `T`:

- the method set of `T` contains methods declared with receiver `T`;
- the method set of `*T` contains methods declared with receiver `T` or `*T`.

Therefore:

```go
type T struct{}

func (*T) Write([]byte) (int, error) { return 0, nil }

var _ io.Writer = T{}   // invalid
var _ io.Writer = &T{}  // valid
```

Primary references:

- [Go specification: Method sets](https://go.dev/ref/spec#Method_sets)
- [Go specification: Implementing an interface](https://go.dev/ref/spec#Implementing_an_interface)
- [Go FAQ: Why do T and *T have different method sets?](https://go.dev/doc/faq#different_method_sets)

Ard additionally requires an explicit `impl` for an Ard-owned type to conform
to a foreign Go interface. That nominal requirement is Ard policy layered over
Go's generated method-set requirement.

## Addressable method calls do not change interface assignability

Go permits pointer-receiver method syntax on an addressable `T` value:

```go
var value T
value.PointerMethod() // shorthand for (&value).PointerMethod()
```

This call shorthand does not add pointer-receiver methods to `T`'s method set.
The following remains invalid:

```go
func consume(value InterfaceWithPointerMethod) {}

consume(value)
```

The Go FAQ explains that if an interface contains `T`, there is no safe way for
a method call to obtain `*T`. Allowing it would mutate an inaccessible value
inside the interface, and taking a temporary address could make mutations
silently disappear from the caller.

This distinction is central to Ard interop: a compiler-generated address during
interface conversion is an adapter, not Go's ordinary interface assignment
from `T`.

## Copying and aliasing

The Go specification classifies primitive values, arrays, and structs as
self-contained values. Assigning one of them to an interface copies its value.
The copy is shallow according to the concrete type:

```go
type V struct {
    Number int
    Shared *int
}

original := V{Number: 1, Shared: pointer}
var value any = original
```

The interface has its own copy of `Number`, but its copied `Shared` pointer
still refers to the same pointee.

Pointers, functions, slices, maps, and channels contain references to
underlying data. Copying an interface containing one of those values preserves
the references carried by the dynamic value. The specification therefore says
an interface value may be self-contained or contain references depending on
its dynamic type.

Passing an interface parameter copies the interface value, but it does not
necessarily copy the data reachable through its dynamic value.

Primary reference:

- [Go specification: Representation of values](https://go.dev/ref/spec#Representation_of_values)

## Pointer boxing is observable

Suppose generated Ard `W` has pointer-receiver methods and only `*W` satisfies a
Go interface. Lowering an Ard value through temporary storage:

```go
valueCopy := value
consume(&valueCopy)
```

places dynamic type `*W` in the interface, not `W`. That is observable:

```go
reflect.TypeOf(interfaceValue) // *W

_, isValue := any(interfaceValue).(W)   // false
_, isPointer := interfaceValue.(*W)     // true
```

Pointer boxing is therefore not merely a hidden storage optimization. It
selects the dynamic Go type and affects:

- reflection and `%T`;
- type assertions and type switches;
- satisfaction of additional interfaces;
- interface equality;
- nil behavior;
- aliasing and mutation;
- allocation and escape behavior.

Ard's interface lowering contract must document these effects.

## Owned pointer boxes and borrowed pointers

Go does not encode ownership in the interface type. The same dynamic type `*W`
can point to either compiler-owned storage or existing caller storage.

Owned box:

```go
valueCopy := value
var interfaceValue SomeInterface = &valueCopy
```

Mutations through the interface persist in `valueCopy` but do not affect the
original Ard value.

Existing reference:

```go
var interfaceValue SomeInterface = existingPointer
```

Mutations through the interface affect the existing referent.

Go code can observe pointer identity and effects, but it does not need a type
level ownership distinction. Ard can make ownership clear from the source:

```text
W source     -> interface-owned box
mut W source -> existing reference
```

A reassignable Ard binding containing `W` remains a `W` source and therefore
uses owned boxing. An explicit `mut <place>` creates a reference source and
therefore preserves identity.

## Why value receivers cannot implement persistent Ard mutation

Generating a mutating Ard method as a Go value-receiver method would let `W`
satisfy the interface directly:

```go
func (value W) Mutate() {
    value.Field = updated
}
```

But the receiver is a copy. Mutations to inline state do not update caller
storage or the value stored in the interface, and they do not persist across
separate method calls. This does not implement Ard mutable-method semantics.
Pointer receivers or another explicit reference-bearing representation are
required when mutation must persist.

## Nil interfaces and typed nils

A nil interface has neither dynamic type nor dynamic value:

```go
var value io.Writer
value == nil // true
```

An interface containing a typed nil pointer still has a dynamic type:

```go
var pointer *Buffer
var value io.Writer = pointer
value == nil // false
```

Semantically these are different:

```text
(nil, no value)
(*Buffer, nil)
```

Primary reference:

- [Go FAQ: Why is my nil error value not equal to nil?](https://go.dev/doc/faq#nil_error)

Ard must not model every interface containing a nil pointer as a nil interface.

## Equality

Two interface values are equal when their dynamic types are identical and
their dynamic values are equal. Comparing interface values whose identical
dynamic type is not comparable causes a run-time panic:

```go
var left any = []int{1}
var right any = []int{1}

_ = left == right // panic: comparing uncomparable type []int
```

An interface containing `*W` uses pointer equality. Two independently boxed
copies of the same `W` normally compare unequal because they have distinct
addresses.

Primary reference:

- [Go specification: Comparison operators](https://go.dev/ref/spec#Comparison_operators)

## Type assertions and switches

Type assertions and type switches inspect the dynamic concrete type:

```go
switch value := interfaceValue.(type) {
case W:
    // matches only dynamic W
case *W:
    // matches only dynamic *W
}
```

An Ard adapter that boxes `W` as `*W` must expect Go assertions to `*W`, not
`W`, to succeed.

Primary references:

- [Go specification: Type assertions](https://go.dev/ref/spec#Type_assertions)
- [Go specification: Type switches](https://go.dev/ref/spec#Type_switches)

## Runtime layout is not the language contract

Go implementations commonly represent an interface using type metadata and a
data word. The language specification does not guarantee that physical layout.
Ard should depend on the semantic contract instead:

- static and dynamic types;
- method sets and assignability;
- copying and aliasing behavior;
- type assertions and reflection;
- nil and equality behavior.

## Generic constraint interfaces are separate

Since Go 1.18, interfaces may contain type terms and unions for generic
constraints:

```go
interface {
    ~int | ~int64
}
```

Such general interfaces are not necessarily valid runtime value types. They
belong to Go generic constraint checking, not the runtime basic-interface
conversion described here.

Primary references:

- [Go specification: General interfaces](https://go.dev/ref/spec#General_interfaces)
- [Go specification: Type constraints](https://go.dev/ref/spec#Type_constraints)

## Ard interop invariants

ADR 0056 adopts these invariants for Ard-owned explicit implementations:

1. A foreign interface is an existential conversion boundary, not an ordinary
   concrete `T` destination.
2. Both `W` and `mut W` are eligible when nominal `W` has the explicit foreign
   interface implementation.
3. A `W` source contributes an owned copy.
4. A `mut W` source preserves its existing reference identity.
5. Reassignability of a binding slot has no role in ownership or eligibility.
6. When only generated `*W` has the required Go method set, both forms produce
   dynamic Go type `*W`.
7. Go does not need to distinguish owned and borrowed pointers; Ard source and
   conversion metadata do.
8. A caller with `mut W` can request owned-copy behavior by first producing an
   explicit `W` snapshot.
9. Existing interface-to-interface conversions preserve the concrete dynamic
   value rather than nesting interfaces.
10. Exact Go ABI implementation checks remain separate from interface argument
    conversion.
11. Ard `Any` and named empty Go interfaces follow the same ownership rule even
    though they require no explicit implementation.
12. A Go generic parameter constrained by `any` is a concrete destination after
    inference, not an interface-value destination; ordinary reference-to-value
    snapshot semantics apply unless the type argument resolves to Ard `Any`.

## Related Ard documentation

- [ADR 0039: Support Explicit Go Interface Interop](./adrs/0039-support-explicit-go-interface-interop.md)
- [ADR 0040: Decouple Mutability from Go Pointer Lowering](./adrs/0040-decouple-mutability-from-go-pointer-lowering.md)
- [ADR 0045: Support Explicit Mutable Reference Expressions](./adrs/0045-support-explicit-mutable-reference-expressions.md)
- [ADR 0056: Preserve Ard Value Semantics When Lowering Go Interfaces](./adrs/0056-preserve-ard-value-semantics-when-lowering-go-interfaces.md)
