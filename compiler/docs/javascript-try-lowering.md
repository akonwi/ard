# JavaScript `try` lowering

This document covers the backend implementation strategy for lowering Ard `try` on JavaScript targets.

It complements `compiler/docs/javascript-runtime-semantics.md`, which defines the semantic guarantees of Ard `try` on JS. This document focuses only on how those semantics are realized in generated JavaScript.

## Goal

Lower Ard `try` into ordinary JavaScript statement flow instead of using thrown sentinel objects and function-boundary `catch` handlers.

This keeps generated JavaScript closer to normal hand-written JS while preserving Ard semantics:

- `try` still unwraps `Result` / `Maybe`
- failures still return early from the current Ard function or lambda
- nested `try` inside larger expressions still works

## Why

The previous JS backend lowered Ard `try` by:

- wrapping `try` expressions in IIFEs
- throwing a sentinel object on `err(...)` / `none()`
- catching that sentinel at each emitted function boundary

That approach worked, but it made generated JS less idiomatic and harder to read.

The new approach always rewrites `try`-containing expressions into normal statement flow.

## Core idea

When an expression contains Ard `try`, the JS backend does not emit it directly as a nested JS expression.
Instead it lowers the expression into:

1. prelude statements that evaluate subexpressions in order
2. explicit early-return guards for `Result` / `Maybe`
3. a final simple JS expression value

Conceptually, the lowerer produces:

- `stmts`: statements that must run first
- `expr`: the simple value expression available afterwards

## Example

Ard:

```ard
fn add_one(value: Int!Str) Int!Str {
  let out = (try value) + 1
  Result::ok(out)
}
```

Lowered JS shape:

```js
function add_one(value) {
  const __try0 = value;
  if (__try0.isErr()) {
    return Result.err(__try0.error);
  }
  const out = (__try0.ok + 1);
  return Result.ok(out);
}
```

## Result lowering

Ard:

```ard
let value = try input
```

JS shape:

```js
const __try0 = input;
if (__try0.isErr()) {
  return Result.err(__try0.error);
}
const value = __try0.ok;
```

## Maybe lowering

Ard:

```ard
let value = try input
```

when `input` is a `Maybe`, lowers to:

```js
const __try0 = input;
if (__try0.isNone()) {
  return Maybe.none();
}
const value = __try0.value;
```

## Catch-block lowering

Ard:

```ard
try parse() -> err {
  "bad: " + err
}
```

JS shape:

```js
const __try0 = parse();
if (__try0.isErr()) {
  const err = __try0.error;
  return "bad: " + err;
}
```

So Ard catch blocks remain early-return branches, not JS exception handlers.

## Nested expression strategy

A parent expression containing `try` is normalized into statement flow.
For example:

```ard
let x = add(1, try foo())
```

becomes the JS shape:

```js
const __try0 = foo();
if (__try0.isErr()) {
  return Result.err(__try0.error);
}
const x = add(1, __try0.ok);
```

The same principle applies to:

- call arguments
- binary expressions
- list and map literals
- struct literals
- template strings
- `if` expressions
- block expressions
- match expressions

## Branching value expressions

Value-producing control-flow expressions such as:

- `if`
- block expressions
- `match`

are lowered through a temporary destination variable.

Example shape:

```js
let __if0;
if (cond) {
  __if0 = left;
} else {
  __if0 = right;
}
```

When a branch contains Ard `try`, that branch emits its own guard returns before assigning the final value.

## Function boundaries

With statement-based lowering in place, emitted JS functions no longer need special `try/catch` wrappers for Ard `try` propagation.

Generated functions now use ordinary JS control flow:

- explicit `return Result.err(...)`
- explicit `return Maybe.none()`
- ordinary local temporaries

## Non-goals

This lowering does **not** change Ard semantics. In particular:

- Ard `try` still only handles `Result` / `Maybe`
- it does not become JS exception handling
- `panic(...)` still throws a runtime failure
- non-sentinel JS exceptions are not caught by Ard `try`

## Current implementation notes

The current JS backend now:

- detects whether an expression tree contains Ard `try`
- lowers only `try`-containing expression trees into statement flow
- keeps existing direct expression emission for try-free expression trees
- removes the old `makeTryReturn(...)` sentinel path from the JS prelude
- removes function-boundary `catch (__ard_try)` wrappers

This keeps the implementation incremental while moving the backend onto the desired foundation for future cleanup.