# 0038: Use Idiomatic Go ABI for Result and Maybe Returns

## Status

Proposed

## Context

Ard has first-class `Result` and `Maybe` semantics:

- `T!E` represents a fallible value.
- `T?` represents explicit absence.

Those types can be passed as values, stored in variables, used in fields, placed in containers, and returned from functions. The Go backend currently needs runtime shapes for those first-class value positions:

- `runtime.Result[T, E]`
- `runtime.Maybe[T]`

However, Go has established ABI conventions for function and method returns:

- fallible functions return `(T, error)` or `error`;
- comma-ok functions return `(T, bool)`.

Direct Go interop already adapts these Go return shapes into Ard `Result` and `Maybe` at call boundaries. The reverse direction is equally important: generated Ard functions and methods should be able to present idiomatic Go signatures where the Ard return type naturally corresponds to those Go conventions.

This matters especially for Go interface satisfaction. For example, Go's `io.Writer` requires:

```go
type Writer interface {
    Write([]byte) (int, error)
}
```

An Ard method with the natural Ard signature:

```ard
fn write(mut self, bytes: [Byte]) Int!Str
```

should be able to lower to a Go method that satisfies `io.Writer` directly, rather than returning `runtime.Result[int, string]` and requiring a generated wrapper.

## Decision

Use distinct Go representations for `Result` and `Maybe` depending on position:

- **ABI return position**: function and method returns use idiomatic Go result shapes.
- **Value position**: parameters, locals, fields, containers, `Any` boxing, and other first-class values use runtime shapes.

### Result

For function and method return position:

| Ard return type | Go ABI return shape |
| --- | --- |
| `Void!Str` | `error` |
| `T!Str` | `(T, error)` |

For value position:

| Ard type | Go value representation |
| --- | --- |
| `Void!Str` | `runtime.Result[struct{}, string]` |
| `T!Str` | `runtime.Result[T, string]` |
| `T!E` | `runtime.Result[T, E]` |

Only `Str` errors receive the idiomatic Go `error` ABI in this decision. Non-`Str` result errors remain runtime values unless a later decision defines an error-interface mapping.

### Maybe

For function and method return position:

| Ard return type | Go ABI return shape |
| --- | --- |
| `Void?` | `bool` |
| `T?` | `(T, bool)` |

For value position:

| Ard type | Go value representation |
| --- | --- |
| `Void?` | `runtime.Maybe[struct{}]` |
| `T?` | `runtime.Maybe[T]` |

The boolean return follows Go's comma-ok convention: `true` means a value is present and `false` means absence.

### Void

This follows the same position-sensitive principle already used by `Void`:

| Ard type | Go representation |
| --- | --- |
| `Void` return | no Go result |
| `Void` value | `struct{}` |

### Boundary packing and unpacking

The backend inserts packing and unpacking code when a value crosses between ABI return representation and first-class value representation.

A call returning `T!Str` used as a value is packed:

```ard
let result: Int!Str = parse("1")
```

lowers conceptually to:

```go
value, err := Parse("1")
result := runtime.Result[int, string]{Value: value, Err: errorString(err), Ok: err == nil}
```

A call returning `T?` used as a value is packed:

```ard
let user: User? = find_user(id)
```

lowers conceptually to:

```go
value, ok := FindUser(id)
user := runtime.Maybe[User]{Value: value, Ok: ok}
```

When a function returning `T!Str` delegates to another function returning `T!Str`, the backend may return the ABI tuple directly:

```ard
fn outer() Int!Str {
    parse("1")
}
```

lowers conceptually to:

```go
func Outer() (int, error) {
    return Parse("1")
}
```

Similarly, a function returning `T?` may directly return another comma-ok call.

### Constructing return values

When lowering into ABI return position:

- `ok(value)` for `T!Str` returns `value, nil`.
- `err(message)` for `T!Str` returns the zero value of `T` and `errors.New(message)` or an equivalent string-to-error helper.
- `ok(())` for `Void!Str` returns `nil`.
- `err(message)` for `Void!Str` returns `errors.New(message)` or equivalent.
- `some(value)` for `T?` returns `value, true`.
- `none()` for `T?` returns the zero value of `T` and `false`.
- `some(())` for `Void?` returns `true`.
- `none()` for `Void?` returns `false`.

When lowering into value position, the backend still constructs `runtime.Result` and `runtime.Maybe` values.

### Parameters remain value position

Function and method parameters use value representation, not ABI tuples:

```ard
fn inspect(result: Int!Str, maybe_user: User?) Str
```

lowers conceptually to:

```go
func Inspect(result runtime.Result[int, string], maybeUser runtime.Maybe[User]) string
```

This keeps `Result` and `Maybe` first-class and avoids tuple-shaped parameter lists that would be ambiguous and hard to compose.

### Go interface satisfaction

Because Ard methods returning `T!Str` and `T?` lower to idiomatic Go method returns, Ard-defined types can satisfy many Go interfaces directly.

For example:

```ard
struct Sink {}

impl Sink {
    fn write(mut self, bytes: [Byte]) Int!Str {
        bytes.size()
    }
}
```

can lower to:

```go
func (s *Sink) Write(bytes []byte) (int, error) {
    return len(bytes), nil
}
```

and therefore satisfy `io.Writer` without an extra adapter wrapper.

This does not make every Ard method automatically satisfy every Go interface. The generated method set must still match Go's interface rules after ordinary Ard-to-Go name conversion, parameter lowering, receiver mutability/addressability, and return ABI lowering.

## Consequences

- Generated Go APIs become more idiomatic and easier to use from Go.
- Ard-defined methods can satisfy common Go interfaces such as `io.Writer` when their Ard signatures match semantically.
- `Result` and `Maybe` remain first-class Ard values through runtime shapes in value positions.
- The Go lowerer must become aware of expression context: ABI return position and value position use different representations.
- Calls returning `Result` or `Maybe` need packing when used as values, and unpacking or direct tuple return when used in ABI return position.
- Direct Go interop becomes more symmetric: Go `(T, error)` and `(T, bool)` map into Ard `Result` and `Maybe`, and Ard `Result`/`Maybe` returns map back to idiomatic Go.
- Non-Go backends are not required to use Go tuple/error ABI; this is a Go backend lowering decision.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0034-reset-go-backend-and-ffi-boundary.md`
- `docs/adrs/0035-use-go-packages-for-ffi-resolution.md`
- `docs/adrs/0037-define-unsafe-nil-interop-policy.md`
- `compiler/go`
- `compiler/runtime`
