# 0011: For Loop Expressions

## Status

Deferred

## Context

Ard supports statement-style `for` loops. A proposed extension would allow `for` loops to be used as expressions that produce a list, similar to comprehensions:

```ard
let doubled: [Int] = for i in 1..10 { i * 2 }
```

Under that proposal, the loop body would contribute values to the resulting list, and `break` might end the loop and return a partial list.

However, expression-position loops introduce questions about where `for` expressions are allowed, how `break` behaves, and how the checker infers the resulting element type when the destination type is not explicit.

## Decision

Do not support `for` loops as expressions for now.

`for` remains a statement/control-flow construct. Ard should not add comprehension-like `for` expressions until the language has a clearer design for expression placement, `break` semantics, and result type inference.

If this is reconsidered later, the design must decide:

- whether `for` expressions are allowed in all expression positions or only restricted contexts
- whether `break` can carry a value or only short-circuits list collection
- how the checker infers the element type when the result is not explicitly typed
- whether a dedicated comprehension syntax would be clearer than overloading `for`

## Consequences

- The parser and checker avoid special cases for loop-valued expressions.
- Ard keeps loop control flow simple and statement-oriented.
- Programs that need mapped lists should use explicit list construction or library helpers instead of `for` expressions.
- A future comprehension feature remains possible, but should be designed deliberately rather than implied by existing `for` syntax.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
