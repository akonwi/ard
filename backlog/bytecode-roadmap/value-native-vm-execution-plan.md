# Value-Native VM Execution Plan

This document is the implementation companion to `backlog/bytecode-roadmap/value-native-vm.md`.

The design doc captures the target runtime model and the architectural decisions already made. This execution plan turns those decisions into a staged implementation sequence.

## References

Primary design reference:

- `backlog/bytecode-roadmap/value-native-vm.md`

Related profiling and motivation:

- `backlog/bytecode-roadmap/performance.md`

## Goal

Replace `runtime.Object` as the VM's universal runtime value container with a value-native runtime model built around:

- native Go scalars for Ard scalar values
- erased runtime structs for Ard control-flow/container shapes
- `[]any` frame locals/stacks
- value-native FFI as the default path
- `runtime.Object` as the unsafe/legacy escape hatch during migration

## Non-goals for the first implementation slice

The first slice should not try to finish the full migration.

Specifically, it should not aim to:

- remove `runtime.Object` from every FFI binding immediately
- redesign every container op at once
- migrate all struct/map/list logic in one PR
- solve every debug/printing/tooling concern up front
- delete the legacy object model early

## Guiding principles

1. Keep the VM core moving toward `[]any`, not toward better object boxing.
2. Preserve behavior with compatibility adapters during migration.
3. Prefer small, testable slices over a single rewrite branch.
4. Make the unsafe/legacy path explicit in implementation structure, even if the Go signature already makes it obvious.
5. Use profiling after each meaningful slice.

## Locked design inputs

The following decisions are already made in the design doc and should be treated as inputs to implementation:

- maps use raw Go scalar keys
- runtime map storage uses `Map[K]` behind a `VMMap` abstraction
- struct instances are index-only: `StructValue { TypeID, Fields []any }`
- bytecode/type metadata gains shared struct layout metadata
- `Maybe` and `Result` runtime values are erased structs:
  - `MaybeValue`
  - `ResultValue`
- `runtime.Object` remains only as the unsafe/legacy interop escape hatch during migration

## Master status checklist

Use this section as the source-of-truth status tracker for the migration.

- [x] Stage 1: introduce value-native runtime types
- [x] Stage 2: add compatibility adapters
- [x] Stage 3: add bytecode/type metadata for field-indexed structs
- [x] Stage 4: migrate frame storage to `[]any`
- [x] Stage 5: migrate the easiest opcode families first
- [x] Stage 6: keep legacy FFI working through the unsafe adapter path
- [ ] Stage 7: migrate hot/default FFI bindings to value-native interop
- [ ] Stage 8: migrate containers and structs more deeply
- [ ] Stage 9: shrink and quarantine `runtime.Object`

## Staged execution plan

## Stage 1: introduce value-native runtime types

Status: [ ] not started / [ ] in progress / [x] done

Create the new runtime types without changing the VM stack representation yet.

### Checklist

- [x] add `VoidValue`
- [x] add `MaybeValue`
- [x] add `ResultValue`
- [x] add `ScalarKey`
- [x] add `VMMap`
- [x] add `Map[K]`
- [x] add `MapValue`
- [x] add `StructValue`
- [x] add `EnumValue`
- [x] add focused unit tests for runtime value helpers
- [x] verify `cd compiler && go test ./...`

### Deliverables

Add runtime definitions for:

```go
type VoidValue struct{}

type MaybeValue struct {
    Value any
    None  bool
}

type ResultValue struct {
    Ok    any
    Err   any
    IsErr bool
}

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

type StructValue struct {
    TypeID bytecode.TypeID
    Fields []any
}

type EnumValue struct {
    TypeID bytecode.TypeID
    Tag    int
    Value  any
}
```

### Notes

- These types should land first as additive runtime infrastructure.
- No VM opcode needs to use them yet.
- This stage is mainly about creating the vocabulary of the new runtime.

### Validation

- focused unit tests for constructors and helper behavior
- no semantic regressions in `go test ./...`

## Stage 2: add compatibility adapters

Status: [ ] not started / [ ] in progress / [x] done

Add explicit conversion helpers between the legacy object model and the new value-native model.

### Checklist

- [x] add `ValueToObject(v any, t checker.Type) *runtime.Object`
- [x] add `ObjectToValue(obj *runtime.Object, t checker.Type) any`
- [x] support scalar conversion
- [x] support `MaybeValue`
- [x] support `ResultValue`
- [x] support `MapValue`
- [x] support `StructValue`
- [x] support `EnumValue`
- [x] add round-trip tests: value -> object -> value
- [x] add targeted maybe/result/list/map/struct conversion tests
- [x] verify `cd compiler && go test ./...`

### Deliverables

Helpers like:

