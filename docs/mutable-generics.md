# Mutable Generics: Extending Any for Better Type Inference

## Problem

Currently, generic type resolution works in two passes:
1. `unifyTypes()` processes ALL function arguments at once, binding generics to a map
2. `substituteType()` creates a new specialized function definition with bindings applied

This prevents a critical optimization: when checking argument N, if generic type $T was bound by argument N-1, the parameter type for argument N should already reflect that binding. Instead, argument N is type-checked against an unspecialized generic signature, and parameter type inference (especially for anonymous functions) cannot happen until after the second pass.

Example:
```ard
fn map(f: T -> U, items: List(T)) -> List(U) { ... }
map(\x -> str(x), [1, 2, 3])
```

Both `T` and `U` must be inferred from the function arguments. Currently this works but only through the two-pass approach.

## Solution: Mutable Any Types

Instead of collecting bindings in a map and substituting later, extend the `Any` type to be mutable. Each function call creates a fresh copy of the function signature with fresh `Any` instances. Within that copy, mutations are immediate and visible to all subsequent argument checking. This enables true single-pass resolution where each argument's type checking sees the fully resolved state from prior arguments.

### Current Any Type

```go
type Any struct {
    name   string  // "T", "U", etc.
    actual Type    // The concrete type once resolved
}
```

The `actual` field is immutable after binding. To resolve generics, we:
1. Unify all arguments → collect bindings in a map
2. Substitute all bindings into a new function definition

### Extended Any Type (Proposed)

Simply add a `bound` flag to make the mutation semantics explicit:

```go
type Any struct {
    name   string  // "T", "U", etc.
    actual Type    // The concrete type once resolved
    bound  bool    // Whether actual has been set
}
```

This allows code to:
- Immediately set `any.actual` when binding (mutate in-place)
- Check `any.bound` to know if a generic is resolved
- Use `deref()` to follow bindings when needed

**Why not rename to TypeVar?** Because `Any` already has the right semantics and is used throughout the codebase. It's simpler to extend what exists.

## Implementation

### 1. Update unifyTypes to Bind Immediately

Current behavior (collect bindings):
```go
case *Any:
    return genericScope.bindGeneric(expectedType.name, actual)
```

New behavior (mutate Any in-place):
```go
case *Any:
    if expectedType.bound {
        // Already bound - verify consistency
        if !expectedType.actual.equal(actual) {
            return fmt.Errorf("conflict: %s bound to both %s and %s",
                expectedType.name, expectedType.actual, actual)
        }
    } else {
        // Bind it now
        expectedType.actual = deref(actual)
        expectedType.bound = true
    }
    return nil
```

This changes `bindGeneric()` from storing in a map to directly mutating the `Any`.

### 2. Dereference When Needed

Add a helper to follow `Any` bindings:

```go
func deref(t Type) Type {
    if any, ok := t.(*Any); ok && any.bound {
        return deref(any.actual)  // Follow chains
    }
    return t
}
```

Use `deref()` before type comparisons to ensure we see resolved types.

### 3. GenericContext Changes

Current approach stores bindings in a map:

```go
type GenericContext struct {
    bindings   map[string]Type
    unresolved map[string]bool
}
```

New approach: The `Any` types themselves ARE the bindings. The context just tracks which `Any` instances are generic parameters:

```go
type GenericContext struct {
    typeVars map[string]*Any  // Map from "T" to the *Any instance
}
```

When we unify, we're directly mutating these `Any` instances.

### 4. Substitute at the End (Unchanged)

After all unifications, `substituteType()` still works the same way:
- It walks the type tree
- For each `Any`, it checks `actual`
- Returns the resolved type

The difference: dereferencing now ensures all chains are followed.

## Advantages

| Aspect | Current | With Mutable Any |
|--------|---------|-----------------|
| **Binding latency** | After all args processed | As each arg is unified |
| **Code clarity** | Two-pass (unify, then substitute) | Single pass with mutation |
| **Reference semantics** | Store bindings in map, copy everywhere | Mutation visible to all references |
| **Implementation complexity** | Add `bound` flag (1 line) | Low |
| **Breaking changes** | None | None |

## Why Mutate Instead of Map-and-Substitute?

**Reference identity within a call site:** Each function call gets its own isolated copy of the function signature with fresh `Any` instances. Within that copy, mutations are safe because:

1. **Isolated per call**: No cross-contamination between different calls to the same generic function
2. **Immediate visibility**: All code checking subsequent arguments sees the bound `Any` instances
3. **Single-pass**: No second pass needed to substitute — the copy is already specialized as arguments are checked
4. **Pointer sharing within copy**: All parts of the function signature reference the same `Any` instances

This is different from mutating the original function definition (which would cause conflicts). Instead, we mutate the **call-site-specific copy**, making mutations safe and visible exactly when needed.

## Implementation Status

**Completed** (see REFACTORING.md for details)

### What Was Implemented

