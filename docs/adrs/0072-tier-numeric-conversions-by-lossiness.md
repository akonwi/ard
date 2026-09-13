# 0072: Tier Numeric Conversions by Lossiness

## Status

Accepted

## Context

Ard has two overlapping spellings for numeric conversion, and neither one is
checked:

- `T::from(value)` (#284) converts into any sized scalar (`Int8`…`Uint64`,
  `Uintptr`, `Byte`, `Float32`) or foreign named numeric type with Go `T(x)`
  semantics. Integer narrowing wraps silently. Float sources are admitted only
  when the target is also a float (`checkScalarFrom`, `checker.go:7156-7159`),
  so float→int is currently rejected outright.
- `.to_int()` on `Byte`, `Rune`, and `Float64`, and `.to_f64()` on `Int`, do
  the same conversions as methods on the source value. `Float64.to_int()` is
  the one truncating float→int path that exists today.

`Int`, `Float64`, and `Rune` are not `from` targets, purely because the
checker's `scalarTypeByName` table omits them (`checker.go:1750`). So
`Float64::from(f32)` — a conversion that cannot lose information — does not
compile, while `Uint8::from(large_int)` silently wraps. That asymmetry is what
#500 reported.

Go's `T(x)` is a single unchecked operation for every numeric pair, and its
float→int behavior on NaN or out-of-range input is implementation-defined.
Rust separates conversions by whether information can be lost: `From`/`Into`
exist only for lossless pairs, `TryFrom` returns a `Result` for fallible pairs,
and `as` is the explicit always-succeeds lossy form with defined saturation for
float→int. That split lets the compiler reject accidental narrowing.

Ard's philosophy prefers one way to do things and prefers compile-time safety.
The current surface offers two ways to do an unchecked thing and no way to do a
checked one.

## Decision

