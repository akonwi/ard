# Ard Testing Framework RFC

Status: implemented

## Summary

Add a built-in testing framework to Ard that is:
- built into the CLI via `ard test`
- explicit in source via `test fn`
- aligned with Ard's explicit `Result`-based error handling
- lightweight, readable, and consistent with Ard's philosophy

This design borrows from:
- Rust: explicit test declarations and internal vs integration testing split
- Gleam: lightweight feel and ordinary-language test code
- Go: built-in toolchain UX and pragmatic test running

## Design goals

- **Readable**: tests should be easy to recognize and understand at a glance.
- **Simple**: there should be one obvious way to write and run tests.
- **Explicit**: test outcomes should be represented as values, not hidden control flow.
- **Reliable**: the runner should isolate tests and report failures clearly.
- **Ard-native**: the feature should fit Ard's module model and existing semantics.

## Non-goals for v1

The first version should not attempt to include:
- parallel test execution
- subtests
- benchmarks
- fuzzing
- snapshots
- doctests
- `should_panic`
- setup/teardown hooks
- test temp-dir/env APIs

## Source syntax

Tests are declared with `test fn`.

```ard
test fn adds_numbers() Void!Str {
  try testing::assert(1 + 1 == 2, "Expected 1 + 1 to equal 2")
  testing::pass()
}
```

### Syntax notes

- `test` should be treated as a contextual keyword in `test fn`.
- v1 should allow `test fn` only for top-level function declarations.
- test functions must be zero-argument.
- test functions must return `Void!Str`.

## Test locations

Ard should support two testing styles.

### 1. Co-located tests

Tests can live beside the code they exercise.

```ard
fn add(a: Int, b: Int) Int {
  a + b
}

test fn add_works() Void!Str {
  try testing::assert(add(1, 2) == 3, "Expected add(1, 2) to equal 3")
  testing::pass()
}
```

This is the preferred style for unit/internal tests.

### 2. Dedicated `/test` directory

Projects can place dedicated test files under `/test`.

```text
my_project/
  ard.toml
  math.ard
  result.ard
  test/
    math_test.ard
    result_test.ard
```

This is the preferred style for integration/public API testing.

## Test result model

All tests return `Void!Str`.

This gives Ard a single explicit model:
- `Result::ok(())` => pass
- `Result::err(message)` => fail
- panic/runtime crash => panic classification in test output

### Why this model

- aligns with Ard's explicit error handling philosophy
- keeps tests "pure" in the sense that success/failure is a value
- avoids introducing a special mutable `testing.T`-style object
- avoids panic-based assertions as the normal failure path

## Failure categories

The runner should distinguish between:
- `PASS`
- `FAIL`
- `PANIC`

### FAIL

A normal test failure is an explicit returned error.

```ard
test fn fails() Void!Str {
  Result::err("expected x to equal 4")
}
```

### PANIC

A panic is still a failed test, but it should be reported separately as a more severe abnormal failure.

```ard
test fn crashes() Void!Str {
  panic("boom")
}
```

## Testing stdlib

Add a standard library module:

```ard
use "ard/testing"
```

Implemented API:
- `testing::pass() Void!Str`
- `testing::fail(message: Str) Void!Str`
- `testing::assert(condition: Bool, message: Str) Void!Str`

These helpers should return `Void!Str`, not panic.

### Intended usage

```ard
test fn adds() Void!Str {
  try testing::assert(1 + 1 == 2, "Expected 1 + 1 to equal 2")
  try testing::assert(2 + 2 == 4, "Expected 2 + 2 to equal 4")
  testing::pass()
}
```

Requiring `try` for intermediate assertions is acceptable and consistent with normal Ard code.

## CLI UX

Add a built-in CLI command:

```bash
ard test
```

Recommended v1 support:
- `ard test`
- `ard test <path>`
- `--filter <substring>`
- `--fail-fast`

Not needed in v1:
- `--list`

## Discovery rules

The source of truth is `test fn`.

A function is a test because it is explicitly declared with `test fn`, not because of:
- its name
- a file suffix alone

Runner discovery should:
1. walk the requested project/file/directory scope
2. include co-located source files in scope
3. include files under `/test`
4. enumerate checked declarations marked as test functions