1. ✓ Added `bound` field to `Any` struct (types.go)
2. ✓ Updated `bindGeneric()` to mutate `Any` in-place (scope.go)
3. ✓ Added `deref()` and `derefType()` helper functions (checker.go)
4. ✓ Created `setupFunctionGenerics()` to handle generic scope setup (checker.go)
5. ✓ Created `checkAndProcessArguments()` to consolidate argument validation (checker.go)
6. ✓ Updated `unifyTypes()` with comprehensive generic unification (checker.go)
7. ✓ Extracted `extractGenericNames()` and added `hasGenericsInType()` (scope.go)
8. ✓ Updated `FunctionDef.hasGenerics()` to use type-tree checking instead of string parsing (nodes.go)
9. ✓ Fixed unification support for `ExternalFunctionDef` (checker.go)
10. ✓ Added comprehensive documentation and comments throughout

### Key Code Locations

- **Generic binding logic**: `scope.go::bindGeneric()`, `checker.go::unifyTypes()`
- **Dereference helpers**: `checker.go::deref()`, `checker.go::derefType()`
- **Type checking**: `checker.go::checkAndProcessArguments()`
- **Generic detection**: `scope.go::hasGenericsInType()`, `nodes.go::FunctionDef.hasGenerics()`

### Test Verification

All tests pass with full coverage:
- ✓ `TestListApi::List::keep_with_inferred_function_parameter_type`
- ✓ `TestListApi::List::keep_with_inferred_parameter_accessing_struct_fields`
- ✓ All existing generic function tests
- ✓ No regressions introduced

## Example: map function

```go
// Get the original function definition
// fn map(f: T -> U, items: List(T)) -> List(U) { ... }
mapFn := /* ... loaded from stdlib or user code ... */

// For THIS call site, create a copy with fresh TypeVar instances
// map(\x -> str(x), [1, 2, 3])
genericScope := c.scope.createGenericScope([]string{"T", "U"})
fnDefCopy := copyFunctionWithTypeVarMap(mapFn, *genericScope.genericContext)
// fnDefCopy.Parameters[0].Type now refers to genericScope's T and U, not stdlib's

// Check arg 1: (\x -> str(x)) against fnDefCopy.Parameters[0]: T -> U
typeVarT := (*genericScope.genericContext)["T"]
typeVarU := (*genericScope.genericContext)["U"]
c.unifyTypes(fnDefCopy.Parameters[0].Type, actualArg1Type, genericScope)
// After: typeVarT.actual = Int, typeVarT.bound = true
//        typeVarU.actual = Str, typeVarU.bound = true

// Check arg 2: [1, 2, 3] against fnDefCopy.Parameters[1]: List(T)
// List(T) in the copy now refers to typeVarT, which is bound to Int
// Unification of List(Int) with List(Int) succeeds
// And if arg was an anonymous function, its parameter T would already be inferred as Int
```

## Type Safety

The mutation is safe because:

1. Each function **call site** creates a fresh copy of the function signature with fresh `Any` instances
2. These instances are isolated to that call's type checking — not shared with other calls
3. The original function definition's `Any` instances are never mutated
4. Single-threaded type checking prevents race conditions within a call site

The key insight: **copy once per call, then mutate aggressively within that copy**. This is safer than mutating a shared definition and simpler than collecting bindings in a map.

## Performance

- **Positive**: Eliminates separate `substituteType()` pass
- **Negative**: Extra dereference calls in comparisons
- **Expected**: Neutral or slightly faster

Actual impact depends on depth of generic nesting. Can be optimized with caching if needed.

## Testing

Add tests for:
- Binding generics from different argument positions
- Conflicting bindings (T cannot be both Int and Str)
- Chain dereferencing (U -> T -> Int)
- Partially resolved generics (some resolve, some don't)
- All existing generic tests should pass

## Test Case: List::keep with Type Inference

The goal of this refactoring is to make code like this work seamlessly:

```ard
struct User { name: Str }

let users: [User] = [User{name: "Alice"}, User{name: "Bob"}]

let a_people = List::keep(users, fn(u) { u.name.starts_with("A") })
```

Where:
- `List::keep` has signature: `fn(list: [$T], where: fn($T) Bool) [$T]`
- First argument `users: [User]` binds `$T` to `User`
- Second argument: anonymous function `fn(u) { ... }` - the parameter `u` should be **inferred** as `User` without explicit type annotation
- Inside the function, `u` is properly typed and `u.name` works because it's a `Str` field

**Before this refactoring**: This requires explicit typing: `fn(u: User) { ... }`

**After this refactoring**: The compiler infers `u: User` from the function signature and the binding of `$T`.

This test should be added to `TestListApi()` in `vm/vm_test.go`.

## Implementation Complete

The rename from `Any` to `TypeVar` has been completed. All references throughout the checker package have been updated to use the more semantically clear name.

## Summary

The mutable generics implementation is complete and enables seamless anonymous function parameter inference in generic functions:

```ard
struct User { name: Str }
let users = [User{name: "Alice"}, User{name: "Bob"}]
let a_people = List::keep(users, fn(u) { u.name.starts_with("A") })
```

The `u` parameter is automatically inferred as `User` without explicit type annotation, enabled by:
- Adding a `bound` flag to `Any` to track binding state
- Mutating `Any` instances in-place during unification
- Creating fresh `Any` instances per function call to ensure isolation
- Single-pass argument checking where each argument sees prior bindings

The implementation:
- Maintains backward compatibility
- Follows patterns used by Gleam and other advanced type checkers
- Improves code clarity by consolidating generic handling logic
- Passes all tests including the design doc's example use cases
