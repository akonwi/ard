# 0010: Support Dollar-Prefixed Generics

## Status

Accepted

## Context

Ard needs reusable functions, methods, and data structures that can work across multiple concrete types while preserving static type safety. The language also aims to keep declarations lightweight and readable, avoiding verbose generic parameter lists when type variables are already visible in signatures.

Generic inference is especially important for collection helpers and anonymous callbacks, where requiring every callback parameter type would make common code noisy.

## Decision

Support generic type parameters using a `$` prefix, such as `$T` and `$U`.

Generic parameters are introduced by appearing in function, method, or struct signatures. They do not require a separate generic parameter declaration list.

Examples:

```ard
fn map(list: [$A]) [$B] {
  // ...
}

struct Box { item: $T }
struct Pair { first: $T, second: $U }
```

Callers may provide explicit type arguments when needed:

```ard
let floats = map<Int, Float>(ints)
let box: Box<Int> = Box{ item: 42 }
```

When explicit type arguments are omitted, the checker should infer generic parameters from usage context, argument values, struct literals, declared variable types, and callback positions.

Generic type parameters may stand for any valid Ard type, including primitives, lists, maps, function types, nested generic instantiations, `Maybe`, and `Result`.

Each generic function or method call should use fresh type variables for that call site, so inference and refinement do not mutate the original definition or leak between calls. Within a call, generic bindings may be refined as arguments are checked, allowing later arguments and anonymous callback parameters to observe earlier inferred types.

Conflicting bindings for the same generic parameter must be rejected.

## Consequences

- Generic declarations stay concise because `$T` in a signature is enough to introduce a type parameter.
- Common higher-order functions can infer anonymous callback parameter types from earlier arguments.
- The checker must support per-call fresh generic variables and consistent type refinement.
- Backends should receive concrete specialized types after checking/AIR lowering rather than open generic definitions where executable code requires concrete types.
- Future generic constraints or trait bounds can build on the `$` generic parameter model without changing the basic syntax.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0009-support-traits-for-shared-behavior.md`
