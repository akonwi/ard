---
title: Direct Go Interop
description: Use Go packages directly from Ard, including Go structs, fields, nil checks, and unsafe interop blocks.
---

Ard's active backend is Go, so Ard code can import Go packages directly for APIs that map cleanly to Ard values. Use direct Go interop for thin bindings and keep companion FFI wrappers when you need domain-specific adaptation.

:::caution
Direct Go interop preserves Go semantics. Ard's typechecker still checks Ard types, but it cannot prove every invariant or nil assumption made by arbitrary Go packages.
:::

## Importing Go Packages

Use `use go:` to import a Go package. The package is available as a namespace, not as an Ard module.

```ard
use go:image as image
use go:net/http as gohttp
```

A project that imports Go packages relies on the project's `go.mod`. Add Go dependencies with ordinary Go tooling, such as `go get`, before building the Ard project.

## Project FFI Bindings

When a Go API needs adaptation before it is pleasant or safe to use from Ard, put a small Go package under your project's `ffi/` directory and import that package with `use go:`. The import path uses your Go module path, or the Ard project name when the compiler generates a minimal Go module.

```
my_app/
├── ard.toml
├── go.mod
├── main.ard
└── ffi/
    └── env.go
```

```go
// ffi/env.go
package ffi

import (
	"fmt"
	"os"
)

func RequiredEnv(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("missing environment variable: %s", name)
	}
	return value, nil
}
```

```ard
use go:my_app/ffi

fn home_dir() Str!Str {
  ffi::RequiredEnv("HOME")
}
```

Project-local Go packages exposed to Ard should live under `ffi/`. Keep these packages small and use them to translate Go-specific shapes into Ard-facing APIs when direct imports are not enough.

## Direct Go Types

Exported Go named types can appear directly in Ard signatures and fields.

```ard
use go:net/http as gohttp

struct RequestBox {
  raw: mut gohttp::Request,
}

fn status(resp: mut gohttp::Response) Int {
  resp.StatusCode
}
```

Raw Go pointer syntax does not appear in Ard source. Use Ard's mutable-reference syntax instead:

- `gohttp::Response` lowers to `http.Response`
- `mut gohttp::Response` lowers to `*http.Response`

## Go Arrays, Slices, and Maps

Go slices map to Ard lists (`[T]`), Go maps map to Ard maps (`[K:V]`), and Go fixed-size arrays map to Ard fixed-size arrays (`[T; N]`). The length is part of a fixed array's type, just like in Go.

```ard
use go:crypto/sha256

mut bytes = "hello".bytes()
let digest: [Byte; 32] = sha256::Sum256(bytes)
let zero: Byte = 0
let first = digest.at(0).or(zero)
```

Ard does not implicitly convert through containers. If a Go API needs `[Byte]` and you have `[Int]`, write the transformation explicitly with `Byte::from(...)` so allocation and truncation are visible in source.

## Numeric Conversions

`T::from(value)` converts a numeric value into a bare sized scalar (`Int64`,
`Uint32`, `Float32`, …) or a foreign named scalar type (a Go named type whose
underlying type is numeric, like `time::Duration`). It is a truncating
conversion, mirroring Go's `T(x)`, and returns `T` — not an optional — so it
composes with arithmetic:

```ard
use go:time

fn every(ms: Int) time::Duration {
  time::Duration::from(ms) * time::Millisecond
}

let page: Uint32 = Uint32::from(count)
```

