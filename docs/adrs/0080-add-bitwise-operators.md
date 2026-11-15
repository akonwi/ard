# 0080: Add Bitwise Operators

## Status

Accepted.

## Context

Ard code that works with Go bit flags had no way to test or combine them
(#568). Cooper, a TUI framework written in Ard over Vaxis, kept a Go bridge
for a single expression:

```go
func HasModifier(modifiers, flag vaxis.ModifierMask) bool {
	return modifiers&flag != 0
}
```

Arithmetic can fake a single-bit test on a non-negative value, as in
`(Int::from(mods) / Int::from(flag)) % 2 == 1`, but it hides what the code
means and does not cover clearing, combining, or packing bits. Cooper's
Fenwick tree also keeps a list of precomputed steps only because it cannot
write `i & -i`. Bit masks appear throughout Go APIs: key modifiers, file
modes, syscall flags, and protocol and encoding work.

Before this change, none of `^`, `~`, `<<`, `>>`, a binary `|`, or a binary
`&` compiled, so no existing program changes meaning.

## Decision

Ard has the operators `&` (and), `|` (or), `^` (xor), prefix `~` (not), `<<`
(shift left), and `>>` (shift right) on integers.

```ard
fn has_modifier(value: vaxis::ModifierMask, flag: vaxis::ModifierMask) Bool {
  value & flag != 0
}

let mods = vaxis::ModShift | vaxis::ModCtrl
let cleared = mods & ~vaxis::ModShift
let packed = Uint16::from(hi) << 8 | Uint16::from(lo)
```

### Syntax

The operators are the symbols Go, C, Rust, and Python use. Logic keeps its
words (`and`, `or`, `not`), as in Python.

- A binary `&` follows an operand on the same line; a prefix `&` is still
  the address-of operator (ADR 0073), the same way binary and unary `-`
  coexist. `a & &b` is an and with an address-of operand. A new line ends an
  expression, so `&x` at the start of a line is still a new statement.
- A binary `|` cannot collide with type unions: `|` separates union members
  only in `type` declarations.
- `<<` and `>>` are two adjacent `<` or `>` tokens, recognised only where a
  binary operator can appear. The lexer is unchanged, so `Map<Str,
  List<Int>>` still closes one type argument list per `>`. Type arguments
  never start with `<`, so `f<<g` is a shift, not a generic call.
- `~` is bitwise not. Go spells it `^x` and Rust `!x`; both are parse errors
  that name `~`. Go's `&^` (and not) is written `a & ~b`.
- `&&` and `||` are parse errors that suggest `and` and `or`. `&mut` after an
  expression is a parse error. `>>>` is a parse error suggesting a shift of an
  unsigned value. There is no compound assignment (`=|`), which would read
  too much like `= |` and `= &x`.

### Precedence

The operators follow Go's precedence ladder:

| level | operators |
| --- | --- |
| unary | `-` `~` `&` `&mut` |
| multiplicative | `*` `/` `%` `<<` `>>` `&` |
| additive | `+` `-` `\|` `^` |
| comparison | `==` `!=` `<` `<=` `>` `>=` |
| | `and` |
| | `or` |

Operators on one level are left associative. Every bitwise operator binds
tighter than a comparison, so `mods & flag != 0` is `(mods & flag) != 0`,
avoiding C's best-known precedence mistake. Ard compiles to Go and its
users read Go code and Go APIs, so an expression copied from Go means the
same thing in Ard.

### Types

The operators accept `Int`, every sized integer (`Int8` to `Int64`, `Uint`
to `Uint64`), `Uintptr`, `Byte`, and foreign named types whose underlying
type is one of those, such as `vaxis::ModifierMask` or `os::FileMode`,
without conversion. Typed Go constants such as `vaxis::ModShift` keep their
named type (ADR 0051).

- `&`, `|`, and `^` require both operands to have the same type, as the
  arithmetic operators do, and an untyped literal adopts the other operand's
  type: `mods & 4`. The result has that type.
- `~x` has the type of `x`. A literal under `~` adopts a type the way a
  negated literal does: `let mask: Uint8 = ~0` is 255, and `u8 & ~15` types
  `15` as `Uint8`.
- A shift count may be any of those integer types, independently of the
  left operand, as in Go and Rust. The result has the left operand's type. A
  literal left operand does not adopt the count's type: `1 << n` is an `Int`
  for any integer `n`.
- `Rune` is excluded: a `Rune` is always a valid Unicode scalar value, and
  bit operations would manufacture invalid ones. Convert with
  `Int32::from(r)`. Floats, `Bool`, and other types are rejected, with a hint
  to use `and`, `or`, or `!=` for `Bool`.

### Shift semantics

Shifts have Go's semantics:

- A count of the operand's width or more shifts every bit out: the result is
  0, or -1 for `>>` of a negative signed value.
- `>>` is arithmetic on signed values and logical on unsigned ones.
- A negative count at run time panics, like division by zero.
- Values wrap at the type's width: `Uint8` 200 `<< 1` is 144.

Go rejects a negative constant count even for a variable operand, so the
checker rejects a negative literal count. A shift of two constants is a Go
constant expression, evaluated while compiling (ADR 0081): its result must
fit its type (`1 << 63` overflows `Int`), while `0 << 64` and `-1 >> 64`
are 0 and -1 as above. Go also rejects a constant shift count above 1074
whatever the result, and so does the checker. A variable shifted by a large
literal count is allowed and follows the rules above.

### Lowering

The checker produces `IntBitwise` (with an operator) and `IntBitNot`. AIR
has `ExprIntBitAnd`, `ExprIntBitOr`, `ExprIntBitXor`, `ExprIntShl`,
`ExprIntShr`, and `ExprIntBitNot`. The Go backend emits Go's `&`, `|`, `^`,
`<<`, `>>`, and unary `^`, through the same operand-ordering path as the
arithmetic operators (ADR 0078). A constant left operand of a shift (a
literal, an imported Go constant, or an expression of those) is emitted
with its type, as in `int(1) << n`: Go gives an untyped constant shifted by
a non-constant count the type its context expects, and `float64(1 << n)`
does not compile.

## Alternatives Considered

- **Named methods** (`a.bit_and(b)`, `a.shl(n)`). No parser, formatter, or
  grammar changes and no precedence to decide, but packing code becomes long
  chains, foreign named integers would need a new built-in method path, and
  the issue's `a.or(b)` would sit next to the lazy `maybe.or(default)` with
  an unrelated meaning.
- **Keyword operators** (`band`, `bor`, `bxor`, `bnot`, `shl`, `shr`, as in
  Erlang and OCaml). Free of symbol collisions, but reserves six words; the
  Kit CLI already uses `band` as a variable name.
- **Python/Rust precedence** (shifts, then `&`, `^`, `|`, each on its own
  level below `+`). Familiar, but `a & b + c` and `a + b << 1` would mean
  something different than in Go, without an error, in code ported from Go.
- **Requiring parentheses** when mixing bitwise operators with each other or
  with arithmetic. Removes the disagreement between Go and Rust entirely and
  could be loosened later, but costs parentheses in common packing code.
- **Panicking on oversized shift counts** (as Rust's debug builds do) or
  **masking the count** (as Java and C# do). Both need a check emitted on
  every shift and depart from the Go semantics that Ard's integers already
  follow for wrapping.

## Consequences

- Cooper's `HasModifier` bridge and the Fenwick tree's step list can be plain
  Ard.
- `&&` now reports "use 'and'" in every position, and `||` reports "use
  'or'". The #567 message "Ard has no bitwise '&' operator" is gone.
- The tree-sitter grammar, Zed highlights, and the website highlighter
  recognise the operators.
- Ard still has no hexadecimal, octal, or binary literals, so masks are
  written in decimal (`255`, `493`).