```go
func ValueToObject(v any, t checker.Type) *runtime.Object
func ObjectToValue(obj *runtime.Object, t checker.Type) any
```

Supporting helpers for:

- scalar conversion
- `MaybeValue`
- `ResultValue`
- `MapValue`
- `StructValue`
- `EnumValue`
- lists and other container shapes as needed

### Why this stage matters

This adapter layer is what lets us move the VM core first while preserving old FFI.

### Constraints

- adapters should live at the boundary
- do not let adapter use pull the VM core back toward object-boxed execution

### Validation

- round-trip tests: value -> object -> value
- targeted tests for maybe/result/list/map/struct conversion

## Stage 3: add bytecode/type metadata for field-indexed structs

Status: [ ] not started / [ ] in progress / [x] done

Before struct execution can become value-native, the program needs explicit shared struct-layout metadata.

### Checklist

- [x] add first-class struct layout metadata to bytecode program definitions
- [x] update emitter to populate struct layout metadata
- [x] update serializer/deserializer
- [x] update verifier where relevant
- [x] add VM type/layout resolution helpers
- [x] add serialization tests
- [x] add layout lookup tests
- [x] verify existing struct-related VM tests still pass
- [x] verify `cd compiler && go test ./...`

### Deliverables

Extend bytecode program metadata with first-class struct layout entries.

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

Update:

- emitter
- serializer/deserializer
- verifier where relevant
- VM type/layout resolution helpers

### Why this stage is separate

This is shared metadata work, not instance-layout work. It should be stable before struct instances stop using object maps.

### Validation

- serialization tests
- layout lookup tests
- existing struct-related VM tests still passing

## Stage 4: migrate frame storage to `[]any`

Status: [ ] not started / [ ] in progress / [x] done

This is the first truly structural VM change.

### Checklist

- [x] change `Frame.Locals` to `[]any`
- [x] change `Frame.Stack` to `[]any`
- [x] update push/pop helpers
- [x] update frame allocation/reuse
- [x] update closure capture storage
- [x] update argument passing between frames
- [x] use adapters only where still necessary at boundaries
- [x] add focused tests for call/return/closure/local behavior
- [x] verify profiling still works
- [x] verify `cd compiler && go test ./...`

### Deliverables

Change frame representation from boxed objects to erased values:

```go
type Frame struct {
    Locals []any
    Stack  []any
    ...
}
```

Update:

- push/pop helpers
- frame allocation/reuse
- closure capture storage
- argument passing between frames

### Important constraint

At this stage, it is acceptable to use adapters internally at some opcode boundaries if necessary, but the frame representation itself should become value-native.

### Why this is the first real milestone

Once stack/locals are `[]any`, the VM has crossed the most important boundary away from universal object boxing.

### Validation

- all existing VM tests
- focused tests for call/return/closure/local behavior
- verify no regressions in profiling infrastructure

## Stage 5: migrate the easiest opcode families first

Status: [ ] not started / [ ] in progress / [x] done

Do not convert every opcode at once. Migrate the ones with the best effort-to-payoff ratio.

### Checklist

- [x] migrate scalar constants
- [x] migrate arithmetic/comparison ops
- [x] migrate boolean ops
- [x] migrate string/int/float/bool method ops
- [x] migrate maybe/result creation and unwrap ops
- [x] migrate `try` ops
- [x] add targeted tests for `MaybeValue` / `ResultValue` behavior under value-native execution
- [x] verify `cd compiler && go test ./...`

### First targets

- scalar constants
- arithmetic/comparison ops
- boolean ops
- string/int/float/bool method ops
- maybe/result creation and unwrap ops
- `try` ops

### Why these first

- minimal container complexity
- high execution frequency
- easiest place to validate `MaybeValue` / `ResultValue`

### Deliverables

VM ops should operate directly on:

- `string`
- `int`
- `float64`
- `bool`
- `MaybeValue`
- `ResultValue`
- `VoidValue`

### Validation

- package tests for VM
- targeted new tests for maybe/result behavior under value-native execution

## Stage 6: keep legacy FFI working through the unsafe adapter path

Status: [ ] not started / [ ] in progress / [x] done

At this point the VM core may already be value-native while old FFI still exists.

### Checklist

- [x] convert `[]any` args to `[]*runtime.Object` for legacy FFI
- [x] call legacy `runtime.Object`-based bindings through the adapter path
- [x] convert returned objects back into value-native runtime values
- [x] add explicit round-trip tests for value-native VM -> legacy FFI -> value-native VM
- [x] verify existing raw-FFI tests still pass
- [x] verify `cd compiler && go test ./...`

### Deliverables

For any `runtime.Object`-based FFI signature:

- convert `[]any` args to `[]*runtime.Object`
- call the legacy binding
- convert the returned object back into a value-native runtime value

### Rule

This path exists to preserve compatibility, not to define the new runtime model.

