# 0079: Evaluate `or` and `expect` Arguments Only When Absent

## Status

Accepted. Refines ADR 0078.

## Context

`or(default)` and `expect(message)` on a Maybe or a Result use their
argument only when the value is absent: none, or an error. Whether the
argument was evaluated when the value was present depended on how the Go
backend lowered it (#580):

```ard
let value = Maybe::new(1)
let a = value.or(fallback("plain default"))                    // fallback did not run
let b = value.or(maybe_fallback("default with setup").or(0))   // maybe_fallback ran
```

An argument that lowers to a plain Go expression was placed only in the
none or error branch, so it ran only on that path. An argument that lowers
to setup statements (a call returning a Maybe or a Result, `if`, `match`, or
`try` in value position) had those statements placed before the branch, so
they always ran. A `try` in the argument could then return early even though
the value was present. A Void-typed `or` evaluated its default before the
branch in every case.

## Decision

The argument of `or` and `expect` on Maybe and Result is evaluated only when
the value is absent. When the value is present, the argument is not
evaluated: its setup statements, its side effects, and any `try` in it do
not run.

- This is what programs already got in the common case, where the argument
  is a plain expression.
- It matches the language's own `or` operator: `a or b` does not evaluate
  `b` when it is not needed.
- A default that is expensive to compute is not paid for when the value is
  present.

The Go backend evaluates the target first, once, into a temporary, then
branches on it. The argument's setup statements and its use are both placed
in the none or error branch, the same shape as the right side of `and` and
`or` (`lazyOperandBody` in `compiler/go/operand_order.go`).

This is a special rule for these four methods. Every other method argument
is evaluated before the call, in source order (ADR 0078).

That includes the callback of `map` and `and_then` on Maybe and Result and
of `map_err` on Result. These methods call the callback only on one path,
and had the same split: a call that produces the callback, such as
`value.map(make_callback())`, ran only on that path when it lowered to a
plain expression and always ran when it lowered to setup statements. The
expression that produces the callback is now always evaluated, once, after
the target and before the branch. Whether the callback is called still
depends on the value. A callback whose evaluation has no effect (a function
literal, a named function, a Go function, or a variable or field read) stays
inline in the branch, so its generated Go is unchanged.

## Consequences

- Generated Go is byte-identical for arguments that lower to a plain
  expression. This was checked by comparing the generated Go before and
  after the change for the samples, the examples, and a program using plain
  arguments of every form. In the Kit CLI and the Cooper examples, the only
  differences are arguments with setup statements, which move into the
  branch.
- An argument with setup statements no longer needs the target to be held
  in an extra temporary, because nothing runs between the target and the
  branch.
- Callers cannot rely on side effects in the argument when the value is
  present. Code that needs an effect on both paths has to perform it before
  the call.
- A call that produces a callback for `map`, `and_then`, or `map_err` now
  runs on both paths. Callbacks written as function literals or names, the
  common case, generate the same Go as before; this was checked against the
  same programs.
