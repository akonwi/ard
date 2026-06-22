# 0030: Use Direct Go Struct Values and Fields

## Status

Proposed

## Context

ADR 0028 introduced direct Go imports so Ard code can reference Go packages, functions, methods, named types, scalar constants, enum-like typed constants, and package variables. Go named struct types can already appear in Ard signatures through direct Go type references, but their fields remain opaque to Ard.

That leaves thin companion FFI wrappers in places where the Go API is already structurally compatible with Ard. For example, `ard/http` still uses stringly pointer-backed extern types and companion helpers for simple field reads such as `http.Response.StatusCode` and `http.Request.URL.Path`:

```ard
private extern type RawRequest = "*http.Request"
private extern type RawResponse = "*http.Response"

private extern fn http_response_status(resp: RawResponse) Int = "HTTP_ResponseStatus"
private extern fn get_req_path(req: RawRequest) Str = "GetReqPath"
```

The better long-term shape is to use Go types directly where Ard is intentionally exposing or wrapping Go handles, without introducing Ard aliases just to name those handles:

```ard
use go:net/http as gohttp

struct Request {
  raw: mut gohttp::Request,
}

fn response_status(resp: mut gohttp::Response) Int {
  resp.StatusCode
}
```

Go struct interop raises more decisions than field reads alone:

- exported field access and assignment should use normal Ard field syntax;
- keyed Go struct construction should be possible from Ard;
- Go pointer nil values and panic risks need an explicit interop boundary rather than being confused with Ard `Maybe` semantics;
- embedded/promoted fields and wider pointer/reference shapes need clear boundaries so this work does not accidentally absorb the whole Go type system.

## Decision

Support direct-Go named struct values as host struct values with field metadata loaded from `go/packages`.

The compiler should discover exported, non-promoted fields for exported Go named struct types. Field names in Ard are the exported Go field names exactly, including casing. Unexported Go fields are not visible in Ard.

### Field reads

Use ordinary Ard member access syntax for direct Go struct fields:

```ard
use go:net/http as gohttp

fn status(resp: mut gohttp::Response) Int {
  resp.StatusCode
}
```

Field reads are valid on both direct Go struct values and `mut` references to direct Go struct values. A selector on a `mut gohttp::Response` lowers to Go selector syntax on the pointer value, matching ordinary Go behavior.

Nested field reads are valid when each field type is representable in Ard:

```ard
use go:image as image

fn min_x(rect: image::Rectangle) Int {
  rect.Min.X
}
```

Pointer-typed hops such as `req.URL.Path` are also valid when the field type is representable, but they preserve Go semantics. If an intermediate Go pointer is nil, the generated Go selector may panic. The checker must reject fields whose Go type cannot be represented by the current direct-Go type mapping and should report an actionable diagnostic naming the unsupported field and Go type.

### Field writes

Support field assignment as part of the direct-Go struct feature, not as a separate future feature:

```ard
use go:net/http as gohttp

fn mark_ok(resp: mut gohttp::Response) {
  resp.StatusCode = gohttp::StatusOK
}
```

A field write requires an addressable mutable target according to Ard's existing mutable-reference model. Assigning through an immutable Go value, a non-addressable temporary, or an unexported field is invalid.

Assignments should use direct-Go assignment compatibility, not exact Go type equality. For scalar fields this means generated Go should emit the same kind of conversions and range checks used at direct-Go call boundaries. For compound fields, only currently supported direct-Go value shapes are accepted.

### Struct construction

Support keyed construction of direct Go struct values using Ard's existing module-qualified struct literal syntax:

```ard
use go:image as image

let point = image::Point{X: 10, Y: 20}
let rect = image::Rectangle{
  Min: image::Point{X: 0, Y: 0},
  Max: image::Point{X: 80, Y: 24},
}
```

Construction rules:

- Only keyed struct literals are supported. Do not support Go-style positional struct literals.
- Keys must name exported, non-promoted Go fields exactly.
- Every exported, non-promoted field in the Ard-visible shape must be supplied. Direct-Go struct literals do not use Go zero values as defaults for omitted fields.
- Ard's existing omission rule only applies to Ard nullable fields. Because Go does not mark pointer, slice, map, interface, channel, or function fields as semantically optional, the compiler must not infer optional fields from Go zero or nil-able types.
- Field values are checked with the same direct-Go assignment compatibility used for field writes.
- If any exported field's Go type is unsupported by the current direct-Go type mapping, construction is rejected rather than allowing the field to be omitted.
- Unexported fields cannot be set. They are not part of the Ard-visible shape; if a Go type relies on unexported state or constructor invariants, use an exported Go constructor or companion wrapper instead of a direct struct literal.
- Embedded fields can be set only by their explicit exported field name in the initial implementation; promoted-field lookup remains deferred to ADR/follow-up work for #249.

