# Mutable Generics Implementation Guide

## Overview

This document guides implementation of mutable generic type inference in Ard's checker. The refactoring extends the `Any` type with a `bound` flag to enable in-place mutation during type unification.

**Key architecture:** Each function call creates a fresh copy of the generic function signature with fresh `Any` instances. Within that copy, mutations are immediate and visible to all subsequent argument checking. This enables single-pass resolution where argument N's type checking sees the fully resolved state from arguments 1..N-1.

## Why Extend Any Instead of TypeVar?

The `Any` type already has the right semantics:
- Has a `name` field (generic parameter name like "T", "U")
- Has an `actual` field (the resolved concrete type)
- Is used throughout the codebase

Simply add a `bound` flag to make mutation explicit:
```go
type Any struct {
    name   string
    actual Type
    bound  bool  // NEW: Is actual set?
}
```

This avoids renaming and maintains backward compatibility.

## Implementation Phases

### Phase 1: Add bound flag (types.go)

```go
type Any struct {
    name   string
    actual Type
    bound  bool  // NEW
}
```

Update `Any.String()` to show bound state:
```go
func (a Any) String() string {
    if a.bound && a.actual != nil {
        return a.actual.String()
    }
    return "$" + a.name
}
```

### Phase 2: Update bindGeneric (scope.go)

Change from storing in a map to mutating the Any in-place:

```go
// OLD approach: store in map
func (st *SymbolTable) bindGeneric(name string, t Type) error {
    st.genericContext.bindings[name] = t
    return nil
}

// NEW approach: mutate Any directly
func (st *SymbolTable) bindGeneric(name string, t Type) error {
    // Get the Any instance for this generic
    any := st.genericContext.typeVars[name]
    
    if any.bound {
        // Verify consistency
        if !deref(any.actual).equal(deref(t)) {
            return fmt.Errorf("conflict: %s bound to both %s and %s",
                name, any.actual, t)
        }
    } else {
        // Bind it now
        any.actual = deref(t)
        any.bound = true
    }
    return nil
}
```

### Phase 3: Add deref helper (checker.go)

```go
// deref follows Any bindings to find the concrete type
func deref(t Type) Type {
    if any, ok := t.(*Any); ok && any.bound {
        return deref(any.actual)  // Recursively follow
    }
    return t
}
```

Use in comparisons:
```go
// Before doing equal() checks, dereference first
expected = deref(expected)
actual = deref(actual)
if !expected.equal(actual) {
    // error
}
```

### Phase 4: Update GenericContext (scope.go)

Change tracking from bindings map to typeVars map:

```go
type GenericContext struct {
    typeVars map[string]*Any  // Direct references to Any instances
}

func (st *SymbolTable) createGenericScope(genericParams []string) *SymbolTable {
    gc := &GenericContext{
        typeVars: make(map[string]*Any),
    }
    
    // Create unbound Any instances for each generic parameter
    for _, param := range genericParams {
        gc.typeVars[param] = &Any{
            name:   param,
            actual: nil,
            bound:  false,
        }
    }
    
    return &SymbolTable{
        parent:         st,
        symbols:        make(map[string]*Symbol),
        genericContext: gc,
    }
}

// Get all resolved bindings (used for substitution at end)
func (gc *GenericContext) getGenericBindings() map[string]Type {
    bindings := make(map[string]Type)
    for name, any := range gc.typeVars {
        if any.bound {
            bindings[name] = any.actual
        }
    }
    return bindings
}
```

## How It Works: Example

```ard
fn keep(list: [$T], where: fn($T) Bool) [$T] { ... }

let users = [User{name: "Alice"}, ...]
let a_people = keep(users, fn(u) { u.name.starts_with("A") })
```

**Step 1**: Load original `keep` function definition (has original `Any` instances)
```
Original FunctionDef:
  Parameters[0].Type = &List{of: &Any{name: "T", bound: false}}
  Parameters[1].Type = &FunctionDef{Parameters[0].Type: &Any{name: "T", bound: false}, ...}
```

**Step 2**: Create call-site-specific copy with fresh `Any` instances
```
callSiteGenericScope := c.scope.createGenericScope(["T"])
fnDefCopy := copyFunctionWithFreshGenerics(keep, callSiteGenericScope)
  → fnDefCopy.Parameters[0].Type.of now references callSiteGenericScope's $T
  → callSiteGenericScope.typeVars["T"] = &Any{name: "T", actual: nil, bound: false}
```

**Step 3**: Check arg 1: `[User]` against fnDefCopy.Parameters[0]: `[$T]`
```
unifyTypes(&List{of: callSiteGenericScope.typeVars["T"]}, &List{of: User})
  → bindGeneric("T", User)
    → callSiteGenericScope.typeVars["T"].actual = User
    → callSiteGenericScope.typeVars["T"].bound = true
```

**Step 4**: Check arg 2: anonymous function against fnDefCopy.Parameters[1]: `fn($T) Bool`
```
The $T in fnDefCopy is the call-site-specific one, already bound to User
checkExpr(fn(u) { ... })
  → Parameter type is fn(User) Bool
  → u is inferred as User
  → Type check function body with u: User
    → u.name accesses Str field ✓
    → u.name.starts_with() works ✓
```

## Test Cases

Added to `TestListApi()` in `vm/vm_test.go`:

1. **List::keep with inferred function parameter type**
   - Anonymous function parameter `u` inferred as `User`
   - Accessing `u.name` and calling `starts_with()`
   - Expected: 2 users whose names start with "A"

2. **List::keep with inferred parameter accessing struct fields**
   - Anonymous function parameter `u` inferred as `User`
   - Accessing both `u.name` (Str) and `u.age` (Int)
   - Expected: 2 users with age >= 30

## Files to Modify

- `types.go`: Add `bound` field to `Any`
- `scope.go`: Update `GenericContext` to use `typeVars map[string]*Any`, implement `copyFunctionWithFreshGenerics()`
- `checker.go`: Update `resolveGenericFunction()` to copy function and use call-site-specific `Any` instances
- `checker.go`: Update `bindGeneric()` to mutate `Any` in-place instead of storing in map

## Verification

After implementation, verify with:
```bash
go test ./checker -v           # Type checker tests
go test ./vm -v                # VM tests including new cases
go test ./...                  # Full suite
```

Key test case (already in `TestListApi()` in `vm_test.go`):
```ard
struct User { name: Str }

let users: [User] = [User{name: "Alice"}, User{name: "Bob"}]
let a_people = List::keep(users, fn(u) { u.name.starts_with("A") })
```

Verification checklist:
- ✓ Generic `$T` is bound from first argument `users: [User]`
- ✓ Anonymous function parameter `u` is inferred as `User` (not explicit type annotation needed)
- ✓ Field access `u.name` works because type inference made `u: User`
- ✓ Method call `.starts_with()` works on inferred `Str` type

## Key Points

1. **Copy-once, mutate-within**: Each call creates a fresh function copy with fresh `Any` instances. No cross-call contamination.
2. **Single-pass argument checking**: Arg N sees the fully resolved generics from args 1..N-1
3. **Reference identity preserved per call**: All parameter types within a function copy reference the same `Any` instances
4. **Backward compatible**: Only adds `bound` flag, doesn't rename `Any`
5. **Enables anonymous function parameter inference**: Arg N can be a function whose parameter types are inferred from bound generics
