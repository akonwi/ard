# 0006: Support Chained Relative Comparisons

## Status

Accepted

## Context

Range and ordering checks are common in Ard programs. Without chained comparisons, these checks require repeating the middle expression and combining pairwise comparisons manually:

```ard
if 200 <= status && status <= 300 {
  // ...
}
```

That form is noisier and can accidentally evaluate an expression with side effects more than once. Ard values readability and left-to-right clarity, so the language should support a concise way to express relative ranges while keeping evaluation semantics predictable.

## Decision

Support chained relative comparisons in expressions.

A chained comparison such as:

```ard
if 200 <= status <= 300 {
  // ...
}
```

is equivalent to pairwise comparisons joined with logical `&&`:

```ard
if 200 <= status && status <= 300 {
  // ...
}
```

Middle operands in a chain are evaluated exactly once, even though their value participates in multiple pairwise comparisons.

Only relative comparison operators may be chained:

- `<`
- `<=`
- `>`
- `>=`

Equality operators (`==` and `!=`) must not be chained. The checker should reject equality chains rather than accepting ambiguous expressions.

Each pairwise comparison in the chain must satisfy the normal comparison type rules. Chained comparisons bind tighter than logical operators, so:

```ard
200 <= status <= 300 && status != 250
```

is interpreted as:

```ard
(200 <= status <= 300) && (status != 250)
```

## Consequences

- Range checks become more readable and direct.
- The language must preserve single-evaluation semantics for middle operands.
- The parser/checker need an explicit representation or lowering path for chained comparisons.
- Targets can lower the checked form as ordinary pairwise comparisons joined by logical `&&`.
- Equality chains remain invalid, keeping comparison semantics simple and avoiding unclear intent.

## Related

- `docs/adrs/0001-record-architecture-decisions.md`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