### Validation

- existing raw-FFI tests still pass
- explicit tests for value-native VM -> legacy FFI -> value-native VM round trips

## Stage 7: migrate hot/default FFI bindings to value-native interop

Status: [ ] not started / [x] in progress / [ ] done

Once the core VM is running value-native, start moving bindings off the unsafe object path.

### Checklist

- [x] migrate first simple/hot value-native bindings
- [x] prioritize env/string/scalar helpers
- [x] migrate dynamic/json helpers
- [ ] migrate decode helpers where return shapes are manageable
- [ ] avoid early migration of closure-aware or VM-sensitive bindings
- [ ] re-run benchmarks and profiling after each meaningful migration batch
- [x] verify `cd compiler && go test ./...`

### First migration targets

Prefer simple, hot, Go-native bindings first:

- env/string/scalar helpers
- dynamic/json helpers
- decode helpers where the return shape is manageable

### Avoid early migration of

- closure-aware bindings
- VM/runtime-sensitive bindings
- complex struct/enum-return bindings if adapters are still in flux

### Validation

- benchmark + profile after each meaningful migration batch
- prioritize `decode_pipeline` and any new targeted microbenchmarks

## Stage 8: migrate containers and structs more deeply

Status: [ ] not started / [ ] in progress / [ ] done

Once the scalar/maybe/result path is stable:

### Checklist

- [ ] migrate lists toward `[]any`
- [ ] migrate maps toward `Map[K]` / `MapValue`
- [ ] migrate structs toward `StructValue`
- [ ] resolve field access through shared layout metadata
- [ ] benchmark container-heavy workloads after migration steps
- [ ] verify `cd compiler && go test ./...`

Once the scalar/maybe/result path is stable:

- lists move toward `[]any`
- maps move toward `Map[K]` / `MapValue`
- structs move toward `StructValue`
- field access resolves through shared layout metadata

This stage is where container-heavy workloads should begin to show larger gains.

## Stage 9: shrink and quarantine `runtime.Object`

Status: [ ] not started / [ ] in progress / [ ] done

After the VM core and most common FFI paths are value-native:

### Checklist

- [ ] isolate remaining legacy object-based helpers
- [ ] document each remaining unsafe binding
- [ ] measure whether remaining object-based APIs still belong in the runtime core
- [ ] decide whether any compatibility layer should remain permanently quarantined

After the VM core and most common FFI paths are value-native:

- isolate legacy object-based helpers
- document remaining unsafe bindings
- measure whether any remaining object-based surface still belongs in the runtime core

The end-state goal is not necessarily immediate deletion, but clear quarantine.

## Near-term PR checklist

### Recommended first PR scope

The first PR should be intentionally narrow.

Checklist:

- [ ] add the new runtime value types
- [ ] add initial adapter helpers and tests
- [ ] do not migrate frame storage yet
- [ ] do not migrate opcode execution yet

### Why

This creates the foundation for every later step without forcing a risky VM-wide change immediately.

### Recommended second PR scope

Checklist:

- [ ] move frame stack/locals to `[]any`
- [ ] add enough adapter usage to keep existing behavior passing
- [ ] keep opcode conversions minimal and focused

This is the first PR that materially changes runtime execution.

## Profiling checkpoints

Re-profile at these points:

- [ ] after frame storage becomes `[]any`
- [ ] after scalar/maybe/result op migration
- [ ] after first wave of value-native FFI migration
- [ ] after list/map/struct migration begins

Track at least:

- [ ] end-to-end `decode_pipeline`
- [ ] extern timed section
- [ ] closure-call counts
- [ ] object constructor/allocation counts
- [ ] any new value-native allocation counters added later

## Risks

### 1. Adapter creep

Risk:

- adapters become so convenient that new code keeps leaning on them

Mitigation:

- keep adapters at clear boundaries
- avoid using object adapters inside already-migrated VM hot paths

### 2. Mixed-runtime confusion

Risk:

- value-native and object-based representations become hard to reason about

Mitigation:

- document which layers are value-native vs legacy
- keep naming explicit
- add targeted tests for boundary conversions

### 3. Too-large PRs

Risk:

- migration becomes unreviewable or hard to debug

Mitigation:

- keep the first few slices deliberately small
- benchmark and test after each slice

## Success criteria

The migration is on track if:

- the VM core steadily shrinks its reliance on `runtime.Object`
- old FFI still works through adapters during transition
- profiling shows reduced object churn over time
- value-native FFI becomes easier to write than legacy object FFI
- Go interop gets simpler, not more magical

## Immediate next step

Current recommended starting checklist:

- [x] define the new runtime value types
- [x] add object/value adapters
- [x] add focused tests

That is the smallest slice that makes the long-term migration real without overcommitting the first implementation PR.
