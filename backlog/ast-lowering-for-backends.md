# AST Lowering: Decouple VM from checker.Type

## Motivation

Enable multiple backends (VM, Go transpiler, WASM, etc.) by decoupling the interpreter from type system introspection. The VM should be a simple executor that walks pre-lowered instructions, not a system that understands the checker's type representation.

This makes it straightforward to:
1. Generate different target languages from the same checked code
2. Reason about the VM as a dumb executor
3. Evolve the type system without breaking backends
4. Add compilation targets (Go code generation, WASM, etc.)

## Current Architecture Problem

```
Source Code
    ↓
parse.AST (minimal, syntax only)
    ↓
checker.AST (type-checked, but incomplete metadata)
    ↓
VM must introspect checker.Type to execute
    ↓
Runtime values
```

The VM is tightly coupled to the checker's type system:
- Calls `.Type()` on AST nodes (38+ times in vm/interpret.go)
- Does type assertions: `e.Type().(*checker.Map)`, `e.Type().(*checker.StructDef)`
- Derives type metadata at runtime instead of using pre-computed values

## Target Architecture

The checker becomes the IR producer by enriching its nodes with pre-computed metadata:

```
Source Code
    ↓
parse.AST (minimal, syntax only)
    ↓
checker.AST (enriched with pre-computed fields)
    ↓
    ┌─────┼──────┐
    ↓     ↓      ↓
   VM   Go    WASM
(simple)(simple)(simple)
backend backend backend
    ↓     ↓      ↓
 Values  Code  Binary
```

Each backend reads pre-computed fields directly from checker nodes instead of doing type introspection.

## Concrete Changes Required

### Case 1: MapLiteral

**Before:**
```go
// checker/nodes.go
type MapLiteral struct {
    Keys   []Expression
    Values []Expression
    _type  Type
}

// vm/interpret.go
case *checker.MapLiteral:
    mapType := e.Type().(*checker.Map)              // Type assertion
    _map := runtime.MakeMap(mapType.Key(), mapType.Value())
```

**After:**
```go
// checker/nodes.go - ADD these fields
type MapLiteral struct {
    Keys      []Expression
    Values    []Expression
    _type     Type
    KeyType   Type    // Pre-computed by checker
    ValueType Type    // Pre-computed by checker
}

// checker/check.go - populate during checking
mapType := lit._type.(*checker.Map)
lit.KeyType = mapType.Key()      // Compute once
lit.ValueType = mapType.Value()  // Compute once

// vm/interpret.go - direct field access, no assertion
case *checker.MapLiteral:
    _map := runtime.MakeMap(e.KeyType, e.ValueType)
```

### Case 2: StructInstance

**Before:**
```go
// vm/interpret.go
case *checker.StructInstance:
    strct := e.Type().(*checker.StructDef)      // Type assertion
    raw := map[string]*runtime.Object{}
    for name, ftype := range strct.Fields {     // Get fields from type
        val, ok := e.Fields[name]
        if ok {
            val := vm.eval(scp, val)
            val.SetRefinedType(ftype)
            raw[name] = val
        } else {
            raw[name] = runtime.MakeNone(ftype)
        }
    }
    return runtime.Make(raw, e.Type())
```

**After:**
```go
// checker/nodes.go - ADD this field
type StructInstance struct {
    Name       string
    Fields     map[string]Expression
    _type      *StructDef
    FieldTypes map[string]Type    // Pre-computed field types
}

// checker/check.go - populate during checking
lit.FieldTypes = make(map[string]Type)
for name, fieldType := range lit._type.Fields {
    lit.FieldTypes[name] = fieldType
}

// vm/interpret.go - use pre-computed map
case *checker.StructInstance:
    raw := map[string]*runtime.Object{}
    for name, ftype := range e.FieldTypes {  // Direct access
        val, ok := e.Fields[name]
        if ok {
            val := vm.eval(scp, val)
            val.SetRefinedType(ftype)
            raw[name] = val
        } else {
            raw[name] = runtime.MakeNone(ftype)
        }
    }
    return runtime.Make(raw, e.Type())
```

### Case 3: ModuleStructInstance

**Before:**
```go
case *checker.ModuleStructInstance:
    strct := e.Property.Type().(*checker.StructDef)  // Type assertion
    raw := map[string]*runtime.Object{}
    for name, ftype := range strct.Fields {
        // ...
    }
```

**After:**
```go
// checker/nodes.go
type ModuleStructInstance struct {
    Property   *ModuleSymbol
    Fields     map[string]Expression
    FieldTypes map[string]Type    // ADD
    _type      Type
}

// vm/interpret.go
case *checker.ModuleStructInstance:
    raw := map[string]*runtime.Object{}
    for name, ftype := range e.FieldTypes {  // Direct access
        // ...
    }
```

## Summary Table

| Node Type | Current | After | Benefit |
|-----------|---------|-------|---------|
| MapLiteral | `e.Type().(*checker.Map).Key()` | `e.KeyType` | No assertion, direct field |
| StructInstance | `e.Type().(*checker.StructDef).Fields` | `e.FieldTypes` | No assertion, no type lookup |
| ModuleStructInstance | `e.Property.Type().(*checker.StructDef).Fields` | `e.FieldTypes` | No assertion, single field |

## Implementation Plan

### Phase 1: High-Impact Cases
1. MapLiteral - 1 assertion eliminated
2. StructInstance + ModuleStructInstance - 2 assertions eliminated
3. Simplifies ~6 lines of VM code total

### Phase 2: Method Dispatch Simplification
Cases like `evalInstanceMethod` (8+ type checks) could use similar pre-computation on AST nodes to dispatch to correct handler.

### Phase 3: Backend Support
- Go transpiler can read the same lowered AST
- WASM compiler can consume the same metadata
- No backend needs to understand `checker.Type`

## Non-Performance Benefits

While the type assertions aren't a performance bottleneck, the refactoring provides:

1. **Clarity**: VM code reads as "execute instruction with metadata" not "figure out the type, then execute"
2. **Decoupling**: VM can be tested/evolved independently of checker
3. **Extensibility**: New backends don't need checker.Type knowledge
4. **Maintainability**: Reduces places where VM depends on type system internals

## Scope

- Rename `ast` package → `parse` (clarifies it's the parse tree, not the semantic AST)
- Minimal API changes to checker nodes (add fields to ~5 node types)
- Localized checker changes (populate fields during checking)
- Straightforward VM simplifications (replace assertions with field reads)
- No behavior changes, only architecture

## Next Steps

### Phase 0: Package Rename
1. Rename `ast` → `parse`
2. Update all imports across codebase
3. Verify tests pass

### Phase 1: Enrich Checker Nodes
1. Start with MapLiteral (simplest, single assertion)
2. Verify no test breakage
3. Move to StructInstance patterns
4. Document any edge cases discovered

### Phase 2: Simplify Backends
1. Update VM to use pre-computed fields
2. Consider Go transpiler backend next
3. WASM backend can follow once transpiler is stable
