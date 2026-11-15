# 0081: Evaluate Constant Integer Expressions in the Checker

## Status

Accepted. Refines ADR 0072.

## Context

The Go backend emits integer literals and Go constants as Go constants. An
expression built only from them is therefore a Go constant expression, which
Go evaluates while compiling. Go rejects such an expression when:

- a value does not fit its type (for typed constants, every intermediate
  value must fit);
- an integer is divided by a constant zero, even when the dividend is a
  variable;
- a constant is converted to a type that cannot hold its value
  (`uint8(-1)`).

Ard checked each literal against its type, but not expressions built from
them. These programs passed the checker and then failed `go build` with an
error pointing into generated Go:

```ard
let a = 9223372036854775807 + 1   // overflows int
let b = Uint8::fit(0 - 1)         // constant -1 overflows uint8
let c = (1 << 62) << 2            // overflows int
let d = time::Second * 10000000000000
fn half(x: Int) Int { x / 0 }     // division by zero
```

The same operations on run-time values compile and wrap. `Uint8::fit(0 - 1)`
also broke ADR 0072's promise that `fit` always succeeds: the conversion was
emitted as Go's `uint8(0 - 1)`, which Go does not wrap for a constant.

ADR 0080 rejected the shift forms of this problem; this decision covers the
rest.

## Decision

The checker evaluates integer expressions whose operands are all constants,
exactly, and reports in Ard what Go would reject.

**Constants** are integer and rune literals, imported Go integer constants
(their exact values come from the Go type information), and the results of
`+`, `-`, `*`, `/`, `%`, unary `-`, and the bitwise operators applied to
constants. A conversion of a constant that keeps its value is a constant.

**Every intermediate value must fit its type.** `9223372036854775807 + 1 - 1`
is an error even though the final value fits. This is Go's rule for typed
constants, and every Ard value has a type: Ard has no untyped constants
beyond a literal adopting its context. The error is "Constant value
9223372036854775808 overflows Int", reported with the literal-overflow code.

**Division or remainder by a constant zero** is an error ("Division by
zero"), whatever the dividend, because Go rejects it. Float division is
unchanged: dividing a float by zero at run time is infinity.

**Evaluation follows Go's integer semantics**: `/` truncates toward zero, `%`
takes the sign of the dividend, `>>` is arithmetic on signed values, and `~`
of an unsigned value complements within its width.

### Relation to the conversion tiers (ADR 0072)

- `T::from(constant)` is lossless by construction and stays a constant.
- `T::fit(constant)` wraps exactly as `T::fit` of the same run-time value
  does: it keeps the low bits, as Go's `T(x)` does for a variable. When the
  value already fits, the conversion is emitted unchanged (so a Go constant
  keeps its name in the generated code, such as `int32(vaxis.KeyEnter)`), and
  its result is a constant. When it does not fit, the checker replaces the
  conversion with the wrapped literal, so `Uint8::fit(0 - 1)` is 255 and
  `Int8::fit(200 + 100)` is 44.
- `T::try(constant)` is unchanged; it is evaluated at run time.

`fit` of a literal is still an error that asks for `from`; the change applies
to constant expressions, which ADR 0072 treats as ordinary values.

### Implementation

Each arithmetic or bitwise node is folded once, when the checker creates it,
from its operands' values. Values are kept in a map on the checker keyed by
node, so evaluating a deep expression is linear and expressions that are not
constant cost a type switch and a map lookup per operation.

## Consequences

- The examples above are Ard errors with source locations, or (for `fit`)
  compile and wrap.
- A program that Go accepted only because a constant's intermediate value
  overflowed and came back into range is now rejected. None of the standard
  library, samples, examples, Cooper, or the Kit CLI is affected, and their
  generated Go is byte-identical.
- Float constant expressions are not evaluated. Go also rejects, for
  example, `1.0 / 0.0` or a float constant too large for `Float32::fit`; those
  can still surface as Go build errors.
