# Foreign Function Interface (FFI)

## Overview

Ard's FFI lets standard library functions declared in Ard call Go implementations inside `compiler/ffi/`.

The system is intentionally narrow:
- it is for Ard's built-in standard library, not user-installed native extensions
- it uses **zero reflection** at runtime
- registration is fully code-generated with `go generate`
- unsupported exported signatures in `ffi/*.go` are treated as **generator errors**

## Architecture

### Directory structure

```text
compiler/
├── ffi/                    # Go implementations of extern bindings
├── std_lib/                # Ard declarations using extern fn ... = "BindingName"
├── runtime/                # Ard runtime object model and VM-facing helpers
└── bytecode/vm/
    ├── ffi_registry.go     # Runtime registry + panic recovery
    ├── ffi_generate.go     # AST-based FFI discovery + wrapper generation
    └── registry.gen.go     # Generated registrations and idiomatic wrappers
```

### Two-tier FFI model

FFI functions can be written in one of two styles.

#### 1. Raw FFI

Raw functions work directly with Ard runtime objects.

```go
func HTTP_Send(args []*runtime.Object) *runtime.Object
```

Use raw FFI when the function needs:
- `Dynamic` values
- Ard structs/enums/results built manually
- closures or VM/runtime internals
- embedded Ard type lookups from `checker`
- custom marshalling logic

#### 2. Idiomatic Go FFI

Idiomatic functions use ordinary Go types, and the generator creates a raw wrapper automatically.

```go
func CryptoMd5(input string) string
func EnvGet(key string) *string
func FS_ReadFile(path string) (string, error)
func OsArgs() []string
```

The generated wrapper in `registry.gen.go`:
- unwraps Ard arguments from `[]*runtime.Object`
- calls the Go function directly
- wraps the Go result back into Ard runtime objects

## Supported idiomatic types

### Parameters

The generator currently supports these Go parameter types:
- `string`
- `int`
- `float64`
- `bool`
- `any` (or `interface{}`)
- `*string`
- `*int`
- `*float64`
- `*bool`
- `[]string`
- `[]int`
- `[]float64`
- `[]bool`
- `map[string]string`

Meaning:
- scalar types map to Ard scalar values
- `any` maps to Ard `Dynamic` or `extern type` (opaque handles)
- pointer-to-scalar types map to Ard `Maybe<T>`
- slice-of-scalar types map to Ard `[T]`
- `map[string]string` maps to Ard `[Str:Str]`

### Returns

The generator supports these return shapes:
- no return value
- `T`
- `*T`
- `[]T`
- `error`
- `(T, error)`

Where `T` is one of:
- `string`
- `int`
- `float64`
- `bool`
- `any`
- `*string`
- `*int`
- `*float64`
- `*bool`
- `[]string`
- `[]int`
- `[]float64`
- `[]bool`
- `map[string]string`

### Ard mapping rules

| Go type | Ard type |
|---|---|
| `string` | `Str` |
| `int` | `Int` |
| `float64` | `Float` |
| `bool` | `Bool` |
| `any` | `Dynamic` or `extern type` |
| `*string` | `Str?` |
| `*int` | `Int?` |
| `*float64` | `Float?` |
| `*bool` | `Bool?` |
| `[]string` | `[Str]` |
| `[]int` | `[Int]` |
| `[]float64` | `[Float]` |
| `[]bool` | `[Bool]` |
| `error` | `Void!Str` |
| `(T, error)` | `T!Str` |

Notes:
- `any` params are unwrapped via `.Raw()`, returns are wrapped via `runtime.MakeDynamic()`
- `any` is the Go-side representation for Ard `extern type` values (opaque FFI handles)
- `nil` pointer return becomes `None`
- non-`nil` pointer return becomes `Some(value)`
- an `error` return is wrapped as `Err(Str)`
- successful `(T, error)` returns are wrapped as `Ok(T)`

## Generator behavior

`go generate ./bytecode/vm` scans every exported, non-underscore-prefixed function in `compiler/ffi/*.go`.

Each function must be exactly one of:
1. a raw FFI function: `func([]*runtime.Object) *runtime.Object`
2. a supported idiomatic Go FFI function

Anything else is a generation error.

That means functions in `ffi/` cannot silently disappear from the registry.

### Example raw registration

```go
if err := r.Register("HTTP_Send", ffi.HTTP_Send); err != nil {
    return fmt.Errorf("failed to register HTTP_Send: %w", err)
}
```

### Example generated idiomatic wrapper

Source function:

```go
func FS_ReadFile(path string) (string, error)
```

Generated wrapper:

```go
func _ffi_FS_ReadFile(args []*runtime.Object) *runtime.Object {
    arg0 := args[0].AsString()
    result, err := ffi.FS_ReadFile(arg0)
    if err != nil {
        return runtime.MakeErr(runtime.MakeStr(err.Error()))
    }
    return runtime.MakeOk(runtime.MakeStr(result))
}
```

## Extern types (opaque handles)

Ard supports `extern type` declarations for opaque FFI handles — values that Ard code
can hold and pass around but never inspect or construct:

```ard
private extern type ConnectionPtr

private extern fn connect(cs: Str) ConnectionPtr!Str = "SqlCreateConnection"
private extern fn close(db: ConnectionPtr) Void!Str = "SqlClose"

struct Database {
  _ptr: ConnectionPtr,
  path: Str,
}
```

