# 0053: Add a Builtin Error Contract for Go Interop

## Status

Accepted

## Context

Ard is Go-native first, but its current treatment of Go's predeclared `error` type is limited to conventional return shapes:

- Go `error` returns become Ard `Void!Str`;
- Go `(T, error)` returns become Ard `T!Str`;
- error-returning Go callbacks use the corresponding `Str`-error Ard function types;
- generated Ard functions returning `Void!Str` or `T!Str` use Go's idiomatic `error` ABI.

This mapping is ergonomic and widely used, but it treats a Go error only as a message. Ard has no source-level type corresponding to a Go `error` value, so Go parameters, fields, and other value positions involving `error` cannot be represented naturally. Ard structs also cannot explicitly declare that they implement Go's `error` interface.

Named Go interfaces already support explicit implementation under ADR 0039, but `error` is predeclared and has no importable package-qualified name. Ard could expose it as a special foreign type, but Ard's Go-first model makes error interoperability common enough to warrant a first-class builtin contract.

Replacing the existing `Str` return mapping would break current Ard programs. The initial design should therefore add explicit error values and implementations while preserving established return and callback behavior.

## Decision

Add `Error` as a reserved builtin Ard contract corresponding to Go's predeclared `error` interface.

Conceptually, the contract is:

```ard
trait Error {
  fn error() Str
}
```

`Error` uses ordinary explicit Ard implementation syntax:

```ard
struct ValidationError {
  message: Str,
}

impl Error for ValidationError {
  fn error() Str {
    self.message
  }
}
```

On the Go target, this implementation emits an `Error() string` method and intentionally makes the generated Go type satisfy Go's `error` interface. An inherent method with the same shape does not establish Ard-level `Error` conformance; the explicit implementation is required.

The initial contract supports Ard structs. A non-mutating implementation gives the generated value type an `Error` method; a mutating implementation follows ordinary Ard method lowering and requires mutable addressability because only the generated pointer type satisfies Go's `error` interface.

### Constructing message-only errors

Provide a builtin factory:

```ard
Error::new(message: Str) Error
```

On the Go target, this lowers to `errors.New(message)`. It preserves the convenience of message-only failures while producing an explicit `Error` value.

### Go error values

Map Go's exact predeclared `error` type to builtin `Error` in ordinary value positions, including:

- function and method parameters;
- struct fields;
- container elements;
- local and returned foreign interface values outside conventional error-return adaptation.

For example, Go:

```go
func Report(err error)
```

is exposed as accepting Ard `Error`, allowing either `Error::new(...)` or a struct with an explicit `impl Error`.

Named Go interfaces that happen to contain `Error() string` remain ordinary named foreign interfaces. Only Go's exact predeclared `error` type maps to builtin `Error`.

### Preserve existing imported return and callback mappings

This decision is additive. Existing mappings remain unchanged:

| Go shape | Ard-facing shape |
| --- | --- |
| `error` return | `Void!Str` |
| `(T, error)` return | `T!Str` |
| `func(...) error` callback | `fn(...) Void!Str` |
| `func(...) (T, error)` callback | `fn(...) T!Str` |

Imported Go errors in these positions therefore continue to be reduced to their message. Existing source code requires no migration.

This creates a deliberate initial asymmetry: explicit `Error` values preserve Go error identity, while conventional imported return adaptation remains message-oriented for compatibility.

### Ard return ABI

Both `Str` and builtin `Error` result errors use idiomatic Go return shapes:

| Ard return type | Go ABI return shape |
| --- | --- |
| `Void!Str` | `error` |
| `T!Str` | `(T, error)` |
| `Void!Error` | `error` |
| `T!Error` | `(T, error)` |

`T!Str` keeps its existing behavior by constructing a Go error from the message. `T!Error` returns the underlying error value directly and preserves its identity.

In first-class value positions, Result remains explicit:

- `T!Str` uses `runtime.Result[T, string]`;
- `T!Error` uses `runtime.Result[T, error]` on the Go target;
- arbitrary `T!E` remains supported and is not required to implement `Error`.

The builtin contract does not change Ard's general rule that errors are values and Result error types are unconstrained.

### Relationship to traits and foreign interfaces

`Error` reuses Ard's explicit trait implementation semantics but has a compiler-defined Go representation. It is not resolved through `use go:` and does not become an ordinary generated Go trait interface.

The compiler recognizes this exact builtin contract in order to:

- reserve the `Error` type name;
- expose `Error::new`;
- map it to Go's predeclared `error` type;
- emit the required `Error() string` method for explicit implementations;
- use idiomatic Go error ABI lowering for `T!Error` returns.

Other Ard traits do not gain special Go error behavior merely by declaring a method named `error`.

## Consequences

- Ard structs can intentionally implement Go's `error` interface without a Go shim.
- Go APIs accepting or storing `error` values become directly representable.
- Ard APIs can preserve structured error identity by returning `T!Error`.
- Existing `T!Str` programs and imported Go error-return callbacks remain source-compatible.
- Error behavior is position-sensitive: imported conventional returns remain strings, while explicit `Error` values preserve the Go interface value.
- The checker, AIR, and Go backend need explicit builtin identity rather than relying on the name `Error` alone.
- Future work may reconsider imported return mapping, error wrapping, `errors.Is`/`errors.As`, typed extraction, and support for additional implementation targets.
- Non-Go backends must provide a representation for the builtin contract if they support programs using it; this ADR defines the Go-native representation first.

## Related

- `docs/adrs/0005-use-result-maybe-and-try-for-error-handling.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0038-use-idiomatic-go-abi-for-result-and-maybe-returns.md`
- `docs/adrs/0039-support-explicit-go-interface-interop.md`
- `docs/language-philosophy.md`
