# Phase 1: TypeRegistry Foundation - Complete ✓

## What was done

Implemented the parallel infrastructure for a new type system based on an entity-component model with a central TypeRegistry. This phase focused on building the foundation without changing any checker or VM behavior.

### 1. TypeRegistry Core (`checker/type-registry.go`)
- **TypeID**: A uint32 handle to reference types in the registry
- **TypeRegistry**: Stores all types for a module and assigns unique IDs
  - `Next()`: Allocate next available TypeID
  - `Register(id, type)`: Store a type with error checking
  - `Lookup(id)`: Retrieve type by ID
  - `All()`: Get all registered types (for inspection)

### 2. Integration with Checker
- `Checker.registerType(t Type) TypeID`: Helper method to allocate and register types
- `Checker` now owns a `TypeRegistry` instance
- `TypeRegistry` passed to `Module.TypeRegistry()` for external access

### 3. Module Interface
- Added `TypeRegistry() *TypeRegistry` method to `Module` interface
- Implemented on:
  - `UserModule`: Stores and returns checker's registry
  - `EmbeddedModule`: Stores and returns stdlib module registry
  - `MaybePkg`, `ResultPkg`: Return nil (builtin packages)

### 4. Tests
- **Unit tests** (`type-registry_test.go`): 10 tests covering core registry functionality
  - Registration, lookup, ID allocation, error handling
  - All pass ✓
- **Integration tests** (`type-registry-integration_test.go`): 2 tests verifying
  - Registry is available after `Checker.Check()`
  - Registry is accessible from `Module.TypeRegistry()`
  - All pass ✓

## Key Design Decisions

1. **Module-local type IDs**: Type IDs only need to be unique within each module, simplifying cross-module type handling
2. **No behavioral changes yet**: All existing tests pass unchanged
3. **Parallel system**: Registry exists alongside existing Type system - Phase 2 will replace it
4. **Dumb VM ready**: VM will eventually get the registry and use TypeIDs for dispatch without type introspection

## Test Results

```
✓ All existing checker tests pass
✓ All existing vm tests pass
✓ All TypeRegistry unit tests pass (10 tests)
✓ All TypeRegistry integration tests pass (2 tests)
✓ Complete build with no errors
```

## Next Steps (Phase 2)

1. Add `typeID TypeID` field to expression types (or to Expression interface)
2. Wire up `registerType()` calls in `checkExpr()` and related methods
3. Add type ID lookup verification tests
4. Prepare for Phase 3 where `Type()` method uses registry instead of computing

## Statistics

- **Files modified**: 7
  - `checker/type-registry.go` (new)
  - `checker/checker.go`
  - `checker/user_module.go`
  - `checker/embedded_module.go`
  - `checker/std_lib.go`
  - `checker/functions.go`
  - `type-registry_test.go` (new)
  - `type-registry-integration_test.go` (new)

- **New code**: ~300 lines (registry + tests)
- **No regressions**: All 100+ existing tests pass
