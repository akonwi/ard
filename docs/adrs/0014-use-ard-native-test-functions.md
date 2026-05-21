# 0014: Use Ard-Native Test Functions

## Status

Accepted

## Context

Ard needs a built-in testing model that is easy to recognize, simple to run, and aligned with the language's explicit error handling philosophy. Tests should be ordinary Ard code where possible, not a separate mini-language or a panic-first assertion system.

The testing system also needs to fit Ard's file-based module model and native Go-target execution path.

## Decision

Use explicit `test fn` declarations for Ard tests and run them through the built-in `ard test` command.

A test is a function explicitly declared with `test fn`:

```ard
test fn adds_numbers() Void!Str {
  try testing::assert(1 + 1 == 2, "Expected 1 + 1 to equal 2")
  testing::pass()
}
```

Test functions must be top-level, take zero arguments, and return `Void!Str`.

All normal test outcomes are represented as values:

- `Result::ok(())` means pass
- `Result::err(message)` means fail
- panic/runtime crash is reported separately as panic

The standard library should provide `ard/testing` helpers that return `Void!Str`, including:

- `testing::pass()`
- `testing::fail(message: Str)`
- `testing::assert(condition: Bool, message: Str)`

`ard test` discovers tests from checked declarations marked as `test fn`, not from naming conventions alone. It should support co-located tests and dedicated `/test` files:

- co-located tests can access same-module private behavior
- `/test` files behave like external/integration tests and do not get special private access

`ard test` should compile in test mode so test declarations are included only for test execution. Normal `run` and `build` flows should not expose test-only declarations as production runtime surface area.

Tests should run through the Go/native target by default, sequentially, with clear reporting for pass, fail, and panic. `--filter` and `--fail-fast` provide basic runner control.

## Consequences

- Tests are explicit in source and easy to discover.
- Test failures align with Ard's `Result`-based recoverable error model.
- Panic remains visible as abnormal failure rather than the normal assertion mechanism.
- The parser, formatter, checker, and runner must preserve and validate `test fn` metadata.
- The compiler needs a test mode that includes tests for `ard test` while excluding them from normal production entrypoints.
- Sequential execution keeps v1 simple and avoids unnecessary flakiness with FFI, filesystem, env, SQL, and async behavior.
- Future testing features such as subtests, benchmarks, snapshots, fuzzing, or setup/teardown hooks can be added without changing the core model.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0005-use-result-maybe-and-try-for-error-handling.md`
- `docs/adrs/0013-use-file-based-modules-and-absolute-imports.md`
