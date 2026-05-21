# Foreign Function Interface (FFI)

Ard's extern/FFI system lets functions declared in Ard call target-specific implementations.

Today that includes:

- Go implementations used by the Go target and standard library
- JavaScript companion modules for `js-server` and `js-browser`

The system is intentionally narrow:

- it is primarily for Ard's built-in standard library plus project-local companion modules
- Go project companions are compiled into the generated Go application
- unsupported Go companion layouts or signatures are rejected early

## Target-aware extern bindings

Extern functions can use either the single-string shorthand or the binding-block form.

```ard
extern fn read_line() Str!Str = "ReadLine"
```

The shorthand is Go-oriented and resolves to the same binding as `go = "ReadLine"`.

```ard
extern fn read_line() Str!Str = {
  go = "ReadLine"
  js = "readLine"
  js-browser = "readLineBrowser"
}
```

Supported binding keys:

- `go`
- `js`
- `js-server`
- `js-browser`

The checker resolves the active extern binding using this precedence:

1. exact target binding, if present
2. shared `js` binding for `js-server` and `js-browser`
3. `go`

## Go project companions

Project code can provide Go implementations for project-local externs when building or running with the Go target. The compiler copies Go companion files into the generated Go workspace and calls the binding directly.

Supported companion locations:

- `ffi.go` at the project root
- `ffi/*.go` under the project root

Companion files must use `package ffi`. The Go target copies them into a generated `projectffi` package and imports that package from generated Ard code.

Projects may provide either a single root `ffi.go` file or a multi-file `ffi/*.go` package directory. Using both forms at once is rejected to keep project FFI package layout unambiguous.

Example Ard declaration:

```ard
extern fn hostname() Str!Str = {
  go = "Hostname"
}
```

Example `ffi.go`:

```go
package ffi

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

The compiler ships standard library companion files at:

- `compiler/std_lib/ffi.js-server.mjs`
- `compiler/std_lib/ffi.js-browser.mjs`

Project companion modules can be supplied beside Ard source as target-specific JavaScript modules. On JavaScript targets, generated JS imports the companion module and calls the exported binding.

## Standard library Go FFI generation

Standard library Go lowering metadata is generated from the Ard standard library declarations and Go implementations:

```bash
cd compiler
go generate ./std_lib/ffi
```

The generated metadata lives in `compiler/go/stdlib_ffi.gen.go` and lets the Go target route standard library extern calls consistently.

## Design guidelines

- Prefer explicit binding blocks for multi-target externs.
- Keep Go companion functions small and idiomatic.
- Do not depend on checker internals from companion code.
- Keep target-specific behavior in target companion files, not in Ard declarations.
