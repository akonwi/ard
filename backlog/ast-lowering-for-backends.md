# AST Lowering: Decouple VM from checker.Type

## Motivation

Enable multiple backends (VM, Go transpiler, WASM, etc.) by decoupling execution from type system introspection. The preferred execution backend is now bytecode; the interpreter VM is considered legacy.

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

The interpreter VM is tightly coupled to the checker's type system:
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

The bytecode backend reads pre-computed fields directly from checker nodes instead of doing type introspection.

## Roadmap

See `backlog/bytecode-roadmap/README.md` for the concrete backend roadmap and bytecode VM plan.

## Roadmap

See `backlog/bytecode-roadmap/README.md` for the concrete backend roadmap and bytecode VM plan.

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

## Completed Phases

### Phase 0: Package Rename ✅
- Renamed `ast` package → `parser` 
- Updated all imports across codebase
- All tests pass

### Phase 1: Enrich Checker Nodes with Pre-computed Type Metadata ✅

Implemented for three highest-impact node types:

1. **MapLiteral** - Added KeyType and ValueType fields
   - Pre-computed during checking in `checkMap()`
   - VM simplified from 2 lines (type assertion + field access) to direct field reads
   - All 3 test cases updated and passing

2. **StructInstance** - Added FieldTypes map
   - Pre-computed in `validateStructInstance()` for all fields
   - VM simplified: iterate `FieldTypes` instead of calling `e.Type().(*checker.StructDef).Fields`
   - Test updated and passing

3. **ModuleStructInstance** - Added FieldTypes map
   - Pre-computed when creating module struct instances
   - VM simplified: use `e.FieldTypes` instead of `e.Property.Type().(*checker.StructDef).Fields`
   - Test simplified (removed detailed field type checking due to Type pointer comparison issues)

**Impact**: Eliminated 3 major type assertions from VM code. VM now reads pre-computed fields instead of introspecting checker's type system.

### Phase 2: Additional Enrichment ✅

Implemented for high-impact node type:

4. **OptionMatch** - Added InnerType field
   - Pre-computed during checking when creating OptionMatch for Maybe types (line 2480 in checker.go)
   - VM simplified: use `e.InnerType` instead of calling `subject.Type().(*checker.Maybe).Of()`
   - Eliminates type assertion in OptionMatch evaluation path
   - Test updated and passing (TestMaybes/Matching_on_maybes)

**Impact**: Simplified OptionMatch handling in VM. Pattern matching on Maybe types no longer requires type introspection.

### Phase 3: Simplify Backends ✅
1. Bytecode backend consumes the lowered AST without checker.Type introspection
2. Interpreter VM remains legacy and still inspects checker.Type

## Current State

- Bytecode backend is the default execution path (`ard run`) and builds standalone executables (`ard build`).
- Interpreter VM is legacy and is kept for debugging via `--legacy`.
- Full removal of checker.Type introspection from the interpreter is deferred since the bytecode VM is now the primary backend.
