# Method Node Design: Type-Specific Method Call Nodes

## Overview

Replace generic `InstanceMethod` with type-specific method call nodes. Each node type has an enum for method kinds, eliminating string-based dispatch and enabling better code generation for backends.

## Design Pattern

For each primitive/collection type, we create:
1. A `MethodKind` enum (uint8 constants)
2. A typed method node struct
3. Direct fields for common patterns (e.g., arguments as expressions)

Example:
```go
// In nodes.go
type StrMethodKind uint8
const (
    StrSize StrMethodKind = iota
    StrIsEmpty
    StrContains
    // ... etc
)

type StrMethod struct {
    Subject Expression
    Kind    StrMethodKind
    Args    []Expression  // For methods with arguments
}

func (m *StrMethod) Type() Type { ... }
```

Benefits:
- No string switching in VM
- Type-safe method enumeration
- Clear what methods are available
- Easier for backends to code-gen directly
- Self-documenting code

## Method Nodes Specification

### 1. StrMethod
**Current implementation**: `evalStrMethod()` in interpret.go

**Methods**:
- `size() Int`
- `is_empty() Bool`
- `contains(needle: Str) Bool`
- `replace(old: Str, new: Str) Str`
- `replace_all(old: Str, new: Str) Str`
- `split(sep: Str) [Str]`
- `starts_with(prefix: Str) Bool`
- `to_str() Str` (identity)
- `trim() Str`

**Enum**:
```go
type StrMethodKind uint8
const (
    StrSize StrMethodKind = iota
    StrIsEmpty
    StrContains
    StrReplace
    StrReplaceAll
    StrSplit
    StrStartsWith
    StrToStr
    StrTrim
)
```

**Node**:
```go
type StrMethod struct {
    Subject Expression
    Kind    StrMethodKind
    Args    []Expression
}
```

---

### 2. IntMethod
**Current implementation**: `evalIntMethod()` in interpret.go

**Methods**:
- `to_str() Str`

**Enum**:
```go
type IntMethodKind uint8
const (
    IntToStr IntMethodKind = iota
)
```

**Node**:
```go
type IntMethod struct {
    Subject Expression
    Kind    IntMethodKind
}
```

---

### 3. FloatMethod
**Current implementation**: `evalFloatMethod()` in interpret.go

**Methods**:
- `to_str() Str`
- `to_int() Int`

**Enum**:
```go
type FloatMethodKind uint8
const (
    FloatToStr FloatMethodKind = iota
    FloatToInt
)
```

**Node**:
```go
type FloatMethod struct {
    Subject Expression
    Kind    FloatMethodKind
}
```

---

### 4. BoolMethod
**Current implementation**: `evalBoolMethod()` in interpret.go

**Methods**:
- `to_str() Str`

**Enum**:
```go
type BoolMethodKind uint8
const (
    BoolToStr BoolMethodKind = iota
)
```

**Node**:
```go
type BoolMethod struct {
    Subject Expression
    Kind    BoolMethodKind
}
```

---

### 5. ListMethod
**Current implementation**: `evalListMethod()` in interpret.go

**Methods**:
- `at(index: Int) T`
- `prepend(item: T) [T]`
- `push(item: T) [T]`
- `set(index: Int, value: T) Bool`
- `size() Int`
- `sort(compareFn: fn(T, T) Bool) Void`
- `swap(left: Int, right: Int) Void`

**Enum**:
```go
type ListMethodKind uint8
const (
    ListAt ListMethodKind = iota
    ListPrepend
    ListPush
    ListSet
    ListSize
    ListSort
    ListSwap
)
```

**Node**:
```go
type ListMethod struct {
    Subject    Expression
    Kind       ListMethodKind
    Args       []Expression
    ElementType Type  // Pre-computed element type
}
```

---

### 6. MapMethod
**Current implementation**: `evalMapMethod()` in interpret.go

**Methods**:
- `keys() [K]`
- `size() Int`
- `get(key: K) V?`
- `set(key: K, value: V) Bool`
- `drop(key: K) Void`
- `has(key: K) Bool`

**Enum**:
```go
type MapMethodKind uint8
const (
    MapKeys MapMethodKind = iota
    MapSize
    MapGet
    MapSet
    MapDrop
    MapHas
)
```

**Node**:
```go
type MapMethod struct {
    Subject   Expression
    Kind      MapMethodKind
    Args      []Expression
    KeyType   Type  // Pre-computed
    ValueType Type  // Pre-computed
}
```

---

### 7. MaybeMethod
**Current implementation**: `evalMaybeMethod()` in interpret.go

**Methods**:
- `expect(message: Str) T`
- `is_none() Bool`
- `is_some() Bool`
- `or(default: T) T`

**Enum**:
```go
type MaybeMethodKind uint8
const (
    MaybeExpect MaybeMethodKind = iota
    MaybeIsNone
    MaybeIsSome
    MaybeOr
)
```

