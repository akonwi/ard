# 0063: Preserve Imported Go Error Identity

## Status

Accepted

## Context

ADR 0053 introduced builtin `Error`, but retained the older compatibility mapping for conventional imported Go error returns and callbacks:

- Go `error` returns became `Void!Str`;
- Go `(T, error)` returns became `T!Str`;
- callbacks returning those shapes used corresponding `!Str` Ard function types.

That adaptation eagerly called `error.Error()` and discarded the original Go interface value. Ard code therefore lost sentinel identity, concrete error type, unwrap chains, and custom `Is`/`As` behavior. Reconstructing a Go error from the message could not recover that information.

The Go backend already represents builtin `Error` as Go's `error` interface and lowers `T!Error` returns to the idiomatic Go error ABI. The remaining asymmetry was therefore an import-signature and call-boundary policy rather than a representation limitation.

## Decision

Make identity-preserving error adaptation the sole mapping for exact predeclared Go error result shapes:

| Go shape | Ard-facing shape |
| --- | --- |
| `error` return | `Void!Error` |
| `(T, error)` return | `T!Error` |
| `func(...) error` callback | `fn(...) Void!Error` |
| `func(...) (T, error)` callback | `fn(...) T!Error` |

This is an immediate breaking change. There is no compatibility mode, contextual return adaptation, raw-call syntax, or error-specific import annotation.

The rule applies consistently to package functions, methods, imported interfaces, first-class function and method values, named Go function types, and generic Go signatures whose declared result is the exact predeclared `error` type. An ordinary result declared as a type parameter and instantiated with `Error` remains an ordinary `Error` value; generic substitution does not retroactively reinterpret its declaration as Go's conventional error-result shape.

Ordinary Go `error` value positions continue to map to builtin `Error`. Pure Ard is unchanged: a function declared to return `Error` returns an ordinary value, while only an explicit `T!Error` return has `Result` semantics. Pure Ard `T!Str` also remains valid and continues to lower to Go's idiomatic error ABI by constructing a message-only Go error.

### Sole-error ambiguity

Go's type system does not distinguish a conventional failure return such as `Close() error` from an error-valued result such as `errors.Unwrap(error) error`. Ard accepts this ambiguity at the direct-Go boundary and applies the one signature-based rule consistently.

Consequently, an idiosyncratic value-returning API such as `errors.Unwrap` appears as `Void!Error`: nil becomes `ok(())`, and a non-nil returned error is obtained from the `err(error)` arm. A pure Ard wrapper may expose a more natural `Error?` API. The compiler does not infer intent from package or function names.

### Boundary packing

Imported error calls test the returned Go error against nil. Success constructs the `ok` arm; failure stores the original Go error interface directly in the `err` arm. The adapter must not call `Error()` while packing the Result.

Error-returning Ard callbacks forward an `Error` payload directly to Go. Message-only callback failures construct one explicitly with `Error::new`.

### String interpolation

Builtin `Error` is stringifiable in Ard interpolation:

```ard
"request failed: {error}"
```

Interpolation observes the value through `error.error() Str`. This is an interpolation-only coercion: `Error` is not assignable or passable as `Str`, and other message conversion remains explicit through `error.error()` or `Result.map_err`.

Existing `to_str` and `ToString` interpolation behavior takes precedence when a concrete type supports both string conversion and `Error`. A value statically typed as `Error` uses the `Error` contract.

## Migration

Code forwarding imported failures should change its error type from `Str` to `Error`:

```ard
fn load() [Byte]!Error {
  os::ReadFile("config.json")
}
```

APIs intentionally exposing messages convert explicitly:

```ard
os::ReadFile(path).map_err(fn(error: Error) Str { error.error() })
```

Callbacks construct message-only errors with `Error::new`. Existing interpolations such as `"failed: {error}"` remain source-ergonomic.

## Consequences

- Imported Go sentinel identity, concrete dynamic type, wrapping chain, and `errors.Is` behavior survive the Ard boundary.
- Error propagation through `try`, function values, methods, and callbacks remains lossless.
- Existing source annotations and wrappers expecting imported `!Str` results must migrate immediately.
- The sole-`error` ambiguity remains a predictable exception confined to direct Go interop.
- No new runtime representation or syntax is required.

## Supersedes

This ADR supersedes ADR 0053's imported-return and callback compatibility mapping. ADR 0053's builtin `Error` contract, ordinary value-position mapping, explicit implementation semantics, and `T!Error` ABI decisions remain in force.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0038-use-idiomatic-go-abi-for-result-and-maybe-returns.md`
- `docs/adrs/0053-add-a-builtin-error-contract-for-go-interop.md`
- `docs/language-philosophy.md`
