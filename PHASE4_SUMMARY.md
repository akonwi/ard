# Phase 4: Registry-Based Type Lookup

## Overview

Phase 4 completes the strangler fig pattern migration by enabling expressions to access their types through the TypeRegistry. While Phase 1-3 built the infrastructure and registered types in parallel, Phase 4 provides the interface for expressions to retrieve their types from the registry.

## Architecture

### Key Components Added

1. **Expression.GetTypeID() Interface Method**
   - Added to Expression interface (nodes.go:25)
   - Provides access to expression's allocated TypeID
   - Implemented on all 48 expression types
   - Returns InvalidTypeID (0) for expressions not yet registered

2. **Checker.LookupType() Helper**
   - New method in checker.go (line 43)
   - Retrieves type from registry using expression's TypeID
   - Falls back to computed Type() during transition
   - Enables safe, gradual migration

3. **Expression TypeID Fields**
   - Added typeID field to 8 additional types:
     - If
     - FunctionDef  
     - ExternalFunctionDef
     - EnumVariant (changed to pointer receiver)
     - ModuleStructInstance
     - ModuleSymbol
     - FiberExecution (changed to pointer receiver)
   - All now implement Expression interface properly

### Design: Strangler Fig Pattern

```
During Transition (Current State):
┌─────────────────────────────────────────────┐
│ Expression with typeID field set during     │
│ checkExpr() via registerExpr()              │
└─────────────────────────────────────────────┘
         │                    │
         ▼                    ▼
    GetTypeID()          Type() [computed]
         │                    │
         ├──LookupType()──────┘
         │   (hybrid lookup)
         ▼
    TypeRegistry
    (parallel system)
```

The system currently operates in "read-from-both" mode:
- Expressions register their types in registry
- Expressions still compute types via Type() methods
- LookupType() tries registry first, falls back to computed type
- Zero behavioral changes to existing code

## Changes Summary

### Files Modified

#### checker/nodes.go
- Added GetTypeID() interface method to Expression
- Implemented GetTypeID() on all 48 expression types (lines 1113-1297)
- Added typeID fields to 8 types (If, FunctionDef, ExternalFunctionDef, EnumVariant, ModuleStructInstance, ModuleSymbol)
- Changed EnumVariant methods to pointer receivers

#### checker/checker.go
- Added LookupType(expr Expression) Type helper function (lines 43-62)
- Enables registry-based lookups with computed type fallback

#### checker/functions.go
- Added typeID field to FiberExecution
- Changed receiver types to pointer receivers for consistency

#### checker/checker_test.go
- Updated compareOptions to ignore typeID fields during test comparisons
- Added new types to IgnoreUnexported list

#### checker/phase3_test.go
- 4 new Phase 4 tests validating registry lookups:
  - TestPhase4_GetTypeIDMethod
  - TestPhase4_LookupTypeHelper
  - TestPhase4_RegistryHybridTransition
  - TestPhase4_TypeIDPersistence

## Test Results

All tests pass:
- 100+ existing tests: ✓ PASS (no behavioral changes)
- 4 new Phase 4 tests: ✓ PASS

Example test output:
```
Phase 4: All expressions have valid GetTypeID() implementations
Phase 4: LookupType verified for 1 expressions
Phase 4: Hybrid transition verified with 11 types registered
Phase 4: Type ID persistence verified
```

## Technical Details

### Type Registration Flow

1. **During checkExpr():**
   ```go
   expr := &IntLiteral{Value: 42}
   expr = c.registerExpr(expr)  // Allocates TypeID, stores in registry, sets field
   ```

2. **Type Lookup via LookupType():**
   ```go
   t := c.LookupType(expr)
   // Returns: registry.Lookup(expr.GetTypeID()) or expr.Type() as fallback
   ```

3. **Registry Storage:**
   ```go
   // TypeRegistry tracks all types
   registry[TypeID(1)] = Int
   registry[TypeID(2)] = Int
   registry[TypeID(3)] = Str
   // Each expression gets unique ID, maps to registered Type
   ```

### Key Invariants

1. **All expressions have typeID**: Either InvalidTypeID (not registered) or valid ID
2. **Valid typeIDs resolve**: registry.Lookup(typeID) != nil for all registered types
3. **Backward compatible**: Type() methods unchanged, existing tests pass
4. **Hybrid mode**: LookupType() provides safe fallback during transition

## Next Steps (Phase 5)

The current phase provides the foundation for future optimizations:

1. **Direct Registry Lookups**: Gradually replace Type() calls with c.LookupType()
2. **Performance**: Cache type lookups to avoid registry overhead
3. **Lazy Initialization**: Delay type computation until first lookup
4. **Type Verification**: Use registry as source of truth for type checking

## Implementation Notes

### Design Decision: GetTypeID() Over Global Registry

We chose to add GetTypeID() method rather than using a global registry because:
1. **Type Safety**: Compiler enforces implementation on all expression types
2. **Explicit Intent**: Clear which expressions participate in registry
3. **Test Isolation**: Tests aren't affected by registry state
4. **Future Flexibility**: Allows per-expression type strategies

### Receiver Type Changes

Some types (EnumVariant, FiberExecution) changed from value to pointer receivers for consistency with Expression interface pattern. This is safe because:
1. No existing code depends on value receivers
2. Aligns with immutability pattern (values are checked, not modified)
3. Consistent with other expression types

## Verification Checklist

- [x] All expressions implement GetTypeID()
- [x] TypeIDs allocated and stored during checkExpr()
- [x] Registry contains all checked expression types
- [x] LookupType() returns correct types with fallback
- [x] All existing tests pass (backward compatible)
- [x] 4 new Phase 4 tests pass (new functionality)
- [x] No behavioral changes to compiler
- [x] typeID fields properly initialized during expression creation
