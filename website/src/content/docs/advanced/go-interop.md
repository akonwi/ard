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
use ard/maybe
use go:net/http as gohttp

let missing: (mut gohttp::Request)? = maybe::none()
```

Use `(mut T)?` when an Ard API intentionally models an optional reference. Direct-Go pointer fields are not automatically wrapped in `Maybe`; Go pointer values remain `mut go::T` and preserve Go nil behavior.

## Checking for Nil

Use `ard/ffi::is_nil` when you need to test a Go value for nil without adding a new Ard `nil` literal.

```ard
use ard/ffi
use go:net/http as gohttp

fn request_path(req: mut gohttp::Request) Str {
  match ffi::is_nil(req.URL) {
    true => "",
    false => req.URL.Path,
  }
}
```

`ffi::is_nil` is a normal generic stdlib function implemented by Go FFI. It returns `false` for values whose Go representation cannot be nil.

The argument expression is evaluated before `is_nil` runs. For example, `ffi::is_nil(req.URL)` can still panic first if `req` itself is nil.

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
- callbacks, variadics, and many compound Go shapes still need companion wrappers;
- generic Go struct construction is not supported yet.
