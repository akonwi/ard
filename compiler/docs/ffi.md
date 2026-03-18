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

Prefer **idiomatic** when the binding is mostly scalar/list/maybe/result marshalling.

Prefer **raw** when the binding needs:
- `Dynamic`
- runtime closures
- Ard structs/enums/maps beyond simple scalar collections
- manual construction of Ard-specific types
- special VM/runtime behavior

## Current status

The FFI system is incremental by design.

Today the codebase uses both tiers:
- many simple crypto, fs, runtime, prelude, and SQL helper bindings are idiomatic Go
- HTTP, SQL execution, decode/dynamic conversion, and other complex bindings remain raw

This keeps the common cases ergonomic without weakening the low-level escape hatch.

## Summary

Ard's FFI now provides:
- zero-reflection dispatch
- generated registration
- generated marshalling for common Go types
- hard errors for unsupported exported signatures
- an escape hatch for complex runtime-aware bindings

That gives standard library authors a much more idiomatic Go authoring model while preserving Ard's existing runtime representation and safety checks.
