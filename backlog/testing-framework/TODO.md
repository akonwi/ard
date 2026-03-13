# Testing Framework TODO

Status: planned

Keep this checklist updated as implementation progresses.

## Spec and design

- [ ] Confirm final v1 syntax: `test fn`
- [ ] Confirm all test functions must return `Void!Str`
- [ ] Confirm failure classification: `PASS`, `FAIL`, `PANIC`
- [ ] Confirm `/test` directory semantics
- [ ] Confirm co-located test visibility semantics
- [ ] Confirm `ard test` CLI scope behavior for project, directory, and file paths
- [ ] Confirm initialization semantics for individually executed tests

## Parser and AST

- [ ] Add AST metadata for test functions
- [ ] Parse `test fn`
- [ ] Keep `test` contextual rather than globally reserved if possible
- [ ] Restrict `test fn` to top-level declarations
- [ ] Add parser tests for valid `test fn`
- [ ] Add parser tests for invalid test declarations
- [ ] Add parser tests for comments around test declarations

## Formatter

- [ ] Add formatter support for `test fn`
- [ ] Add formatter tests for `test fn`
- [ ] Validate formatter idempotence for test declarations

## Checker

- [ ] Validate test functions are top-level
- [ ] Validate test functions have zero parameters
- [ ] Validate test functions return `Void!Str`
- [ ] Decide whether generic test functions are disallowed in v1
- [ ] Preserve test metadata in checked output
- [ ] Implement test-mode semantics in checker pipeline
- [ ] Add checker tests for invalid test signatures
- [ ] Add checker tests for co-located test visibility
- [ ] Add checker tests for `/test` privacy behavior

## CLI and discovery

- [ ] Add `test` subcommand to `compiler/main.go`
- [ ] Add `--filter` flag
- [ ] Add `--fail-fast` flag
- [ ] Implement project-root/path-scoped discovery
- [ ] Include `/test` directory in discovery
- [ ] Avoid duplicate file discovery
- [ ] Enumerate tests from checked programs/modules
- [ ] Add CLI/discovery tests

## Runner and execution

- [ ] Add test runner orchestration code
- [ ] Run each test in a fresh VM execution
- [ ] Classify `Result::ok(())` as `PASS`
- [ ] Classify `Result::err(...)` as `FAIL`
- [ ] Classify panic/runtime errors as `PANIC`
- [ ] Implement continue-on-failure default behavior
- [ ] Implement `--fail-fast`
- [ ] Add runner tests for pass/fail/panic outcomes
- [ ] Add tests for filter behavior
- [ ] Add tests for fail-fast behavior

## Stdlib

- [ ] Add `compiler/std_lib/testing.ard`
- [ ] Add `testing::fail(message: Str) Void!Str`
- [ ] Add `testing::assert(condition: Bool, message: Str?) Void!Str`
- [ ] Add `testing::equal(actual: $T, expected: $T) Void!Str`
- [ ] Add `testing::not_equal(actual: $T, expected: $T) Void!Str`
- [ ] Decide whether equality helpers can be implemented purely in Ard
- [ ] Add stdlib tests for testing helpers

## Reporting

- [ ] Add `PASS` output
- [ ] Add `FAIL` output with message
- [ ] Add `PANIC` output with panic details
- [ ] Add final summary counts for passed/failed/panicked
- [ ] Review output for readability and scanability

## Integration and regression coverage

- [ ] Add end-to-end tests for co-located tests
- [ ] Add end-to-end tests for `/test` discovery
- [ ] Add end-to-end tests for explicit failure via `Result::err(...)`
- [ ] Add end-to-end tests for panic classification
- [ ] Run `go test ./parse`
- [ ] Run `go test ./checker`
- [ ] Run `go test ./bytecode/vm`
- [ ] Run `go test ./formatter`
- [ ] Run `go test ./...`
- [ ] Run `go build`
- [ ] Run manual `go run main.go test ...` checks
- [ ] If needed, run `go generate ./bytecode/vm`

## Docs

- [ ] Add compiler/user docs for writing tests
- [ ] Document co-located tests vs `/test`
- [ ] Document `FAIL` vs `PANIC`
- [ ] Document `--filter` and `--fail-fast`
- [ ] Update root README if appropriate
- [ ] Keep this backlog checklist updated during implementation
