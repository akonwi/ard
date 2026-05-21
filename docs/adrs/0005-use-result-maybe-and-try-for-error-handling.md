# 0005: Use Result, Maybe, and Try for Error Handling

## Status

Accepted

## Context

Ard aims to make failures explicit and type-checked while keeping common happy-path code readable. Exception-based error handling would introduce hidden control flow and make recoverable failures less visible in function signatures.

Ard already has two language-level shapes for recoverable absence and failure:

- `Result` values, written as `ValueType!ErrorType`, for operations that can succeed with a value or fail with an error value.
- `Maybe` values, written as `Type?`, for values that may be absent.

Programs need a concise propagation mechanism for both shapes without losing type safety or making failure paths implicit.

## Decision

Use `Result`, `Maybe`, and `try` as Ard's primary recoverable error-handling model.

Recoverable failures should be represented in types:

- `T!E` means success with `T` or failure with `E`.
- `T?` means presence of `T` or absence.

`try` unwraps these values for happy-path code:

- Applying `try` to a successful `Result` unwraps the success value.
- Applying `try` to an error `Result` returns early from the current function with a compatible error result.
- Applying `try` to a present `Maybe` unwraps the contained value.
- Applying `try` to an absent `Maybe` returns early from the current function with `none` when the return type is compatible.

`try` may also use a catch block or function reference to transform the failure/absence case into the current function's return type. Catch blocks execute only for the error or none case and return early from the current function.

Restrictions:

- `try` applies only to `Result` and `Maybe` values.
- `try` without a catch requires the current function to return a compatible `Result` or `Maybe` type.
- Result propagation must preserve or explicitly transform the error type.
- Maybe propagation may change the inner success type, but absence remains absence unless transformed by a catch.

`panic` and unrecoverable runtime failures remain separate from this recoverable error model.

## Consequences

- Recoverable failures are explicit in function signatures.
- Callers can see and handle failure through normal type checking.
- Happy-path code remains readable through `try` without introducing exception-style hidden control flow.
- Functions that need user-facing or context-specific errors can transform failures at the propagation site.
- Backend implementations must preserve early-return semantics for `try` on both `Result` and `Maybe`.
- Interop/FFI boundaries should adapt recoverable host failures into `Result` or `Maybe` where appropriate instead of relying on exceptions or panics.

## Related

- `docs/adrs/0012-represent-optional-values-with-maybe.md`
- `docs/adrs/0015-use-esm-javascript-targets-with-explicit-runtime-semantics.md`
- `docs/adrs/0001-record-architecture-decisions.md`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
