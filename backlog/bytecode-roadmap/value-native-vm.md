# Value-Native VM Runtime Sketch

This document sketches a longer-term direction for the bytecode VM: eliminating `runtime.Object` as the universal runtime value container and replacing it with native Go values plus a small set of Ard-specific runtime shapes.

This is motivated by two goals:

- better runtime performance by removing pervasive boxing/unboxing
- more seamless Go interop by making VM values closer to ordinary Go values

It is intentionally a design sketch, not an implementation plan with exact milestones yet.

## Core idea

Today `runtime.Object` does too many jobs at once:

- VM stack/local value container
- type/kind metadata carrier
- `Maybe` / `Result` tagging mechanism
- FFI boundary transport object
- copy/equality helper surface

The value-native direction is:

- use native Go values for native Ard primitives
- use explicit runtime structs only for Ard-specific composite/control-flow shapes
- let VM frames store `[]any` rather than `[]*runtime.Object`
- let FFI operate on ordinary Go values by default

## Value model

### Native values

These Ard values should map directly to Go values inside the VM:

- `Str` -> `string`
- `Int` -> `int`
- `Float` -> `float64`
- `Bool` -> `bool`
- `Dynamic` -> `any`
- closures/functions -> `*Closure`

### Void

Use a dedicated sentinel for `Void` rather than overloading plain `nil`.

Reason:

- `nil` is already useful for Dynamic/raw interop
- explicit sentinel avoids ambiguity between `Void`, missing values, and `Dynamic(nil)`

Candidate shape:

```go
type VoidValue struct{}

var Void = VoidValue{}
```

## Ard-specific runtime shapes

### Maybe

Use an explicit erased maybe runtime value in the VM core.

Locked-in direction:

```go
type MaybeValue struct {
    Value any
    None  bool
}
```

Why:

- gives the VM one uniform maybe shape
- avoids relying on flags on a universal object wrapper
- keeps `try`, `or`, and maybe-method logic simple
- matches the fact that VM stack/local storage will already be erased to `any`

Notes:

- `Some(x)` -> `MaybeValue{Value: x}`
- `None` -> `MaybeValue{None: true}`
- checker/bytecode metadata still determines the Ard-visible contained type

### Result

Use an explicit erased result runtime value in the VM core.

Locked-in direction:

```go
type ResultValue struct {
    Ok    any
    Err   any
    IsErr bool
}
```

Why:

- gives the VM one uniform result shape
- avoids object-flag encoding
- keeps `try`, `map`, `map_err`, `and_then`, and unwrap logic direct
- matches the erased nature of the VM stack/runtime representation

Notes:

- `Ok(x)` -> `ResultValue{Ok: x}`
- `Err(e)` -> `ResultValue{Err: e, IsErr: true}`
- only one of `Ok` or `Err` is semantically active at a time
- checker/bytecode metadata still determines Ard-visible ok/err types

Optional helper constructors or generic convenience APIs may still exist on the Go side, but they should lower to these erased runtime structs rather than defining a second core runtime representation.

## Lists and maps

### Lists

Lists should become native slices of values:

```go
type ListValue []any
```

This is one of the main performance wins:

- no boxed object per element by default
- simpler interop with Go slices
- simpler iteration in the VM

Copy semantics would still require explicit deep-copy helpers when Ard mutation rules demand them.

### Maps

Maps should use raw Go scalar keys internally, not normalized string keys and not a custom `MapKey` wrapper.

Ard allows exactly these scalar map key types:

- `Str`
- `Int`
- `Float`
- `Bool`

That means the VM can preserve native Go keys directly.

Locked-in direction:

```go
type ScalarKey interface {
    ~string | ~int | ~float64 | ~bool
}

type VMMap interface {
    Len() int
    GetAny(key any) (any, bool)
    SetAny(key any, value any) bool
    DropAny(key any) bool
    HasAny(key any) bool
    Keys() []any
    Copy() VMMap
}

type Map[K ScalarKey] struct {
    Entries map[K]any
}

type MapValue struct {
    KeyType   bytecode.TypeID
    ValueType bytecode.TypeID
    Storage   VMMap
}
```

Rationale:

- preserves raw Go scalar keys directly
- avoids lossy or implicit string normalization
- avoids inventing a custom boxed key representation
- gives the VM a uniform runtime abstraction through `VMMap`
- aligns better with the long-term goal of seamless Go interop

