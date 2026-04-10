# FFI Refactoring Opportunities

This document tracks identified improvements to reduce FFI complexity and maintenance burden.

See also: [ffi-simplification.md](./ffi-simplification.md) for analysis of breaking down complex FFI functions using opaque types.

## Current State

The FFI system has **62 functions total**: 46 idiomatic, 16 raw.

Raw functions remain raw because the generator doesn't support their required types.

Some raw functions could be simplified by using opaque handles and moving logic to Ard code.

## Opportunity 1: Add `[]any` Parameter and Return Support

**Gap:** Generator doesn't support `[]any` (slice of any type).

**Impact:** These functions could be idiomatic:
- `ListToDynamic` - takes `[]any`, returns `any`
- `DynamicToList` - takes `any`, returns `([]any, error)`
- `SqlQuery`, `SqlExecute` - need `[]any` for values parameter

**Implementation in `ffi_generate.go`:**

Add to `parseGoType()`:

```go
case *ast.ArrayType:
    if t.Len == nil {
        if ident, ok := t.Elt.(*ast.Ident); ok {
            switch ident.Name {
            case "string", "int", "float64", "bool":
                return GoType{Base: ident.Name, IsSlice: true}, true
            case "any":// NEW
                return GoType{Base: "any", IsSlice: true}, true
            }
        }
    }
```

Generated wrapper for `[]any` param:

```go
// func Foo(items []any)
_sl0 := args[0].AsList()
arg0 := make([]any, len(_sl0))
for _i, _e := range _sl0 {
    arg0[_i] = _e.Raw()
}
```

Generated wrapper for `[]any` return:

```go
// func Foo() []any
result := ffi.Foo()
_items := make([]*runtime.Object, len(result))
for _i, _v := range result {
    _items[_i] = runtime.MakeDynamic(_v)
}
return runtime.MakeList(checker.Dynamic, _items...)
```

---

## Opportunity 2: Add `map[string]any` Parameter Support

**Gap:** Generator supports `map[string]string` but not `map[string]any`.

**Impact:** `MapToDynamic` could be idiomatic.

**Implementation:**

Add `IsStringMapAny bool` field to `GoType` struct.

Generated wrapper:

```go
// func Foo(m map[string]any)
_rawMap0 := args[0].AsMap()
arg0 := make(map[string]any, len(_rawMap0))
for _k, _v := range _rawMap0 {
    arg0[_k] = _v.Raw()
}
```

---

## Opportunity 3: Extract Type Lookup Helper

**Current pattern (repeated in 3 files):**

```go
var (
    fsDirEntryType     checker.Type
    fsDirEntryTypeOnce sync.Once
)

func getFSDirEntryType() checker.Type {
    fsDirEntryTypeOnce.Do(func() {
        mod, ok := checker.FindEmbeddedModule("ard/fs")
        if !ok {
            panic("failed to load ard/fs embedded module")
        }
        sym := mod.Get("DirEntry")
        if sym.Type == nil {
            panic("DirEntry type not found in ard/fs module")
        }
        fsDirEntryType = sym.Type
    })
    return fsDirEntryType
}
```

**Proposal:** Create `ffi/types.go`:

```go
package ffi

import (
    "fmt"
    "sync"

    "github.com/akonwi/ard/checker"
)

var (
    typeCache   = make(map[string]checker.Type)
    typeCacheMu sync.RWMutex
)

// GetStdType returns an Ard type from a standard library module.
// It caches the lookup for subsequent calls.
//
// Example: GetStdType("ard/fs", "DirEntry")
func GetStdType(modulePath, typeName string) checker.Type {
    key := modulePath + "." + typeName

    typeCacheMu.RLock()
    if t, ok := typeCache[key]; ok {
        typeCacheMu.RUnlock()
        return t
    }
    typeCacheMu.RUnlock()

    typeCacheMu.Lock()
    defer typeCacheMu.Unlock()

    if t, ok := typeCache[key]; ok {
        return t
    }

    mod, ok := checker.FindEmbeddedModule(modulePath)
    if !ok {
        panic(fmt.Errorf("FFI: module %q not found", modulePath))
    }
    sym := mod.Get(typeName)
    if sym.Type == nil {
        panic(fmt.Errorf("FFI: type %q not found in module %q", typeName, modulePath))
    }
    typeCache[key] = sym.Type
    return sym.Type
}
```

