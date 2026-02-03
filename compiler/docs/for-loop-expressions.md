# For Loop Expressions (Deferred)

This document records the decision to defer adding `for` loops as expressions.

## Proposal Summary

The idea was to allow `for` loops to return a list, similar to a comprehension:

```
let doubled: [Int] = for i in 1..10 { i * 2 }
```

Key properties discussed:
- The loop would evaluate to a list of body results.
- `break` would end the loop and return a partial list.

## Decision

We are not pursuing this feature right now.

## Reasons

- Allowing `for` as a full expression introduces usage in arbitrary expression positions.
- Restricting usage to assignment or similar contexts is a special-case rule.
- Either approach complicates parsing and type checking in ways that are not worth it right now.

## Revisit Notes

If reconsidered later, clarify the following first:

- Whether `for` expressions are allowed in all expression positions.
- Whether `break` can carry a value or only short-circuits list collection.
- How the checker infers the element type when the result is not explicitly typed.
