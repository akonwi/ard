# Foreign Function Interface (FFI)

## Overview

Ard's extern/FFI system lets functions declared in Ard call target-specific implementations.

Today that includes:
- Go implementations inside `compiler/ffi/` for the bytecode VM and Go-oriented runtime paths
- JavaScript companion modules for `js-server` and `js-browser`

The system is intentionally narrow:
- it is primarily for Ard's built-in standard library plus project-local JS companion modules
- it uses **zero reflection** at runtime on the Go side
- Go registration is fully code-generated with `go generate`
- unsupported exported signatures in `ffi/*.go` are treated as **generator errors**

## Target-aware extern bindings

Extern functions can use either the original single-string shorthand or the newer binding-block form.

### Single-string shorthand

```ard
extern fn read_line() Str!Str = "ReadLine"
```

This remains supported and is effectively Go-oriented shorthand.

### Binding blocks

```ard
extern fn read_line() Str!Str = {
  go = "ReadLine"
  js = "readLine"
  js-browser = "readLineBrowser"
}
```

Binding blocks let one Ard declaration resolve differently per target.

### Supported binding keys

Current binding keys include:
- `go`
- `bytecode`
- `js`
- `js-server`
- `js-browser`

### Resolution precedence

The checker resolves the active extern binding using this precedence:

1. exact target binding, if present
2. shared `js` binding for `js-server` and `js-browser`
3. `go`
4. `bytecode` for `go`, `bytecode`, or target-unspecified checking

Examples:
- `js-server` prefers `js-server`, then `js`, then `go`
- `js-browser` prefers `js-browser`, then `js`, then `go`
- `go` prefers `go`, then `bytecode`

## Go project companions

Project code can provide Go implementations for project-local externs when building with `--target go`.
The compiler copies Go companion files into the generated Go workspace and calls the binding directly.

Supported companion locations:
- `ffi.go` at the project root
- `ffi/*.go` under the project root

Companion files must use `package main` because they are compiled into the generated application package.

Example Ard declaration:

```ard
extern fn hostname() Str!Str = {
  go = "Hostname"
}
```

Example `ffi.go`:

```go
package main

import "os"

func Hostname() (string, error) {
    return os.Hostname()
}
```

Project Go FFI currently uses idiomatic direct-call adaptation:
- scalar/list/map/function arguments pass as their generated Go values
- `T?` arguments pass as `*T` (`nil` for `None`)
- `T` returns directly as `T`
- `T?` expects `*T` (`nil` becomes `None`, non-`nil` becomes `Some`)
- `Void!Str` expects `error`
- `T!Str` expects `(T, error)`

## JavaScript companion modules

JavaScript externs are implemented through companion `.mjs` files rather than per-function module paths embedded in Ard source.

### Standard library companions

The compiler ships standard library companion files at:
- `compiler/std_lib/ffi.js-server.mjs`
- `compiler/std_lib/ffi.js-browser.mjs`

These modules export the JS implementations for stdlib extern bindings on each JS target.

### Project companions

Projects can also provide target-specific JS companions at the project root:
- `ffi.js-server.mjs`
- `ffi.js-browser.mjs`

When a build uses project JS externs, the compiler copies these into the build output as:
- `ffi.project.js-server.mjs`
- `ffi.project.js-browser.mjs`

Standard library companions are copied into output as:
- `ffi.stdlib.js-server.mjs`
- `ffi.stdlib.js-browser.mjs`

Generated JS then imports these as normal ESM namespaces.

### Example

Ard declaration:

```ard
extern fn read_line() Str!Str = {
  go = "ReadLine"
  js-server = "readLine"
}
```

JS companion export:

```js
export function readLine() {
  // ...
}
```

On `js-server`, generated JS imports the companion module and calls `readLine(...)` from it.

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

For the Go target, project extern types can optionally bind to concrete Go type
expressions. This keeps the Ard type opaque while allowing project FFI functions
to use precise Go signatures instead of `any`:

```ard
extern type Vaxis = "*vaxis.Vaxis"

extern fn tui_open() Vaxis!Str = "TuiOpen"
extern fn tui_close(term: Vaxis) Void!Str = "TuiClose"
```

```go
package main

import "git.sr.ht/~rockorager/vaxis"

func TuiOpen() (*vaxis.Vaxis, error) {
    return vaxis.New(vaxis.Options{})
}

func TuiClose(vx *vaxis.Vaxis) error {
    vx.Close()
    return nil
}
```

The binding may also use target-specific binding-block syntax when needed:

```ard
extern type Vaxis = {
  go = "*vaxis.Vaxis"
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

Ard code binds extern functions either by string shorthand or by target-aware binding block.

### Shorthand example

```ard
extern fn read(path: Str) Str!Str = "FS_ReadFile"
extern fn get(key: Str) Str? = "EnvGet"
extern fn os_args() [Str] = "OsArgs"
```

### Target-aware example

```ard
private extern fn _print(string: Str) Void = {
  go = "Print"
  js-server = "printLine"
}

extern fn read_line() Str!Str = {
  go = "ReadLine"
  js-server = "readLine"
}
```

The checker validates the Ard side and resolves the active binding for the current target.

On the Go/bytecode side, the generator validates supported Go FFI signatures.
On JavaScript targets, the backend imports the relevant JS companion module and calls the exported binding.

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

### Remaining raw functions

| Function(s) | Why raw | Possible future migration |
|---|---|---|
| `HTTP_Serve` | Needs runtime closure invocation, Ard `Request`/`Response` struct construction, and embedded checker type lookup | Unlikely without major generator/runtime changes |
| `DecodeString/Int/Float/Bool` | Returns `ard/decode::Error` structs built from embedded module types | Needs struct-return support plus embedded type lookup |
| `DynamicToList`, `DynamicToMap`, `ExtractField` | Need direct `runtime.Object` access for Dynamic coercion and richer raw-value error formatting | Would need more object-aware generated helpers |
| `Join` | Reaches into Fiber structs to extract and wait on opaque `WaitGroup` handles | Needs struct field access in generated wrappers |

### Current generator gaps

The generator already supports `[]any` and `map[string]any`, which is why bindings like
`SqlQuery`, `SqlExecute`, `ListToDynamic`, `MapToDynamic`, `FS_ListDir`, and `JsonEncode`
now use the idiomatic path.

The main remaining gaps are:
- Ard struct/enum construction on return
- embedded module type lookup during marshalling
- generated access to fields inside Ard runtime structs
- closure-aware bindings that need VM/runtime participation

## Summary

Ard's FFI provides:
- zero-reflection dispatch
- generated registration and marshalling
- `extern type` for type-safe opaque handles
- `any` support for Go-side handle functions
- hard errors for unsupported exported signatures
- an escape hatch for complex runtime-aware binding
