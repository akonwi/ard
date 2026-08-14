# 0058: Add Checked List and String Slicing

## Status

Accepted

## Context

Ard lists currently support checked point access and whole-list operations, but
cannot select a contiguous subrange without routing through a Go FFI shim. Ard
strings likewise lack checked substring extraction. Go's slice expression
provides the required representation:

```go
view := values[start:end]
```

Returning that representation as an ordinary Ard list would hide important
behavior. A Go subslice shares element storage with its source, exposes capacity,
and may or may not remain attached to the source after either descriptor grows.
`append` can also overwrite elements beyond the visible range when spare
capacity exists. Those behaviors are too surprising for an ordinary Ard
`List<T>` value.

Copying every selected range into a new list would avoid aliasing, but would
remove the primary utility of a slice: a cheap fixed-length view over existing
storage. Ard instead needs a type that makes view semantics explicit while
excluding operations that change its size or capacity.

ADR 0057 already separates ordinary values from mutable-reference values. A
second mutable view type would duplicate that capability model. Writable slice
access should use ordinary `mut Slice<T>` references rather than a separate
`MutSlice<T>` type or a receiver-dependent return type.

## Decision

Introduce a builtin generic `Slice<T>` type representing a fixed-length shared
view over contiguous list element storage.

### Creating a slice

Add checked `slice` methods to lists and slices with nullable, optional bounds:

```ard
fn List<$T>.slice(start: Int?, end: Int?) Slice<$T>?
fn Slice<$T>.slice(start: Int?, end: Int?) Slice<$T>?
```

For these compiler-defined methods, omitted nullable arguments are supplied as
`none`. An omitted `start` defaults to zero, and an omitted `end` defaults to the
receiver's visible size. Named arguments provide Go-like open-bound forms
without introducing subscript syntax:

```ard
values.slice()                       // values[:]
values.slice(start: 1)               // values[1:]
values.slice(end: 3)                 // values[:3]
values.slice(start: 1, end: 3)       // values[1:3]
```

Callers may also pass nullable bounds computed at runtime. A `none` value has the
same meaning as omitting that bound.

The result type does not depend on whether the receiver is a value or a mutable
reference. Calling `slice` through `mut List<T>` or `mut Slice<T>` still returns
an ordinary `Slice<T>?`.

```ard
let values = mut [10, 20, 30, 40]
let view = values.slice(start: 1, end: 3).expect("valid bounds")
// view: Slice<Int>
```

After applying defaults, bounds are zero-based and half-open: `start` is
inclusive and `end` is exclusive. A range is valid when:

```text
0 <= start <= end <= size
```

Invalid resolved bounds return `none`. Equal bounds return an empty slice,
including at the end of the source. Bounds are checked against visible length,
never hidden capacity. `slice()` always returns a full-range view wrapped in
`some`.

The receiver and every provided argument are evaluated exactly once, in source
order, before defaults and bounds validation are applied.

### Slicing strings

Strings provide the same checked, optional-bound call shape:

```ard
fn Str.slice(start: Int?, end: Int?) Str?
```

String slice indexes are UTF-8 byte offsets, matching Go string slicing and the
existing byte length returned by `Str.size()`:

```ard
"hello".slice(start: 1)         // some("ello")
"hello".slice(end: 4)           // some("hell")
"hello".slice(start: 1, end: 4) // some("ell")
```

After applying the same zero and visible-size defaults, a range is valid when
`0 <= start <= end <= source.size()`. Invalid bounds return `none`, equal bounds
return `some("")`, and `slice()` returns the complete string wrapped in `some`.
The receiver and provided bounds retain the same single-evaluation guarantees as
list slicing.

Byte indexing intentionally follows Go rather than `Str.at()`, which addresses
Unicode scalar values. A byte range may split a multi-byte UTF-8 encoding and
produce a string containing invalid UTF-8. Code that must preserve rune
boundaries should use `runes()`, slice that list, and convert the materialized
result back with `Str::from`.

On the Go target, a valid string slice lowers directly to:

```go
substring := source[start:end]
```

The result is an immutable `Str`, not `Slice<Byte>`. Its observable semantics do
not depend on whether a backend copies or shares the source's string storage.
The Go target may retain the larger source allocation for the lifetime of a
small substring.

### View and aliasing semantics

Creating a slice does not copy elements. The slice and its source initially
share the selected element storage. Writes to an existing shared element are
visible through every list or slice view still attached to that storage.

```ard
let values = mut [10, 20, 30, 40]
let view = values.slice(1, 3).expect("valid bounds")
let writable = mut view

writable.set(0, 99)
// values is [10, 99, 30, 40]
```

A slice has fixed visible length and no source-level capacity. It cannot grow,
shrink, append, prepend, clear, or replace its descriptor. A nested slice is
another fixed-length view over the selected portion of the same storage.

Structural mutation of the source list may allocate new backing storage. An
existing slice remains valid and retains its old backing storage, but may no
longer observe later changes made through the source's new descriptor. This is
intentional Go-like view behavior and must be documented prominently.

A small slice may retain a much larger backing allocation. Code that needs to
release that storage must materialize an independent list with `to_list()`.

### Mutability follows ordinary reference semantics

`Slice<T>` is the only slice type. Ard does not add `MutSlice<T>`.

An ordinary slice value supports read-only operations. Writable element access
requires an explicit mutable reference created with the existing `mut`
expression:

```ard
let view = values.slice(1, 3).expect("valid bounds")
let writable: mut Slice<Int> = mut view

writable.set(0, 99)
```