Replace the method-style conversions with a single family of static functions
on the **target** type, split into three tiers by lossiness. These are checker
intrinsics resolved on the type name, following the `Str::from` (#283) and
`T::from` (#284) pattern — not prelude modules. The checker knows the source
and target types statically, so it can reject the wrong tier.

| tier | spelling | returns | permitted when |
|---|---|---|---|
| lossless | `T::from(x)` | `T` | every source value is exactly representable in `T` |
| checked | `T::try(x)` | `T?` | the pair is fallible |
| forced | `T::fit(x)` | `T` | the pair is lossy but total |

```ard
let wide: Float64 = Float64::from(ratio)              // Float32 → Float64
let ms: Int = try Int::try(elapsed).ok_or("too big")  // Int64 → Int?
let low: Byte = Byte::fit(hash)                       // Int → Byte, wraps
```

Exactly one tier compiles for any given source/target pair. Using the wrong
tier is a compile error naming the right one, so each conversion has a single
spelling and the lossy spellings always announce themselves.

### Targets and sources

All numeric scalars are both sources and targets: `Int`, `Int8`, `Int16`,
`Int32`, `Int64`, `Uint`, `Uint8`, `Uint16`, `Uint32`, `Uint64`, `Uintptr`,
`Byte`, `Rune`, `Float32`, `Float64`, and foreign named scalar types whose
underlying Go type is numeric.

A foreign named scalar participates as its underlying **Go kind**, resolved by
`foreignScalarPrimitive`: a Go `type Duration int64` behaves as `Int64`, a
`type Color int32` behaves as `Int32` (never as `Rune`), and a
`type Flag uint8` behaves as `Uint8`. Unicode validation is never inferred from
a foreign type's width.

`Str` and `Bool` are not numeric and take no tier. `Str::from([Byte])` and
`Str::from([Rune])` keep their current meaning; `Str::try` and `Str::fit` do
not exist and report an unknown static function. `.to_str()` stays on every
primitive because it is formatting, not numeric conversion.

### Identity

When the source and target resolve to the same primitive after
`foreignScalarPrimitive`, the pair is lossless and `T::from(x)` is a no-op
conversion. This covers `Int::from(i: Int)`, `Int64::from(d: time::Duration)`,
`time::Duration::from(ms: Int64)`, `Duration::from(other: Duration)`, and
`Byte` ↔ `Uint8`, which share one primitive. `try` and `fit` are compile errors
on an identity pair.

`Rune` and `Int32` are **not** the same primitive: `Rune` carries a validity
invariant that `Int32` does not (see below).

### Platform-sized types

`Int`, `Uint`, and `Uintptr` are platform-sized. Go guarantees only that they
are at least 32 bits, so the matrix treats them as **32 bits when they are a
target and 64 bits when they are a source**. A conversion must be correct on
every supported platform, so `Int32::from(i: Int)` is rejected — `Int` may hold
64 bits — while `Int::from(i: Int32)` is lossless. This is why `Int64 → Int`
(the #500 case) is fallible even though it never fails on a 64-bit host.

`Uint → Uintptr` is likewise excluded even though the two are the same width on
every real Go platform, because nothing guarantees it. This asymmetry is a
compiler rule that users will meet in diagnostics, so it must be documented in
the language guide, not only here.

### `Rune` is a validated scalar

ADR 0026 defines `Rune` as a Unicode scalar value, excluding surrogates. This
ADR preserves that invariant: **no operation may manufacture an invalid
`Rune`.**

- `Rune::from(x)` accepts only `Byte`/`Uint8`, whose whole range is valid.
- `Rune::try(x)` accepts any other integer or float source and returns `none`
  unless the value is a valid scalar.
- **`Rune::fit` does not exist.** There is no total lossy conversion into
  `Rune`, because the only candidates would either forge an invalid value or
  silently substitute `U+FFFD`.

Because every `Rune` is a valid scalar (`0..0x10FFFF`), `Rune` as a *source*
widens losslessly into anything holding that range, including `Uint32`,
`Uint64`, `Uint`, `Uintptr`, and `Float32`.

### Lossless matrix

`Float32` holds 24 significant bits and `Float64` holds 53, so only integers
narrower than that are lossless into a float. Identity pairs (above) are
lossless and omitted from the table.

| source | lossless targets |
|---|---|
| `Int8` | `Int16` `Int32` `Int64` `Int` `Float32` `Float64` |
| `Int16` | `Int32` `Int64` `Int` `Float32` `Float64` |
| `Int32` | `Int64` `Int` `Float64` |
| `Int64` | — |
| `Int` | `Int64` |
| `Uint8`, `Byte` | `Int16` `Int32` `Int64` `Int` `Uint16` `Uint32` `Uint64` `Uint` `Uintptr` `Rune` `Float32` `Float64` |
| `Uint16` | `Int32` `Int64` `Int` `Uint32` `Uint64` `Uint` `Uintptr` `Float32` `Float64` |
| `Uint32` | `Int64` `Uint64` `Uint` `Uintptr` `Float64` |
| `Uint64` | — |
| `Uint` | `Uint64` |
| `Uintptr` | `Uint64` |
| `Rune` | `Int32` `Int64` `Int` `Uint32` `Uint64` `Uint` `Uintptr` `Float32` `Float64` |
| `Float32` | `Float64` |
| `Float64` | — |

Every pair not listed and not an identity pair is fallible (`try`) or lossy
(`fit`). Notably `Int → Float64` is **not** lossless, because `Int` may carry
64 bits and `Float64` has 53 of mantissa.

### `try` semantics

`T::try(x)` returns `T?`. The success condition depends on the target class.

**Integer targets** — `some` exactly when `x` is representable in `T`
unchanged:

- integer → integer: `x` lies within `T`'s range. Signedness is respected; a
  matching bit pattern is not sufficient, so `Uint8::try(n: Int8)` is `none`
  for negative `n`.
- float → integer: `x` is finite, integral, and within `T`'s range. `NaN`,
  `±Inf`, and fractional values are `none`. Negative zero is integral and in
  range, so it yields `some(0)`.

**Float targets** — `some` when the *result is finite*; rounding is permitted
and does not make the conversion fail:

- `Float32::try(x: Float64)` is `none` only when a finite `x` overflows to
  `±Inf`. `Float32::try(0.1)` is `some` (rounded), and a finite value that
  underflows to a subnormal or to `0.0` is `some`. `NaN` and `±Inf` inputs are
  preserved and yield `some`.
- Integer → float can always round, never overflow, so it is never fallible:
  `Float64::try(i: Int64)` is a compile error pointing at `fit`. This keeps the
  compiler out of the business of proving float exactness, which cannot be done
  portably (see Implementation notes).

**`Rune` target** — `some` when `x` is a valid Unicode scalar
(`0..0x10FFFF`, excluding `0xD800..0xDFFF`). A float source must additionally
be finite and integral: both the float→integer rule and the scalar rule apply.

### `fit` semantics

`T::fit(x)` always produces a `T`. Where Go defines the conversion, `fit` is
Go's `T(x)`; where Go leaves it implementation-defined, this ADR defines it:

- integer → integer: two's-complement wrap (Go's defined behavior).
- integer → float: IEEE round-to-nearest (Go's defined behavior).
- `Float64 → Float32`: IEEE round-to-nearest; a finite value that exceeds
  `Float32`'s range becomes `±Inf`. Go calls this implementation-dependent for
  non-constant operands, so Ard defines it here rather than inheriting it.
- float → integer: **saturating**. Values at or beyond `T`'s bounds clamp to
  `T`'s minimum or maximum, and `NaN` becomes `0`. This replaces Go's
  implementation-defined result and matches Rust's `as`.

There is no `fit` into `Rune` (above), and no `fit` on a lossless or identity
pair.

### Literals

A numeric literal argument adopts the target type, as today, so an
out-of-range literal is a compile error. Since an adopted literal is by
definition lossless, `try` and `fit` reject literal arguments and point at
`from`; `Int64::from(5)` remains valid.

"Literal" here is the existing `isNumericLiteralNode` predicate
(`checker.go:7178`): a numeric literal, optionally negated. A constant
*expression* such as `Int8::try(1 + 2)` is an ordinary runtime value of its own
inferred type and goes through normal tier checking.

### Removed forms

- `Int.to_f64()` → `Float64::fit(i)` (`Int → Float64` is lossy).
- `Float64.to_int()` → `Int::try(f)` or `Int::fit(f)`.
- `Byte.to_int()`, `Rune.to_int()` → `Int::from(b)`, `Int::from(r)`.
- Byte and rune arithmetic, which ADR 0026 routed through `to_int()` and
  `Byte::from_int`, becomes `Byte::try(Int::from(b) + 1)` or
  `Byte::fit(Int::from(b) + 1)`.
- Existing narrowing `T::from` calls become compile errors naming both
  replacements.

`unsafe::cast<T>()` is unchanged: it is a dynamic type test on `Any`, not a
numeric conversion, and `Int` and `Int64` remain distinct there.

### Superseded and amended decisions

- **ADR 0026** specified prelude modules `ard/byte` and `ard/rune` exposing
  `Byte::from_int` and `Rune::from_int`, "matching the existing `Int::from_str`
  style". Those modules and `Int::from_str` do not exist after the ADR 0034
  std-lib reset. That section is superseded: primitive statics are checker
  intrinsics, and `Byte::try`/`Rune::try` replace the `from_int` constructors.
  ADR 0026's `Rune` validity invariant is retained and strengthened.
- **ADR 0031** lists `to_int`, `to_float`, `Byte::from_int`, `Rune::from_int`
  as the conversion surface; that paragraph is replaced by the three tiers.
  0031 also states the runtime introduces no shared types beyond `Maybe` and
  `Result`; this ADR adds runtime **functions** only, which is consistent with
  existing helpers such as `runtime/unsafe.go`'s `IsNil`.

## Implementation notes

### Checker

- Tier admissibility is a pure function of `(source primitive, target
  primitive, tier)` implemented as one table. Foreign named scalars resolve
  through `foreignScalarPrimitive` first, keyed on the Go underlying kind.
- `scalarTypeByName` (`checker.go:1750`) gains `Int`, `Float64`, and `Rune`.
  Its other call sites (`builtin_types.go:4`, `checker.go:1915`) are unaffected
  by the additions.
- Static dispatch currently cuts the `::from` suffix (`checker.go:9624-9632`);
  `try` and `fit` need the same handling, including the existing spread
  rejection.
- `ScalarFrom` gains a tier field.
- Removing the four methods touches the method tables in `checker/types.go`
  (`byteType.get` `:285`, `runeType.get` `:330`, `_int.get` `:376-388`, and the
  float equivalent) and the name maps in `checker/builtin_methods.go:16-20`,
  not only `createByteMethod`/`createRuneMethod`/`createIntMethod`/
  `createFloatMethod` (`checker.go:7227-7296`). Removing only the constructors
  would leave the method resolvable and then panic.

### Diagnostics

All four use the existing invalid-conversion diagnostic so the LSP has a stable
anchor for quick fixes. Wording, with the offending call spanned:

```
lossy pair given `from`
  Int may not hold every Int64 value; use Int::try for a checked
  conversion or Int::fit to truncate

lossless pair given `try` / `fit`
  every Float32 value is exactly representable as Float64; use Float64::from

literal given `try` / `fit`
  a literal is range-checked at compile time; use Uint8::from

removed method
  to_f64 has been removed; use Float64::fit(value)
```

The platform-sized rule needs its own note in the lossy-pair case, e.g.
"because `Int` may be 64 bits on some platforms", so the error does not look
like a compiler bug on a 64-bit host.

### AIR and Go backend

- `ExprScalarConvert` is shared with `ForeignScalarConvert` and implicit
  coercions, so it keeps its current meaning and carries the `from` tier.
  `ExprScalarTryConvert` and `ExprScalarFitConvert` are separate kinds rather
  than a payload, leaving the existing coercion paths untouched.
- `from`, and `fit` for every pair except float-to-integer, lower to Go `T(x)`.
- `try` and saturating `fit` call helpers in `runtime/convert.go`, registered
  in `runtime/embed.go`. These are functions, not new runtime types.
- Helpers are generic over the target with `~` constraints so foreign named
  scalars participate, and they take the target's **bit width** rather than its
  bounds. A platform-sized target has no constant bounds the backend could
  emit, so it passes `math/bits.UintSize` and the helper derives the range with
  integer arithmetic.

### Float range checking is not a round trip

Two traps, both of which the implementation must avoid:

- `float64(math.MaxInt64)` rounds **up** to exactly 2⁶³, so `x <= float64(
  math.MaxInt64)` wrongly accepts 2⁶³. Upper bounds must be strict against the
  power of two (`x < 2⁶³`), not `<=` against the type's maximum. Same for
  `MaxUint64` → 2⁶⁴ and `float32(MaxInt32)` → 2³¹.
- `int64(float64(x)) == x` is unsound as an exactness test: for
  `x == math.MaxInt64`, `float64(x)` is 2⁶³ and `int64(2⁶³)` is
  implementation-defined — amd64 wraps to `MinInt64` while arm64 saturates to
  `MaxInt64`, giving opposite answers. Range-guard before converting back, and
  never execute a Go float→int conversion on an unguarded value. Defining
  integer → float as `fit`-only (above) removes the need for this test
  entirely.

Bounds are derived from the bit width with integer arithmetic (`int64(-1) <<
(bits - 1)`, `^uint64(0) >> (64 - bits)`) rather than from float constants, so
the maximum is exact. Float comparison bounds use `math.Ldexp(1, bits)`, which
is the exact power of two.

## Test plan

**Checker** — table-driven over source/target pairs asserting which tiers
compile and which diagnostic the others produce; identity pairs including
foreign named scalars in both directions; literal and constant-expression
arguments; `Rune::fit` and `Str::try` rejection; each removed method's
suggestion. Existing `checker/scalar_from_test.go`,
`checker/byte_rune_test.go`, `checker/diagnostics_test.go:731`, and
`checker/go_import_test.go:1501` need updating.

**Go target** — executable tests for:

- `try` integer→integer at both endpoints in range, one past each endpoint out
  of range, and signedness (`Uint8::try(-1)`).
- `try` float→integer at **2³¹, 2⁶³, and 2⁶⁴ exactly**, which are representable
  as floats while the corresponding type maxima are not; plus `NaN`, `±Inf`,
  `-0.0`, and fractional values.
- `try` `Float64 → Float32` overflow to `±Inf` (`none`), rounding (`some`),
  subnormal underflow (`some`), and `NaN`/`±Inf` passthrough (`some`).
- `try` into `Rune` for valid scalars, surrogates, negatives, and `> 0x10FFFF`.
- `fit` integer wrap and float→integer saturation at the same boundaries,
  `NaN` → `0`, `-0.0` → `0`.
- `from` preserving signed zero, `±Inf`, and `NaN` for `Float32 → Float64`.
- Identity `from` on foreign named scalars emitting a no-op conversion.

**Formatter** — no syntax change; verified that `Int::try(x)`,
`try Int::try(x).ok_or(..)`, `try Int::try(x) -> e {..}`, and
`match Int::try(x) {..}` already parse and format idempotently.

**Tree-sitter** — verified `Int::try(...)` parses with zero ERROR nodes as a
`qualified_identifier`, and `highlights.scm:16` scopes the `try` keyword to
`try_expression`, so no grammar or query change is required.

**Migration** — `examples/vaxis-demo/main.ard:1006`; `compiler/go/
scalar_from_test.go:32,40,48`, `go/backend_test.go:6033,6035`,
`go/byte_rune_test.go:14,30,50`. `go/embed_test.go`'s `Byte::from(0)` literals
stay valid. Std-lib `.ard` files and `compiler/samples` contain no uses.

**Docs** — `website/src/content/docs/advanced/go-interop.md:147` and
`:176-205` describe `from` as truncating and use `Uint32::from(count)`, which
becomes `Uint32::fit(count)`; `time::Duration::from(ms)` at `:188` stays valid
under the identity rule. Add a conversion section to `guide/types.md` covering
the three tiers and the platform-sized asymmetry, which the website currently
does not document at all.

## Consequences

- One spelling per conversion, with the target type as the namespace, so
  discovery is "what does `Int::` offer".
- Accidental narrowing is a compile error. Code that intends narrowing says so
  with `fit`; code that needs a guarantee gets `try`.
- Float→int overflow and `Float64 → Float32` overflow become defined behavior
  rather than Go's implementation-defined result.
- **`Int → Float64` requires `fit`.** Because `Int` may be 64 bits, the most
  common conversion in ordinary numeric code is a lossy tier —
  `Float64::fit(width - 1)` where today it is `.to_f64()`. This is honest
  (Rust's `i64` has no `From<f64>` either) and is accepted deliberately, but it
  does dilute the "lossy spellings announce themselves" signal, and it is the
  most visible cost of this ADR.
- `Rune` gains a guarantee it did not previously have: no conversion can
  produce an invalid scalar, and `Rune` widens losslessly everywhere its range
  fits.
- Breaking: narrowing `T::from` calls and four methods stop compiling. Every
  failure carries a suggestion naming the replacement.
- `try` is also the error-propagation keyword, so `try Int::try(x).ok_or(..)`
  carries two meanings on one line. Verified that the parser, formatter, and
  tree-sitter grammar all handle it today with no changes; the cost is
  readability, not implementation.

## Related

- #500, #284, #283
- ADR 0026 (Byte/Rune primitives — prelude-module section superseded, `Rune`
  invariant retained)
- ADR 0031 (Go backend lowering contract — conversion paragraph amended)
- ADR 0034 (std-lib reset)
