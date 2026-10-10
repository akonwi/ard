# 0075: Allow Only Declarations at the Top Level of a Module

## Status

Accepted. Partially supersedes ADR 0002 and ADR 0031.

## Context

The checker accepted executable statements (calls, assignments, `if`,
loops, `defer`, and so on) at the top level of any module. AIR collected them
into a synthetic `Script` function, but only for root modules, and the Go
backend ran the script only when the program had no `main` (#533):

- In a module that defines `main`, top-level statements were checked and then
  silently dropped. The only real-world use found was
  `samples/word_frequency.ard`, which defined `fn main()` and also ended with
  a top-level `main()` call that never ran.
- In an imported module, top-level statements never ran at all, because
  imported modules never get a script function.
- Under `ard test`, a test file's top-level statements never ran, while
  `ard run` on the same file would run them.

So the same source meant different things depending on whether the module
had `main`, was imported, or was run as a test. A survey of every Ard project
available (this repository's samples, standard library, and examples, plus
several applications and libraries) found no code that relied on script
mode. Only the compiler's own tests used it.

Two alternatives were considered:

- **Keep script mode and reject only the cases that are ignored**
  (statements beside `main`, importing a module with statements, statements
  in test files). This keeps two ways to write a program, and makes a
  module's validity depend on how it is used.
- **Run top-level statements as module initialization**, like Go's `init`,
  before `main` and in import order. This makes importing a module run
  hidden side effects, adds ordering rules across modules, and runs setup
  code for every imported module under `ard test`. It also works against
  ADR 0021, which models module-level values as declarations.

## Decision

Only declarations may appear at the top level of a module:

- `let` and `mut` bindings (module globals, ADR 0021);
- functions, including `test fn` and static functions;
- type declarations: structs, enums, `type` unions and aliases, and traits;
- `impl` blocks and trait implementations;
- comments.

Any other top-level statement is a checker error (`top_level_statement`) that
points at the statement and suggests moving it into `main` or another
function. The rule is the same for every module, whether it is run, imported,
or tested, so it does not depend on check order or on how a module is used.

`fn main()` is a program's only entry point. `ard run` and `ard build` report
an error when the entry file has no `main`, instead of running an empty
program.

Script mode is removed throughout the pipeline:

- the checker no longer tracks a script scope; `defer` is allowed only in
  function, method, and closure bodies;
- AIR has a single execution root, `Program.Entry`, and no `Script` root or
  `Function.IsScript` flag;
- the Go backend's synthetic `package main` always calls the entry module's
  `Main`.

## Consequences

- Top-level code can no longer be silently ignored: code that previously
  compiled but never ran is now a compile error with a clear fix.
- Programs that used script mode need a `fn main()`. No known Ard code
  outside the compiler's tests did.
- Module-level initialization is expressed only through `let` and `mut`
  initializers, which ADR 0031 already orders through Go package-variable
  initialization.
- Checker tests that were written as bare statements are checked through a
  test-only helper that places each run of statements in a synthetic
  function, preserving source locations.
- Shared setup for tests, if it is needed later, should be a separate
  feature rather than top-level statements.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0021-represent-module-level-lets-as-air-globals.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `compiler/checker`
- `compiler/air`
- `compiler/go`