Runtime values are truncated at the boundary exactly like Go. A numeric
**literal**, however, is range-checked against the target, so a constant that
cannot fit is a compile error (again matching Go's constant conversion):

```ard
Uint8::from(200) // ok
Uint8::from(300) // error: Integer literal 300 overflows Uint8
```

Numeric literals already adopt a foreign scalar type directly in arithmetic and
annotated bindings (`let d: time::Duration = 5 * time::Millisecond`); `from` is
for converting **runtime** values.

## Variadic Calls

Direct Go variadic functions and methods can be called with repeated trailing arguments. Ard expands those arguments at the Go call site.

```ard
use go:fmt

fmt::Println()
fmt::Println("hello")
fmt::Println("hello", 42, true)
```

Ard does not currently have spread syntax (`args...`) or Ard-native variadic function declarations. Forwarding an existing list to a Go variadic parameter still needs a Go helper or an explicit wrapper.

## Go Interfaces

Named Go interfaces can be used as direct Go types. Ard checks assignability with Go's interface rules for direct-Go values, similar to Ard trait compatibility, while generated code keeps native Go interface values.

```ard
use go:io
use go:strings

fn read_all(reader: io::Reader) [Byte]!Str {
  io::ReadAll(reader)
}

let bytes = read_all(strings::NewReader("hello")).expect("read")
```

Interface-to-interface assignability also follows Go's rules, so a value such as `io::ReadCloser` can be used where `io::Reader` or `io::Closer` is expected when the required methods match. Go slices and maps remain invariant: `[mut strings::Reader]` is not automatically converted to `[]io.Reader`.

Ard-defined structs can satisfy Go interfaces when their `impl` methods have Go-compatible method names and signatures. The Go backend emits receiver methods for those impls, including methods that are only needed by Go interface dispatch. Functions and closure adapters still need companion FFI wrappers.

### Go errors

Go's predeclared `error` interface maps to builtin `Error` in parameters, fields, and other value positions:

```ard
struct AppError {
  message: Str,
}

impl Error for AppError {
  fn error() Str {
    self.message
  }
}

let error: Error = AppError{message: "request failed"}
```

`Error::new("message")` creates a simple Go-compatible error. An explicit `impl Error` emits Go's `Error() string` method, so the Ard struct can be passed directly to Go APIs accepting `error`.

For compatibility, conventional Go error returns retain their existing mapping: Go `error` becomes `Void!Str`, and `(T, error)` becomes `T!Str`. Ard functions returning `Void!Error` or `T!Error` also use Go's idiomatic error return ABI, while preserving the underlying error value.

## Struct Field Reads

Exported Go struct fields use ordinary dot syntax. Field names match Go exactly, including casing.

```ard
use go:image as image

fn min_x(rect: image::Rectangle) Int {
  rect.Min.X
}
```

Pointer-typed hops preserve Go behavior. If an intermediate pointer is nil, the generated Go selector may panic.

```ard
use go:net/http as gohttp

fn request_path(req: mut gohttp::Request) Str {
  req.URL.Path
}
```

## Struct Field Writes

Assignments to exported Go fields also use ordinary field syntax. The target must be mutable or a mutable Go pointer.

```ard
use go:net/http as gohttp

fn mark_ok(resp: mut gohttp::Response) {
  resp.StatusCode = gohttp::StatusOK
}
```

Scalar values use the same direct-Go conversion and range-check rules used at Go call boundaries.

## Struct Construction

Construct direct Go structs with keyed literals.

```ard
use go:image as image

let point = image::Point{X: 10, Y: 20}
let rect = image::Rectangle{
  Min: image::Point{X: 0, Y: 0},
  Max: image::Point{X: 80, Y: 24},
}
```

Rules:

- all exported Ard-visible fields must be provided;
- omitted fields do not default to Go zero values;
- field names must match exported Go field names exactly;
- unexported fields cannot be set;
- embedded/promoted field lookup is not supported yet;
- unsupported field types reject the literal.

Prefer Go constructors or companion wrappers for structs whose zero value is unsafe or whose invariants live in unexported fields.

## Optional References

Nullable mutable references are written with grouping:

```ard
use go:net/http as gohttp

let missing: (mut gohttp::Request)? = Maybe::new()
```

Use `(mut T)?` when an Ard API intentionally models an optional reference. Direct-Go pointer fields are not automatically wrapped in `Maybe`; Go pointer values remain `mut go::T` and preserve Go nil behavior.

## Checking for Nil

Use `ard/unsafe::is_nil` when you need to test a Go value for nil without adding a new Ard `nil` literal.

```ard
use ard/unsafe
use go:net/http as gohttp

fn request_path(req: mut gohttp::Request) Str {
  match unsafe::is_nil(req.URL) {
    true => "",
    false => req.URL.Path,
  }
}
```

`unsafe::is_nil` is a compiler-backed stdlib intrinsic. It returns `false` for values whose Go representation cannot be nil.

The argument expression is evaluated before `is_nil` runs. For example, `unsafe::is_nil(req.URL)` can still panic first if `req` itself is nil.

## Unsafe Interop Blocks

Use `unsafe { ... }` as an explicit escape hatch around direct Go operations that may panic. If the block's final value has type `T`, the unsafe block has type `T!Str`.

```ard
use go:net/http as gohttp

fn request_path_or_default(req: mut gohttp::Request) Str {
  try unsafe {
    req.URL.Path
  } -> _ {
    ""
  }
}
```

`unsafe` recovers panics in the same goroutine and converts them to `Str` errors. It does not undo partial mutation, and `break` is currently rejected inside unsafe blocks.

## Current Limits

Direct Go interop is intentionally incremental. Current limitations include:

- embedded/promoted Go fields are not resolved through promotion;
- Ard functions and closures cannot implement Go callback-shaped interfaces directly yet;
- spread/forwarding for Go variadics still needs companion wrappers;
- generic Go struct construction is not supported yet.