On the Go side, extern types map to `any`. The Go function receives/returns the raw
Go value (e.g., a `*sqlConnection` pointer) and the runtime wraps it as a Dynamic object:

```go
func SqlCreateConnection(connectionString string) (any, error) {
    conn := &sqlConnection{db: db, driver: driver}
    return conn, nil
}

func SqlClose(handle any) error {
    conn := handle.(*sqlConnection)
    return conn.db.Close()
}
```

### Properties

- **Type-safe**: `ConnectionPtr` ≠ `TransactionPtr` — the checker catches misuse
- **No construction**: Ard code cannot create extern type values; only FFI can
- **No equality**: `==` on extern types is a checker error
- **No pattern matching**: extern types cannot be matched
- **Coercion to Dynamic**: extern types can be passed where `Dynamic` is expected
- **Runtime representation**: identical to Dynamic (wraps Go `any`), zero overhead

## Standard library integration

Ard code binds extern functions by string name:

```ard
extern fn read(path: Str) Str!Str = "FS_ReadFile"
extern fn get(key: Str) Str? = "EnvGet"
extern fn os_args() [Str] = "OsArgs"
```

The checker validates the Ard side. The generator validates the Go side.

## Error handling

### Explicit errors

Idiomatic functions should use normal Go `error` returns when possible:

```go
func ReadLine() (string, error)
func FS_WriteFile(path, content string) error
```

### Panic recovery

`RuntimeFFIRegistry.Call()` still wraps panics.

Behavior:
- if the Ard return type is a `Result`, a panic becomes `Err("panic in FFI function ...")`
- otherwise the panic is re-thrown with FFI context

This applies to both raw functions and generated idiomatic wrappers.

## Raw FFI examples

Use raw FFI when you need full control over Ard values:

```go
func FS_ListDir(args []*runtime.Object) *runtime.Object {
    path := args[0].Raw().(string)
    entries, err := os.ReadDir(path)
    if err != nil {
        return runtime.MakeErr(runtime.MakeStr(err.Error()))
    }

    dirEntryType := getFSDirEntryType()

    var dirEntries []*runtime.Object
    for _, entry := range entries {
        dirEntries = append(dirEntries, runtime.MakeStruct(dirEntryType, map[string]*runtime.Object{
            "name":    runtime.MakeStr(entry.Name()),
            "is_file": runtime.MakeBool(!entry.IsDir()),
        }))
    }

    return runtime.MakeOk(runtime.MakeList(dirEntryType, dirEntries...))
}
```

This stays raw because it needs embedded Ard type lookup and manual struct construction.

## Development workflow

### Adding a new FFI binding

1. Add a Go function in `compiler/ffi/`
2. Choose either raw or idiomatic style
3. Run:

```bash
cd compiler
go generate ./bytecode/vm
```

4. Add or update the Ard declaration in `compiler/std_lib/*.ard`
5. Validate with:

```bash
go build
go test ./...
```

### When to choose idiomatic vs raw

Prefer **idiomatic** when the binding uses:
- scalar types, lists, maps, maybe, results
- opaque handles (use `any` on the Go side, `extern type` on the Ard side)

Prefer **raw** when the binding needs:
- runtime closures
- Ard structs/enums constructed manually
- embedded Ard type lookups from the checker
- special VM/runtime behavior

## Current status

62 FFI functions total: 16 raw, 46 idiomatic.

### Remaining raw functions

| Function(s) | Why raw | Possible future migration |
|---|---|---|
| `HTTP_Send`, `HTTP_Serve` | Complex multi-type with closures/structs | Unlikely without major generator changes |
| `SqlQuery`, `SqlExecute` | Mixed `[Value]` list with `sqlArgValue` unwrapping | `[]any` param support would help |
| `DecodeString/Int/Float/Bool` | Return custom error struct from embedded module lookup | Needs struct return support |
| `DynamicToList`, `DynamicToMap`, `ExtractField` | Return Dynamic collections + Object for error formatting | `[]any` return support would help |
| `MapToDynamic` | Needs `map[string]any` support | `map[string]any` param type |
| `FS_ListDir` | Embedded module struct construction | Needs struct return support |
| `Join` | Iterates Fiber struct list, extracts WaitGroups | Needs struct field access |
| `JsonEncode` | Generic `$T` input needs full Object for marshaling | Unlikely |

### Potential future type extensions

Adding `[]any` support would enable migrating `SqlQuery`, `SqlExecute`, `ListToDynamic`,
`DynamicToList`, and similar functions. `[]any` params would iterate `.AsList()` and call
`.Raw()` on each element; `[]any` returns would wrap each element with `MakeDynamic` and
build a list with `MakeList(checker.Dynamic, ...)`.

Adding `map[string]any` would enable `MapToDynamic`.

## Summary

Ard's FFI provides:
- zero-reflection dispatch
- generated registration and marshalling
- `extern type` for type-safe opaque handles
- `any` support for Go-side handle functions
- hard errors for unsupported exported signatures
- an escape hatch for complex runtime-aware bindings

See [ffi-refactoring.md](./ffi-refactoring.md) for identified improvement opportunities.