## Visibility rules

### Co-located tests

Co-located tests should have same-module visibility and be able to test internal/private behavior in that module.

### `/test` tests

Tests under `/test` should behave like external/integration tests and should not receive special access to private symbols.

## Compilation mode

The compiler should gain a notion of **test mode**.

Recommended semantics:
- `ard test` compiles with test declarations included
- `run` and `build` do not treat tests as production entrypoints
- normal non-test flows should not expose test-only declarations as runtime surface area

## Execution model

v1 should run tests:
- sequentially
- in isolation
- in fresh VM executions
- with continue-on-failure by default
- with `--fail-fast` support

### Why sequential first

- simpler implementation
- easier debugging
- safer with FFI, filesystem, env, SQL, and async behavior
- less risk of flaky tests

## Reporting

Recommended output shape:

```text
running 4 tests

✓  math::add_works
✗  math::subtract_handles_negative
  expected -1, got 1

💥  parser::invalid_input_crashes
  panic: index out of range

✓  result::ok_wraps_value

2 passed; 1 failed; 1 panicked
```

The summary should distinguish ordinary failures from panics.

## Architectural fit with the current compiler

This design fits the current Ard architecture because:
- Ard already has `Result`, `Maybe`, and `panic`
- the VM can already run functions by name
- the module system is file-based, so a Rust/Gleam-style split maps better than a Go package merge model

## Implementation plan

1. **Finalize the v1 spec**
   - Lock syntax, result model, visibility rules, and CLI behavior.

2. **Add parser/AST support**
   - Parse `test fn` and preserve test metadata.

3. **Update formatter support**
   - Ensure `test fn` formats and round-trips correctly.

4. **Add checker validation**
   - Enforce top-level placement, zero parameters, and `Void!Str` return type.

5. **Define test-mode visibility semantics**
   - Co-located tests can test internals; `/test` files remain external-style.

6. **Add CLI discovery**
   - Implement `ard test`, path scoping, `/test` traversal, `--filter`, and `--fail-fast`.

7. **Enumerate tests from checked programs**
   - Expose stable checked metadata for the runner.

8. **Reuse emitter/VM execution**
   - Run discovered test functions by name and classify outcomes.

9. **Add `ard/testing` stdlib module**
   - Implement assertion helpers returning `Void!Str`.

10. **Implement reporting**
   - Distinguish `PASS`, `FAIL`, and `PANIC` clearly.

11. **Add full-stack tests**
   - Parser, checker, runner, and CLI coverage.

12. **Document the feature**
   - README, website docs, and backlog status updates.

## Likely files to touch

Highest-likelihood files:
- `compiler/main.go`
- `compiler/parse/ast.go`
- `compiler/parse/parser.go`
- `compiler/checker/checker.go`
- `compiler/std_lib/testing.ard` (new)

Likely support files:
- `compiler/formatter/printer.go`
- `compiler/bytecode/emitter.go`
- new runner/discovery files under `compiler/`
- parser/checker/formatter/VM tests

Possibly touched depending on implementation details:
- `compiler/parse/lexer.go`
- `compiler/checker/module_resolver.go`
- `compiler/ffi/*`
- `compiler/bytecode/vm/registry.gen.go`

## Validation plan

Run from `compiler/`:

```bash
go test ./parse
go test ./checker
go test ./bytecode/vm
go test ./formatter
go test ./...
go build
```

Manual validation should cover:
- co-located tests
- `/test` discovery
- explicit `FAIL` via `Result::err(...)`
- `PANIC` reporting
- `--filter`
- `--fail-fast`

If new FFI is added:

```bash
go generate ./bytecode/vm
go test ./bytecode/vm
go test ./...
```

## Open implementation questions

These should be settled early during implementation:
- exact initialization semantics when running tests individually
- exact file/path scoping rules for `ard test <path>`
- whether stdlib equality helpers need pure Ard implementations or small runtime support

## Recommended rollout philosophy

Keep v1 intentionally small and coherent.

The goal is not to ship every testing feature at once. The goal is to establish a clean Ard-native testing model that can be expanded later without redesigning the foundations.