**Node**:
```go
type MaybeMethod struct {
    Subject   Expression
    Kind      MaybeMethodKind
    Args      []Expression
    InnerType Type  // Pre-computed inner type
}
```

---

### 8. ResultMethod
**Current implementation**: `evalResultMethod()` in result.go

**Methods**:
- `expect(message: Str) T`
- `or(default: T) T`
- `is_ok() Bool`
- `is_err() Bool`

**Enum**:
```go
type ResultMethodKind uint8
const (
    ResultExpect ResultMethodKind = iota
    ResultOr
    ResultIsOk
    ResultIsErr
)
```

**Node**:
```go
type ResultMethod struct {
    Subject Expression
    Kind    ResultMethodKind
    Args    []Expression
    OkType  Type  // Pre-computed OK type
    ErrType Type  // Pre-computed Error type
}
```

---

### 9. StructMethod
**Current implementation**: `EvalStructMethod()` in interpret.go

User-defined methods, cannot use enum (varies per struct).

**Node**: Keep as-is or enhance slightly
```go
type StructMethod struct {
    Subject Expression
    Name    string  // Method name
    Args    []Expression
    // fn already contains function definition
}
```

Implementation doesn't change - still looks up method from struct definition.

---

### 10. EnumMethod
**Current implementation**: `EvalEnumMethod()` in interpret.go

User-defined methods, cannot use enum (varies per enum).

**Node**: Keep as-is
```go
type EnumMethod struct {
    Subject Expression
    Name    string  // Method name
    Args    []Expression
    Enum    *Enum   // Pre-computed enum type
}
```

Implementation doesn't change - still looks up method from enum definition.

---

## Implementation Phases

### Phase 3a: Add specialized method nodes to checker/nodes.go ✅
- Add all 8 method kind enums (Str, Int, Float, Bool, List, Map, Maybe, Result)
- Add all 8 method node types
- Make them satisfy `Expression` interface
- Include `fn *FunctionDef` field for proper generic type resolution

### Phase 3b: Update checker to emit specialized nodes ✅
- In `checkExpr()` when creating method calls, determine subject type
- Emit appropriate method node instead of generic `InstanceMethod`
- Pre-compute meta fields (ElementType, KeyType/ValueType, InnerType, OkType/ErrType)
- Pass function definition to nodes for generic type resolution

### Phase 3c: Update VM to use specialized nodes ✅
- Add 8 direct handler functions (evalStrMethodNode, evalIntMethodNode, etc.)
- Dispatch specialized nodes in eval() for each type
- Remove string-based dispatch from each handler (execute based on Kind enum)
- Keep InstanceMethod dispatch for backward compatibility and runtime-created methods

### Phase 3d: Update checker tests ✅
- Verify OptionMatch still works (uses specialized MaybeMethod)
- All tests pass with new nodes

### Phase 3e: Ready for backends
- Go transpiler can emit different code per method kind
- No need to understand method dispatch logic
- InstanceMethod still used for user-defined types (structs, enums)

## Design Notes

### Generic Type Resolution
Collection method nodes (`ListMethod`, `MapMethod`, `MaybeMethod`, `ResultMethod`) store a `fn *FunctionDef` field to properly resolve generic types. When the method return type depends on generic parameters, we use `fn.ReturnType` which respects the generic resolution done by the checker. The pre-computed type fields (ElementType, KeyType/ValueType, InnerType, OkType/ErrType) are kept for backwards compatibility and direct access when available.

### InstanceMethod Persistence
While most method calls now use specialized nodes, `InstanceMethod` remains for:
1. **User-defined types** (structs and enums) - these have variable method names
2. **Runtime-created methods** (e.g., template strings that call `to_str()` on arbitrary types)
3. **Fallback dispatch** - ensures future method types can still work

## Backward Compatibility

- `InstanceMethod` kept for backward compatibility and runtime method creation
- No API changes to checker exports
- Tests verify behavior doesn't change
- Specialized nodes are transparent to users of the checker module

## Testing Strategy

1. **Unit tests**: StrMethod, IntMethod, etc. emit correct results
2. **Integration tests**: Existing VM tests pass (transparently using new nodes)
3. **Verification**: Method dispatch works correctly for each type

## Code Generation Example

**Current (generic dispatch)**:
```go
// VM must understand all types
case *checker.InstanceMethod:
    subj := vm.eval(...)
    if subj.Type() == checker.Str {
        // switch on method name
        switch e.Method.Name {
        case "size":
            return runtime.MakeInt(len(subj.AsString()))
        }
    }
```

**New (specific nodes)**:
```go
// VM is dumb executor
case *checker.StrMethod:
    subj := vm.eval(...)
    switch e.Kind {
    case checker.StrSize:
        return runtime.MakeInt(len(subj.AsString()))
    }
```

**Backend code generation**:
```go
// Go backend can emit directly
case *checker.StrMethod:
    switch e.Kind {
    case checker.StrSize:
        codegen.emit(fmt.Sprintf("len(%s)", codegen.expr(e.Subject)))
    }
```