A direct Go struct literal creates a Go value, not a pointer. If Ard needs `mut go::T`, it should follow normal mutable-reference rules: bind the value to mutable storage or pass an addressable mutable value where a `mut go::T` is required. Do not add special pointer-literal syntax as part of this decision.

### Nil semantics and direct-Go safety boundary

Do not implicitly translate Go `nil` into Ard `Maybe` for direct-Go struct fields.

ADR 0024 keeps Ard nullability explicit: `T?`/`Maybe<T>` means presence or absence, while Go pointers represent reference identity, addressability, mutation, and host interop. Direct-Go struct fields should preserve that distinction. A Go field of type `*T` has Ard type `mut go::T`, not `(mut go::T)?`.

This keeps direct-Go type mapping uniform and avoids a special checker/lowering mode where pointer fields behave differently from pointer parameters, pointer returns, package variables, or method receivers. The direct-Go feature is an interop boundary: Ard's typechecker promotes safety for Ard semantics, but it cannot prove all invariants or nil assumptions of arbitrary Go APIs. Code that opts into direct Go APIs accepts Go's nil and panic risks.

For example, this compiles and lowers to ordinary Go selectors:

```ard
fn path(req: mut gohttp::Request) Str {
  req.URL.Path
}
```

If `req` or `req.URL` is nil at runtime, the generated Go code may panic. Public Ard APIs that want safer domain semantics should adapt those Go conventions explicitly, using Ard wrappers, companion FFI, or the unsafe/recovering block described below.

Go `nil` is a host value-state rather than Ard `Maybe` absence. In some APIs it means default behavior, uninitialized storage, or a sentinel understood only by the Go package; in that sense it is often closer to Ard's no-payload `Void` idea than to semantic optionality. This ADR does not add a general `nil` literal or convert `Void` to Go nil. Direct-Go struct construction still requires every exported Ard-visible field to be supplied; if a Go pointer field must be nil and Ard has no value to express that safely, users should call an exported Go constructor or use a companion wrapper.

Nil-able Go slices, maps, interfaces, functions, and channels likewise preserve Go semantics when their types become representable. They should not be silently mapped to Ard `Maybe` unless a later ADR accepts that broader interop rule.

Provide nil checks through an explicit standard-library predicate rather than comparing against `Void`:

```ard
use ard/ffi

if ffi::is_nil(req.URL) {
  "/"
} else {
  req.URL.Path
}
```

`ffi::is_nil` should be a normal function in an `ard/ffi` standard-library module, implemented by a stdlib Go companion function such as `ffi.IsNil` rather than by a special checker intrinsic. Conceptually:

```ard
extern fn is_nil(value: $T) Bool = "IsNil"
```

The Go implementation should perform the nil test in Go, including typed nil pointers. It should not rely only on comparing a boxed `any` value to nil, because a typed nil pointer stored in an interface is not itself a nil interface:

```go
var req *http.Request = nil
var value any = req

fmt.Println(value == nil) // false: value carries dynamic type *http.Request
```

A correct implementation should check the interface itself first, then use reflection only for nil-able Go kinds:

```go
func IsNil(value any) bool {
  if value == nil {
    return true
  }
  reflected := reflect.ValueOf(value)
  switch reflected.Kind() {
  case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
    return reflected.IsNil()
  default:
    return false
  }
}
```

Calling `IsNil` on non-nil-able values such as integers, strings, and structs should return `false`, not panic.

Because `ffi::is_nil` is an ordinary generic function, it does not need a special type-system exception. It can accept any Ard value and return `false` for values whose Go representation cannot be nil. This keeps nil testing visible as an FFI operation instead of making `()` an exception to equality or assignability rules.

The predicate only tests the value passed to it. For example, `ffi::is_nil(req.URL)` can still panic first if `req` itself is nil, because Go must dereference `req` to read the `URL` field.

#### Unsafe/recovering interop blocks

Add an explicit unsafe interop block as the final phase of this work. The block can recover Go panics and expose them through Ard's `try` flow:

```ard
fn request_path(req: mut gohttp::Request) Str {
  try unsafe {
    req.URL.Path
  } -> err { "" }
}
```

`unsafe { expr }` types as `T!Str` when the block's final value has type `T`. It lowers to a small Go function/defer/recover wrapper around the block. That lets Ard code intentionally cross a trusted Go boundary while keeping panic recovery visible at the call site. The unsafe block does not change direct-Go field typing; it only marks a region where Go panics are expected interop risks and are converted into Ard error flow.

