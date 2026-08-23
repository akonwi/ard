# 0066: Project Maybe Fields in Try and Match

## Status

Accepted

## Context

Ard represents absence explicitly with `Maybe<T>`. Nested nullable struct data is
common, but inspecting it required nested matches or `and_then` callbacks. The
checker already had partial support for field chains under `try`, but it did not
support the same chain as a `match` subject. It also gave required fields the
bare field type instead of wrapping them in `Maybe`, and its absent branch could
re-evaluate the receiver with an unrelated Maybe type.

Unconditionally allowing `maybe.field` would make ordinary member access
implicitly type-directed. The desired scope is narrower: null-propagating field
chains are useful at the two constructs which explicitly consume absence,
`try` and `match`.

## Decision

Allow a field chain through `Maybe<T>` when the chain is directly consumed as:

- the operand of `try`; or
- the subject of `match`.

For an Ard-owned struct field `T.field`:

- if the field type is `U`, projecting through `T?` produces `U?`;
- if the field type is already `U?`, the result remains `U?` rather than
  becoming `U??`.

Projection propagates an absent receiver and evaluates each receiver expression
at most once. Chained projections apply the same rule at every Maybe layer.
Required fields are wrapped as present values only after the receiver is known
to be present.

Ordinary field access through Maybe remains invalid:

```ard
let name = user.profile.name // invalid when user or profile is Maybe
```

Callers outside `try` and `match` should use an explicit `match`, `map`, or
`and_then`.

The initial feature covers fields on Ard-owned structs, including specialized
generic structs and Maybe values observed through Ard mutable references. The
field's outer optionality must be concrete while its generic function body is
checked. A direct unresolved field type such as `$T` is rejected because the
checker cannot soundly decide whether specialization should wrap `$T` or flatten
`$T?`. It does not implicitly dispatch methods on the contained value. Existing
methods on Maybe, such as `is_some`, `map`, and `and_then`, retain their ordinary
meaning.

The checker represents each propagation step explicitly as an `OptionMatch`.
The present branch reads the field and either wraps it in `some` or returns its
existing Maybe value. The absent branch constructs `none` of the projected
result type. AIR therefore receives explicit, target-neutral control flow; a
backend does not infer optional chaining from a field access.

## Consequences

- `try user.profile.name` and `match user.profile.name` have consistent
  null-propagating behavior.
- Match bindings receive the unwrapped field value, including for required
  fields projected as Maybe.
- No parser, formatter, or Tree-sitter syntax changes are required.
- Field assignment and borrowing rules do not change because projection is only
  introduced in read contexts.
- Maybe values reached through mutable references are observed without changing
  the reference or mutation model.
- Generic field types whose optionality is unresolved receive an explicit
  diagnostic instead of changing meaning across specializations.
- Foreign-field and contained-method projection can be considered separately
  without weakening this feature's semantics.
- Tooling must associate projected field tokens with the contained Ard struct's
  field declaration.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0005-use-result-maybe-and-try-for-error-handling.md`
- `docs/adrs/0012-represent-optional-values-with-maybe.md`
- `docs/adrs/0024-preserve-maybe-semantics-in-go-lowering.md`
- `docs/adrs/0057-separate-binding-mutability-from-reference-values.md`