Operational model:

- `[Str:T]` uses `*Map[string]`
- `[Int:T]` uses `*Map[int]`
- `[Float:T]` uses `*Map[float64]`
- `[Bool:T]` uses `*Map[bool]`
- VM code talks to the shared `VMMap` interface
- concrete `Map[K]` implementations perform typed key assertions internally

This design intentionally keeps value typing erased at runtime for now:

- key type is represented concretely by `Map[K]`
- value type remains `any` in storage
- Ard-level type metadata stays available via `MapValue`

### Float-key caveat

Using raw Go `float64` keys means inheriting Go float-key semantics:

- `NaN != NaN`
- `0.0 == -0.0`

That is acceptable for the current design sketch, but should be revisited explicitly if Ard later wants float-map-key semantics that differ from Go.

## Structs and enums

### Structs

Struct runtime values should be index-only at the instance level.

Locked-in direction:

```go
type StructValue struct {
    TypeID bytecode.TypeID
    Fields []any
}
```

Why:

- avoids repeated string-key lookups during execution
- keeps instance layout compact
- avoids per-instance field-name maps
- supports faster field access and future layout optimization

Shared metadata requirements:

- field order/layout metadata associated with struct types
- shared field name <-> field index lookup keyed by `TypeID`
- debugging/tooling/error formatting should resolve names through shared metadata, not instance-local maps

This aligns with the current FFI reality too: Ard structs are not part of the normal generated FFI boundary today except through raw/runtime-aware paths, so there is no strong reason to optimize for name-addressable per-instance struct interop inside the runtime core.

### Struct layout metadata

This question is now resolved.

Locked-in direction:

- bytecode/type metadata should gain first-class struct layout metadata
- struct layout should be shared per type, not stored per instance
- the metadata should be sufficient to support index-based execution without reconstructing layout indirectly from checker type names

Minimum useful shared metadata per struct type:

- `TypeID`
- struct name
- ordered field list
- field name -> field index lookup
- field type IDs by field index

Illustrative shape:

```go
type StructFieldEntry struct {
    Name   string
    TypeID TypeID
}

type StructTypeEntry struct {
    TypeID TypeID
    Name   string
    Fields []StructFieldEntry
}
```

Operational consequences:

- struct instances stay compact as `StructValue { TypeID, Fields []any }`
- field access/set can resolve names through shared layout metadata
- future optimizations can lower field access directly to indexes if desired

This is intentionally more than the current `TypeEntry.Name` string, but much less than introducing a full reflective runtime type system.

### Enums / union-like runtime values

Candidate shape:

```go
type EnumValue struct {
    TypeID bytecode.TypeID
    Tag    int
    Value  any
}
```

This is enough for:

- discriminant checks
- payload storage
- match lowering/runtime dispatch

Exact shape may vary depending on how Ard enums/unions evolve, but the important point is that they become explicit runtime values rather than universal-object metadata.

## VM frame model

A value-native VM should move frame storage to plain `any`.

Candidate shape:

```go
type Frame struct {
    Fn         *bytecode.Function
    IP         int
    Locals     []any
    Stack      []any
    StackTop   int
    MaxStack   int
    ReturnType checker.Type
}
```

This is the central runtime change.

Consequences:

- push/pop operate on `any`
- arithmetic/method ops use type assertions on raw values or explicit runtime structs
- FFI boundaries no longer need to box every argument into `*runtime.Object`

## FFI / Go interop model

This is one of the main reasons to move in this direction.

### Default interop model

By default, FFI should receive and return ordinary Go values.

Examples:

- Ard `Str` -> Go `string`
- Ard `Int` -> Go `int`
- Ard `Dynamic` -> Go `any`
- Ard list -> Go `[]any`
- Ard map -> either `MapValue` or converted Go map form depending on signature
- Ard `extern type` -> raw Go handle/pointer stored directly as `any`

### Complex interop

Some bindings will still need deeper runtime access.

For those, keep `runtime.Object`-based FFI as the unsafe/legacy escape hatch during migration.

Locked-in direction:

- default ABI: value-native Go values
- unsafe/legacy ABI: VM-aware `runtime.Object`-based functions

Important note:

