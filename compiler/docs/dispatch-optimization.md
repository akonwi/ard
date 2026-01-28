# Dispatch Optimization Architecture

## Overview

This document describes the dispatch optimization pattern applied to the VM's eval() loop to eliminate runtime type checks. By pre-computing dispatch discriminators during type checking, we trade small memory overhead (a Kind field) for measurable performance improvements in hot paths.

## The Pattern

**Core principle**: Move type dispatch decisions from runtime (VM) to compile time (checker).

1. Add a `Kind` field to expression nodes that have multiple runtime behaviors
2. During type checking, determine the exact kind and set it on the node
3. In VM evaluation, dispatch directly based on the Kind field instead of runtime type checks

**Benefits**:
- Eliminates runtime type checks from hot paths
- Checker validates correctness at compile time
- Clearer code with explicit dispatch paths
- Minimal memory overhead (single uint8 field)

## Implemented Optimizations

### 1. InstanceProperty - SubjectKind Dispatch

**Problem**: Property access had runtime type checks to determine if subject is a struct.

**Solution**: Pre-compute property dispatch based on subject type.

**Implementation**:
- Added `SubjectKind` enum (currently only `StructSubject` since Ard strings don't have properties)
- Checker sets `Kind` field when creating InstanceProperty nodes
- VM dispatches directly to `Struct_Get()` without runtime checks

**Result**: Eliminated 2 type checks per property access (`IsStruct()` check and type equality check).

**Code locations**:
- `checker/nodes.go`: SubjectKind enum, Kind field on InstanceProperty
- `checker/checker.go`: Pre-compute Kind in property access handling (~line 1914)
- `vm/interpret.go`: Direct dispatch on Kind (~line 373)

### 2. TryOp - TryKind Dispatch

**Problem**: Try expressions had runtime type checks to distinguish between Result and Maybe types.

**Solution**: Pre-compute try behavior based on expression type.

**Implementation**:
- Added `TryKind` enum: `TryResult` and `TryMaybe`
- Checker sets Kind when creating TryOp nodes
- VM has separate code paths but branches on pre-computed Kind instead of runtime checks

**Result**: Eliminated runtime type checks (`IsResult()` and `IsMaybe()` type assertion).

**Code locations**:
- `checker/nodes.go`: TryKind enum, Kind field on TryOp
- `checker/checker.go`: Pre-compute Kind in try expression handling (~line 3388-3510)
- `vm/interpret.go`: Switch-based dispatch on Kind (~line 623)

## Design Decisions

### Single Kind Field vs. Separate Node Types

For InstanceProperty, we chose a single node type with a Kind discriminator rather than separate `StructProperty` and `StrProperty` node types because:

1. **No structural duplication**: All variants have identical fields (Subject, Property, _type)
2. **Simpler to extend**: Adding support for new property types requires only adding a Kind constant and a case in eval()
3. **Clearer intent**: The Kind field explicitly documents that dispatch happens based on subject type

The alternative (separate node types) is appropriate when node structures differ significantly, but not here.

### Compile-Time vs. Runtime Dispatch

The decision to move dispatch to compile time (via pre-computed Kind) rather than relying on runtime type checks is based on:

1. **Correctness by construction**: The checker ensures only valid kind values are set
2. **Performance**: Runtime type checks are non-zero cost; pre-computed enums have zero dispatch overhead
3. **Clarity**: The Kind field explicitly documents what the VM will do
4. **Consistency**: All dispatch is now done the same way (switch on Kind)

## Extension Points

### Adding a New Property Type

If Ard adds properties to another type:

1. Add a variant to `SubjectKind` enum
2. Update checker to set the new Kind
3. Add case in VM dispatch switch

### Adding a New Try-able Type

If Ard adds a new type that works with try (beyond Result and Maybe):

1. Add a variant to `TryKind` enum
2. Update checker to set the new Kind
3. Add case in VM dispatch switch

## Performance Impact

These optimizations have measurable but modest impact:

- **InstanceProperty**: Eliminates 2 type checks per property access (small impact, as property access is not on critical path)
- **TryOp**: Eliminates 2-3 type checks per try evaluation (measurable on error-handling-heavy code)

The real benefit is **architectural clarity**: all dispatch is now explicit and pre-computed, making future optimizations easier to reason about.

## Related Code Patterns

- **Method dispatch** (StrMethod, IntMethod, etc.): Uses the same Kind-based dispatch pattern
- **Primitive methods** (StrMethod, ListMethod, MapMethod, etc.): Established this pattern before InstanceProperty/TryOp
- **Enum dispatch**: Similar pattern used throughout the codebase

## Testing

All optimizations are tested via existing test suites:
- `go test ./checker`: Validates pre-computed Kind values
- `go test ./vm`: Validates dispatch behavior
- Sample programs: Verify end-to-end functionality

Changes are transparent to users; behavior is identical before and after.
