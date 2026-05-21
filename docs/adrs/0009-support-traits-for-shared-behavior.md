# 0009: Support Traits for Shared Behavior

## Status

Accepted

## Context

Ard needs a way to express shared behavior across otherwise unrelated types. Without a trait-like abstraction, functions that need behavior such as string conversion would either be limited to concrete types or require ad hoc dynamic handling.

Ard's language goals favor static checking and explicit interfaces. Rust-style traits provide a familiar model for grouping required method signatures and allowing types to implement those behaviors.

## Decision

Support traits as named sets of method requirements that types can implement.

A trait declares behavior by listing method signatures:

```ard
trait String {
  fn to_str() Str
}
```

A concrete type implements a trait with an `impl Trait for Type` block:

```ard
impl String for Person {
  fn to_str() Str {
    "{self.name} is {self.age.to_str()}"
  }
}
```

Traits may be used as function parameter types when a function only depends on the behavior described by the trait:

```ard
fn debug(thing: String) {
  io::print(thing.to_str())
}
```

The checker should validate that trait implementations provide the required methods with compatible signatures and that values passed to trait-typed parameters implement the required trait.

## Consequences

- Shared behavior can be expressed statically without tying functions to concrete types.
- Standard library conventions such as string conversion can be represented as trait requirements.
- The checker must track trait definitions, implementations, and conformance.
- Backends need a lowering strategy for trait-typed values and method dispatch.
- Trait support should remain focused on explicit behavior requirements rather than becoming implicit structural typing by default.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
