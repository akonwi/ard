# 0078: Evaluate Operands in Source Order

## Status

Accepted. Amends ADR 0031.

## Context

The Go backend lowers an Ard expression to setup statements followed by a Go
expression that uses their results (ADR 0031, Control flow). Many
expressions lower to setup statements: a call that returns a Maybe, a Result,
or a tuple is unpacked from its Go results into temporaries; `if`, `match`,
and `try` in value position assign a temporary; a Void value is evaluated as
a statement.

When one expression combines several operands, each operand's setup
statements were appended in order, but each operand's own Go expression
stayed inline in the combined expression. The setup of a later operand
therefore ran before the inline part of every earlier operand (#571):

```ard
fn take(a: Int, b: Int?) {}

take(loud(), maybe()) // maybe() ran before loud()
```

```go
// maybe()'s setup statements
Take(Loud(), _tmp_9)
```

The same shape appeared wherever operands are combined: list, fixed-array,
map, struct, and Go struct literals; call arguments and the callee or
receiver they follow; binary operators and string interpolation; the target
and arguments of built-in methods; `select` channels and sent values; and an
assignment's target and value. Tuple literals had been fixed on their own
(#570).

## Decision

The Go backend evaluates the operands of a composite expression in source
order. Each operand is evaluated once, before the next operand starts.

One mechanism enforces this, `sequenceOperands` in
`compiler/go/operand_order.go`. Each site lowers its operands, hands them to
it in source order, and builds its Go expression from the expressions it
returns.

- When no operand after the first emits setup statements, the operands'
  statements are returned unchanged and every expression stays inline. Go
  evaluates the calls in an expression left to right, so nothing is added and
  the generated Go is unchanged.
- Otherwise, every operand to the left of the last one that emits statements
  is evaluated into a temporary immediately after its own statements.
- A temporary is declared with the operand's type, `var t T = value`, so an
  untyped constant expression keeps the type its context gives it. `t :=
  value` is used only where the adapted Go expression's own type is the one
  required.
- Operands whose value cannot change and whose evaluation has no effect stay
  inline: constants, function literals, and Void values.
- An assignment or mutation target is a place, not a value. A variable stays
  as it is; a dereference `*p` fixes its pointer (`t := p`, used as `*t`);
  any other place is fixed by address (`t := &place`, used as `*t`). Writes
  still reach the original storage.

Calls with a spread argument keep ADR 0062's rule of evaluating every
component into a temporary.

This decision orders the operands that are evaluated. It does not change
which operands are evaluated: the right side of `and` and `or` is still
evaluated only when needed.

New lowering code that combines operands must use `sequenceOperands` (or
`sequenceOperandPair` and `sequenceOperandTriple` for two or three value
operands) instead of concatenating their statements.

## Limits

These ordering problems remain and are tracked separately:

- Go orders only calls, method calls, and channel operations within an
  expression. A plain variable or field read next to a call can be evaluated
  after it, so `[box.value, bump(box)]` sees the value `bump` wrote (#575).
  Fixing this needs temporaries in code that has none today.
- Named arguments and struct literal fields reach AIR in declaration order,
  so the backend cannot see the written order (#576). Go struct literal
  fields are lowered in alphabetical order (#577).
- `push`, `swap`, and `sort` on a list repeat their receiver expression
  (#578).

## Consequences

- Operands evaluate left to right even when a later operand lowers to setup
  statements.
- Generated Go for code where no later operand emits setup statements is
  byte-identical to before. This was checked by building the backend with
  materialization disabled and comparing the generated Go for the samples,
  the examples, the Cooper examples, and the Kit CLI against the previous
  compiler.
- Code that hits the case gains one temporary per earlier operand that is
  not a constant, a function literal, or Void.