- no extra unsafe annotation is required
- the Go signature itself is the signal
- if a binding takes or returns `runtime.Object`-based types, it is clearly in the unsafe/legacy bucket

That gives the runtime a clean default path while preserving an explicit escape hatch for complex VM-coupled bindings.

## Copy semantics

Removing `runtime.Object` does not remove Ard's copy semantics.

Instead, copy logic becomes explicit per runtime shape.

Need helpers like:

```go
func CopyValue(v any) any
func CopyList(v ListValue) ListValue
func CopyMap(v MapValue) MapValue
func CopyStruct(v StructValue) StructValue
```

Copy rules:

- primitives copy by value naturally
- lists/maps/structs deep-copy where Ard semantics require it
- closures and opaque extern handles remain shared references unless language semantics require otherwise
- `MaybeValue` / `ResultValue` copy their active payloads according to the payload shape

## Equality

Equality should become explicit runtime logic over native values and explicit runtime structs.

Need a helper such as:

```go
func EqualValues(left, right any, leftType checker.Type, rightType checker.Type) bool
```

This should handle:

- scalar equality directly
- `Maybe` equality by none/some + payload equality
- `Result` equality by ok/err + payload equality
- list/map/struct equality according to Ard semantics
- extern handles / dynamic values only where checker rules allow it

## Why this should help performance

Potential wins:

- fewer allocations
- fewer pointers on stack/local paths
- less wrapper churn at FFI boundaries
- less duplicated kind/type metadata per runtime value
- more direct use of Go data structures for Dynamic-heavy workloads

In particular, decode-heavy programs may benefit from:

- `Dynamic` staying as raw `any`
- JSON arrays staying closer to `[]any`
- JSON objects staying closer to keyed raw Go data
- less boxing/unboxing in primitive decode helpers

## Migration direction

This should be staged rather than rewritten all at once.

### Phase 0: define the runtime value types

Before changing execution, define the canonical runtime shapes for:

- `Void`
- `Maybe`
- `Result`
- lists
- maps + key representation
- structs
- enums
- closures

### Phase 1: add adapters

Introduce conversion helpers between the current object model and the new value-native model.

This allows incremental migration of:

- VM ops
- FFI
- tests

### Phase 2: move stack/locals to `any`

Change frame storage first, then migrate op implementations incrementally.

### Phase 3: make value-native FFI the default

Ordinary FFI should work on native Go values by default.

During migration, `runtime.Object`-based FFI remains supported as the unsafe/legacy escape hatch.

### Phase 4: remove `runtime.Object` from hot paths

Once stack, locals, and common FFI no longer need it, remove it from steady-state execution.

### Phase 5: delete or quarantine the old object model

If compatibility adapters are still needed for niche cases, isolate them clearly rather than leaving them in the runtime core.

## Migration adapter decision

This question is now resolved.

Locked-in direction:

- value-native FFI is the default/safe path
- `runtime.Object`-based FFI is the unsafe/legacy escape hatch during migration
- no extra annotation is required because the Go signature already makes the unsafe path obvious
- compatibility adapters should live at the boundary rather than reintroducing object boxing into the VM core

This keeps the long-term direction clear:

- the VM itself becomes value-native
- `runtime.Object` is not part of the future default runtime model
- object-based FFI exists only to preserve compatibility and support complex runtime-coupled bindings while migration is underway

## `Maybe` / `Result` runtime-shape decision

This question is now resolved.

Locked-in direction:

- the VM core uses erased runtime structs: `MaybeValue` and `ResultValue`
- generics are not part of the core runtime representation
- optional Go-side helper constructors or generic convenience APIs may exist, but they should lower to the erased runtime shapes rather than define a second runtime model

This is the simpler and more honest design for a value-native VM because stack/local values are already erased to `any`.

## Open design questions

At the moment, the major open questions in this sketch have been resolved at the architectural level.
Further work should move from high-level design questions to concrete migration planning and implementation sequencing.

## Recommendation

This direction is worth pursuing.

It aligns performance work with the broader product goal of better Go interop instead of just shaving overhead around `runtime.Object`.

The next concrete step should be to refine this sketch into a code-adjacent design that specifies:

- exact runtime type definitions
- map key representation
- struct field layout metadata needs
- migration adapter strategy for current FFI
- first VM subsystem to migrate experimentally
