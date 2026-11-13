# 0078: Evaluate Operands in Source Order

## Status

Accepted. Amends ADR 0031. Amended for reads before calls (#575).

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

### Reads before calls

Go orders only the calls, method calls, and channel receives in an
expression. Any other operand, such as a variable or field read, may be
evaluated after a call to its right, and the gc compiler does read it after
the call. In `[box.value, bump(box)]` the read saw the value `bump` wrote
(#575). The same applies to a call's receiver or callee relative to the calls
in its arguments.

`sequenceOperands` therefore also orders an operand that a call to its right
could change. It does this only when a later operand contains a call, and only
for an operand whose value a call can change:

- A local stays inline unless something other than its own function's
  statements can write it: its address is taken (`&x`, `&mut x`, or the
  address of a field in its own storage), a closure captures its binding slot
  (the closure assigns it), or the function contains an `unsafe` block, which
  runs as a Go function literal. This holds for `let` and `mut` locals and
  for parameters. A `let` local can change when Go code writes through a
  pointer to it. A closure that only reads a local captures a snapshot.
- A field of a struct or tuple value follows the value it belongs to. A field
  or element reached through a pointer, list, or map can change, and so can
  `p.*`. The length of a list or string value follows the value; the length
  of a map, or of anything reached through a pointer, can change.
- A `mut` module global and a Go package variable can change. A `let` module
  global and a Go constant stay inline.
- An operand that is itself a call, including a built-in that lowers to one
  such as `Maybe::new` or a string method, needs nothing: its arguments are
  read before it runs.

A pointer operand, such as the receiver of a built-in method on a `&mut`
list or map, is ordered as the pointer it holds. The method reads the pointee
when it runs, after its arguments, as a function taking the pointer would.

A later operand counts as containing a call when it calls a function, a
method, or a closure, or receives from a channel. Conversions, `len`, the
runtime package's functions, and the function literal that snapshots a
closure's captures do not count. The backend does not look into the callee:
any call may change anything reachable.

A value is ordered by wrapping it in a call to the runtime's identity
function, `ard.Read(value)`. Go evaluates a call's arguments before the call
and the calls of an expression left to right, so the read happens before any
call to its right. Wrapping adds no statements: an expression that had none
still has none, so it still lowers where statements are not allowed, such as
a while condition, and it does not cause earlier operands to be evaluated into
temporaries under the rule above. An assignment target is fixed by a
statement as before (`t := &place`, used as `*t`), since every assignment
already emits statements; this happens only when the target's pointer itself
can change, for example `holder.inner.value = f()` where `inner` is a pointer
field.

The classification works on the AIR expression each operand was lowered
from, carried in the operand as its source, together with facts about the
function being lowered. The lowered Go expression alone cannot tell a
conversion from a call or a field of a value from a field through a pointer.
An operand whose Go expression was adapted after lowering carries no source
and is treated as a read unless it is a call.

This decision orders the operands that are evaluated. It does not change
which operands are evaluated: the right side of `and` and `or` is still
evaluated only when needed.

New lowering code that combines operands must use `sequenceOperands` (or
`sequenceOperandPair` and `sequenceOperandTriple` for two or three value
operands) instead of concatenating their statements.

## Limits

These ordering problems remain and are tracked separately:

- Named arguments and struct literal fields reach AIR in declaration order,
  so the backend cannot see the written order (#576). Go struct literal
  fields are lowered in alphabetical order (#577).
- `push`, `swap`, and `sort` on a list repeat their receiver expression
  (#578).
- A `let` module global is read inline next to a call. Go code that receives
  its address can still write it; finding those globals needs a scan of the
  whole program rather than of one function.

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
- A read that a call to its right could change is wrapped in `ard.Read`.
  In the Kit CLI and the Cooper examples, about 1% of the expressions that
  combine operands gain one (391 wrappers among Kit's 33,400 such
  expressions, 204 among Cooper's 19,069); `std_lib` and the samples gain
  none. Generated Go grows by 0.1%, and compile time and allocation do not
  change measurably.
  Wrapping every non-constant read before a call instead would have changed 8
  to 11% of those expressions and grown generated Go by 1.3 to 2%; with
  temporaries instead of a wrapper, it would also have put statements into
  while conditions, which do not compile.
- With reads before calls left out, generated Go is byte-identical to before
  for the samples, `std_lib`, the Cooper examples, and the Kit CLI. With them,
  the only differences are the `ard.Read` wrappers and the few
  assignment-target statements.
- Lowering code that builds an operand must give it its AIR source, with
  `sourcedOperand` or the source parameter of `sequenceOperandPair` and
  `sequenceOperandTriple`, and must drop the source if it adapts the Go
  expression in a way that reads through a pointer.
