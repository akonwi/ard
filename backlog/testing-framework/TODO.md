# Testing Framework TODO

Status: complete

Keep this checklist updated as implementation progresses.

## Spec and design

- [x] Confirm final v1 syntax: `test fn`
- [x] Confirm all test functions must return `Void!Str`
- [x] Confirm failure classification: `PASS`, `FAIL`, `PANIC`
- [x] Confirm `/test` directory semantics
- [x] Confirm co-located test visibility semantics
- [x] Confirm `ard test` CLI scope behavior for project, directory, and file paths
- [x] Confirm initialization semantics for individually executed tests (each test runs in a fresh VM)

## Parser and AST

- [x] Add AST metadata for test functions
- [x] Parse `test fn`
- [x] Keep `test` contextual rather than globally reserved if possible
- [x] Restrict `test fn` to top-level declarations
- [x] Add parser tests for valid `test fn`
- [x] Add parser tests for invalid test declarations
- [x] Add parser tests for comments around test declarations

## Formatter

- [x] Add formatter support for `test fn`
- [x] Add formatter tests for `test fn`
- [x] Validate formatter idempotence for test declarations

## Checker

- [x] Validate test functions are top-level
- [x] Validate test functions have zero parameters
- [x] Validate test functions return `Void!Str`
- [x] Decide whether generic test functions are disallowed in v1 (disallowed)
- [x] Preserve test metadata in checked output
- [x] Implement test-mode semantics in checker/emitter pipeline (test functions stripped from `run`/`build`, included via `NewTestEmitter()` for `ard test`)
- [x] Add checker tests for invalid test signatures
- [x] Add checker tests for co-located test visibility
- [x] Add checker tests for `/test` privacy behavior

## CLI and discovery

- [x] Add `test` subcommand to `compiler/main.go`
- [x] Add `--filter` flag
- [x] Add `--fail-fast` flag
- [x] Implement project-root/path-scoped discovery
- [x] Include `/test` directory in discovery
- [x] Avoid duplicate file discovery
- [x] Enumerate tests from checked programs/modules
- [x] Add CLI/discovery tests

## Runner and execution

- [x] Add test runner orchestration code
- [x] Run each test in a fresh VM execution
- [x] Classify `Result::ok(())` as `PASS`
- [x] Classify `Result::err(...)` as `FAIL`
- [x] Classify panic/runtime errors as `PANIC`
- [x] Implement continue-on-failure default behavior
- [x] Implement `--fail-fast`
- [x] Add runner tests for pass/fail/panic outcomes
- [x] Add tests for filter behavior
- [x] Add tests for fail-fast behavior

## Stdlib

- [x] Add `compiler/std_lib/testing.ard`
- [x] Add `testing::fail(message: Str) Void!Str`
- [x] Add `testing::assert(condition: Bool, message: Str?) Void!Str`
- [x] Add `testing::equal(actual: $T, expected: $T) Void!Str`
- [x] Add `testing::not_equal(actual: $T, expected: $T) Void!Str`
- [x] Decide whether equality helpers can be implemented purely in Ard
- [x] Add stdlib tests for testing helpers (co-located in std_lib/testing.ard)

## Reporting

- [x] Add `PASS` output
- [x] Add `FAIL` output with message
- [x] Add `PANIC` output with panic details
- [x] Add final summary counts for passed/failed/panicked
- [x] Review output for readability and scanability

## Integration and regression coverage

- [x] Add end-to-end tests for co-located tests
- [x] Add end-to-end tests for `/test` discovery
- [x] Add end-to-end tests for explicit failure via `Result::err(...)`
- [x] Add end-to-end tests for panic classification
- [x] Run `go test ./parse`
- [x] Run `go test ./checker`
- [x] Run `go test ./bytecode/vm`
- [x] Run `go test ./formatter`
- [x] Run `go test ./...`
- [x] Run `go build`
- [x] Run manual `go run main.go test ...` checks
- [x] If needed, run `go generate ./bytecode/vm` (not needed — no new FFI)

## Docs

- [x] Add compiler/user docs for writing tests
- [x] Document co-located tests vs `/test`
- [x] Document `FAIL` vs `PANIC`
- [x] Document `--filter` and `--fail-fast`
- [x] Update root README if appropriate
- [x] Keep this backlog checklist updated during implementation

## Tree-sitter / Editor support

- [x] Update tree-sitter grammar to recognize the `test` keyword in `test fn` declarations
- [x] Update highlighting queries to highlight `test` as a keyword
- [x] Sync highlights to the Zed plugin