The block has important limits: recovery only catches panics in the same goroutine, it cannot roll back partial mutation, and it may also catch ordinary Ard runtime panics inside the block. The initial implementation rejects `break` inside `unsafe` blocks. The Go lowering uses an inner function with `defer`/`recover`, and Go cannot directly `break` an outer Ard loop from inside that nested function. Supporting that later would require a control-flow signal from the unsafe helper back to the outer lowered loop. Treat `unsafe` as a mitigation tool for direct Go interop, not as a guarantee that arbitrary Go APIs are safe.

### Phased implementation

Implement this ADR in phases while keeping the feature scope intact:

1. **Field metadata and reads**
   - Load exported struct fields from Go package metadata.
   - Type-check field reads on direct Go struct values and `mut` references.
   - Lower field reads to Go selectors, preserving Go pointer and nil semantics.
   - Migrate simple stdlib raw-handle fields and signatures to direct Go types where safe.

2. **Field writes**
   - Validate addressability and mutability for direct-Go field assignment.
   - Lower assignment to Go selector assignment.
   - Reuse direct-Go scalar conversion/range-check helpers for assigned values.

3. **Nil predicate helper**
   - Add an `ard/ffi` module with a normal generic `is_nil(value: $T) Bool` extern.
   - Implement the binding in stdlib Go FFI, using Go reflection where needed so typed nil pointers are detected correctly.
   - Extend stdlib FFI generation/lowering for this generic extern if needed; do not add a checker intrinsic or opaque special form.
   - Return `false` for non-nil-able values.
   - Do not add a general Go `nil` literal and do not allow `()` as a pointer assignment, constructor field value, or equality special case.

4. **Keyed struct construction**
   - Type-check module-qualified direct-Go struct literals.
   - Reject literals that omit any exported, non-promoted Ard-visible field.
   - Lower to keyed Go composite literals once all visible fields are supplied.
   - Reject fields whose Go type is not representable by current direct-Go assignment compatibility.

5. **Stdlib cleanup**
   - Prefer direct Go types in fields and signatures for raw Go handles such as HTTP request/response handles.
   - Remove companion wrappers that are only forwarding field reads or writes.
   - Keep wrappers that adapt nil, headers/maps/slices, body lifecycles, interfaces, callbacks, or other unsupported Go shapes.

6. **Unsafe/recovering interop blocks**
   - Add parser, checker, AIR, and Go lowering support for `unsafe { ... }` as a panic-recovering interop expression.
   - Type `unsafe { expr }` as `T!Str` when `expr` has type `T`.
   - Lower through Go `defer`/`recover` so `try unsafe { ... }` can convert panics into ordinary Ard error flow.
   - Keep `break` out of unsafe blocks until explicit rules are accepted.

## Consequences

- Ard users can work with ordinary Go structs without writing companion functions for every field.
- Standard-library code can expose and store Go handles directly instead of using stringly `extern type` declarations where the handle is a Go named type or pointer to one.
- Field writes and construction become part of the same direct-Go struct story, so users can both inspect and build common Go configuration structs.
- Direct-Go struct construction is stricter than Go keyed literals: omitted exported fields are checker errors rather than implicit zero values.
- Go pointer fields do not receive special `Maybe` treatment, keeping checker and lowering complexity lower and preserving a uniform direct-Go mapping.
- Direct-Go field access inherits Go nil and panic behavior. That is an intentional interop risk users accept when using Go APIs directly.
- Ard's safety promise remains focused on Ard code and Ard semantics; arbitrary Go package invariants are outside what the Ard checker can prove.
- Nil-sensitive public Ard APIs should keep explicit adapters when they need domain-safe behavior.
- `ffi::is_nil(value)` provides a small nil-test surface without introducing a general nil literal or special `Void` comparison rule.
- `unsafe { ... }` provides an escape hatch and mitigation path by making panic recovery explicit in Ard control flow.
- `ffi::is_nil` can cover broader nil-able Go values as those representations become available through direct Go interop.
- Embedded/promoted fields are not part of the initial lookup model; #249 remains the place to expand that behavior.
- Interfaces, callback bridges, wider pointer shapes, variadics, and compound conversion gaps still constrain how far stdlib cleanup can go.

## Related

- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0024-preserve-maybe-semantics-in-go-lowering.md`
- `docs/adrs/0028-use-direct-go-imports-for-ffi.md`
- `docs/adrs/0029-remove-javascript-targets.md`
- `docs/standard-library-ffi-audit.md`
- GitHub issue #240
- GitHub issue #247
- GitHub issue #249