**Migration:** Replace all `getXxxType()` functions:

| File | Function | Replacement |
|------|----------|-------------|
| `ffi/decoders.go` | `getDecodeErrorType()` | `GetStdType("ard/decode", "Error")` |
| `ffi/fs.go` | `getFSDirEntryType()` | `GetStdType("ard/fs", "DirEntry")` |
| `ffi/http.go` | `getHTTPResponseType()` | `GetStdType("ard/http", "Response")` |

---

## Opportunity 4: Naming Convention Documentation

**Current:** Mix of prefixed and unprefixed FFI function names.

**Convention:** FFI function names follow `{Module}_{Action}` pattern where the module prefix is preserved for disambiguation. Functions in dedicated single-concept modules use unprefixed names.

| Module | Prefix | Examples |
|--------|--------|----------|
| `fs.ard` | `FS_` | `FS_Exists`, `FS_ReadFile` |
| `crypto.ard` | `Crypto` | `CryptoMd5`, `CryptoHashPassword` |
| `sql.ard` | `Sql` | `SqlCreateConnection`, `SqlQuery` |
| `http.ard` | `HTTP_` | `HTTP_Send`, `HTTP_Serve` |
| `io.ard` | (none) | `Print`, `ReadLine` |
| `argv.ard` | (none) | `OsArgs` |
| `base64.ard` | (none) | `Base64Encode`, `Base64Decode` |
| `hex.ard` | (none) | `HexEncode`, `HexDecode` |

---

## Migration Plan

### Phase 1: Helper Extraction (Low Risk)

1. Create `ffi/types.go` with `GetStdType()`
2. Update `decoders.go` to use `GetStdType("ard/decode", "Error")`
3. Update `fs.go` to use `GetStdType("ard/fs", "DirEntry")`
4. Update `http.go` to use `GetStdType("ard/http", "Response")`
5. Remove the old `getXxxType()` functions

### Phase 2: Generator Extensions (Medium Risk)

1. Add `[]any` support to `ffi_generate.go`
2. Add `map[string]any` support to `ffi_generate.go`
3. Update `checkerTypeStr()` to handle `Dynamic` type
4. Add tests for new type unwrapping/wrapping
5. Run `go generate ./bytecode/vm`

### Phase 3: Function Conversions (Low Risk after Phase 2)

1. Convert `ListToDynamic` to `func ListToDynamic(items []any) any`
2. Convert `MapToDynamic` to `func MapToDynamic(m map[string]any) any`
3. Evaluate `DynamicToList`, `DynamicToMap` for idiomatic conversion
4. Keep complex functions (`HTTP_Send`, `HTTP_Serve`, `Join`) as raw

---

## Functions That Must Remain Raw

Some functions require runtime access that idiomatic FFI cannot provide:

| Function | Reason |
|----------|--------|
| `HTTP_Send` | Builds Ard struct from response |
| `HTTP_Serve` | Registers closure as HTTP handler, needs VM isolation |
| `Join` | Iterates Fiber structs, extracts WaitGroups, calls VM |
| `JsonEncode` | Marshals generic `*runtime.Object` |
| `DecodeString/Int/Float/Bool` | Return custom error struct with embedded type lookup |
| `ExtractField` | Returns `Dynamic` with complex construction |

---

## Testing Checklist

After any FFI changes:

```bash
cd compiler
go generate ./bytecode/vm
go build
go test ./...
go run main.go run samples/basic.ard# Run formatter on std_lib after changes
go run main.go format std_lib
```