The `let` binding holding `writable` is not reassignable, but its stored value is
a mutable reference and therefore permits sanctioned interior mutation under
ADR 0057.

Slicing never propagates mutable-reference shape implicitly:

```ard
let outer: mut Slice<Int> = mut view
let inner = outer.slice(0, 1).expect("valid bounds")
// inner: Slice<Int>, not mut Slice<Int>
```

Code that needs a writable nested view explicitly borrows `inner`.

### Initial method surface

`Slice<T>` provides fixed-view observation and conversion:

```text
size() Int
is_empty() Bool
at(index: Int) T?
slice(start: Int?, end: Int?) Slice<T>?
to_list() List<T>
```

Slices are iterable using ordinary `for` iteration.

The following fixed-length mutations are available only through
`mut Slice<T>`:

```text
set(index: Int, value: T) Bool
swap(left: Int, right: Int) Void
```

No slice method may alter length or capacity. In particular, `push`, `prepend`,
and whole-descriptor replacement are unavailable even through
`mut Slice<T>`.

### Explicit list materialization

`to_list()` allocates independent top-level list storage and shallow-copies the
visible elements:

```ard
let independent: [Int] = view.to_list()
```

Reference-valued elements retain their pointer identity, consistent with ADR
0057's shallow-copy rules. On the Go target, materialization uses one exact-size
allocation and Go's predeclared `copy` builtin:

```go
result := make([]T, len(source))
copy(result, source)
```

`.@` on `mut Slice<T>` produces an ordinary `Slice<T>` view value; it does
not convert the view into a list.

`Slice<T>` is not implicitly interchangeable with `List<T>`. APIs requiring a
list must receive `view.to_list()` explicitly. A future common read-only
sequence abstraction may allow algorithms to accept both without copying, but
is outside this decision.

### Representation and lowering

The checker and AIR represent `Slice<T>` as a distinct builtin nominal type,
not as an alias for `List<T>`. AIR uses a dedicated checked slice-view operation
so backends cannot accidentally choose copy semantics.

On the Go target, creating a slice lowers to a full slice expression with
capacity restricted to visible length:

```go
view := source[start:end:end]
```

Conceptually:

```text
Slice<T>     -> []T
mut Slice<T> -> *[]T
```

The pointer-shaped mutable form follows ordinary descriptor-reference lowering
from ADR 0057. Restricting Go capacity to length is defense in depth: accidental
backend or foreign growth allocates rather than overwriting elements beyond the
view. Ard still exposes no capacity operation.

Other backends must preserve the same fixed-length shared-view behavior and
keep backing storage alive for the lifetime of every derived slice.

### Foreign boundaries

`Slice<T>` has the normal Go `[]T` representation at direct Go boundaries. As
with Ard lists under ADR 0057, a compatible named or unnamed Go slice parameter
requires an actual mutable reference because Go has no read-only slice
parameter type:

```ard
let tail = values.slice(start: start, end: end).expect("valid bounds")
go_package::Consume(mut tail)
```

The source value is `mut Slice<T>`, while lowering projects the referenced slice
descriptor and passes exact Go `[]T`, not `*[]T`:

```go
go_package.Consume(*tail)
```

This is an explicit FFI trust boundary. Go may mutate visible elements or
retain the slice, and those mutations remain observable through attached Ard
lists and slices. Because the view's capacity is restricted to its length, a Go
`append` must allocate rather than overwrite elements beyond the visible range.
Replacing or growing the descriptor local to the Go call does not resize the Ard
slice.

An ordinary `Slice<T>` does not satisfy a Go slice parameter without an explicit
`mut` reference. `Slice<T>` also does not implicitly convert to Ard `List<T>`;
those APIs still require `to_list()`.

Imported Go slice results continue to produce Ard `List<T>` values because they
represent complete returned collections rather than explicitly bounded views.

Direct Go pointer-to-slice destinations such as `*[]T` do not initially accept
`mut Slice<T>`. They could replace the referenced descriptor and violate the
fixed-length invariant. Supporting that ABI would require a separate explicit
unsafe or writeback policy.

### Scope

Slices remain non-comparable and cannot be map keys, matching ordinary list
comparability restrictions.

## Consequences

- Ard gains zero-copy list subrange access without exposing growth or capacity.
- Strings gain checked Go-compatible byte substring extraction; callers are
  responsible for choosing UTF-8 boundaries when validity matters.
- Shared mutation is explicit in both the `Slice<T>` type and the existing
  `mut Slice<T>` reference capability.
- No receiver-dependent return typing or duplicate mutable slice type is
  required.
- Bounds failures remain recoverable and consistent with `List.at()` through
  `Maybe`.
- Source growth may detach a list from existing slices, and small slices may
  retain large backing allocations; both behaviors must be documented.
- Algorithms accepting only `List<T>` require explicit `to_list()` allocation
  until a common sequence abstraction exists.
- A `mut Slice<T>` projects to exact Go `[]T` at compatible direct Go slice
  boundaries, while pointer-to-slice destinations remain unsupported.
- The parser needs no new syntax, but the checker, AIR, Go backend, formatter
  type rendering, LSP type display/completion, tests, and documentation need
  support for the new builtin type and methods.

## Related

- ADR 0040: Decouple Mutability from Go Pointer Lowering
- ADR 0045: Support Explicit Mutable Reference Expressions
- ADR 0057: Separate Binding Mutability from Mutable Reference Values
- Go specification: Slice expressions
