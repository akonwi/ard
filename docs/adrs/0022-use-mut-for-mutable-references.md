# 0022: Use `mut` for Mutable References

## Status

Proposed

## Context

Ard currently uses `mut` for two related but different ideas:

- a local binding may be reassigned or mutated;
- a function parameter or call argument marked `mut` requests a mutable local copy of the value.

This copy-oriented rule makes mutation explicit and avoids accidental mutation of shared data, but it also makes `mut` read like a reference while behaving like a clone. For example:

```ard
fn update_person(mut person: Person) {
  person.age = 99
}

let alice = Person{name: "Alice", age: 30}
update_person(mut alice) // today this mutates only a copy
```

That behavior is surprising for APIs that intend to mutate caller-owned state, and it means `mut` is not a representation boundary. A `mut Context`, `mut ViewTree`, or `mut Node` position cannot currently be used to break a recursive value-layout cycle because the language still models the value as an owned copy rather than an explicit reference.

Retained UI and runtime-capability APIs are a motivating case. A framework may want a context or tree value to be passed around as a mutable capability with identity, not copied into each function or closure. In those designs, mutable reference positions should have finite representation, similar to pointers/references/interfaces in target languages.

Recursive type references are another motivating case. ADR 0020 supports self-recursive struct fields through existing indirection forms such as lists, maps, and nullable values, but broader mutually recursive type groups still need a finite representation boundary. If `mut T` is an explicit mutable reference, it can serve as that boundary for recursive graphs where identity and mutation are intended.

Ard still needs explicit copy semantics. If `mut` becomes reference-oriented, the language needs another visible way to ask for an independent value copy when that is the intended behavior.

## Decision

Explore changing `mut` from copy semantics to explicit mutable-reference semantics.

Under this direction, a `mut` parameter, function-type parameter, receiver, closure capture, or field is a mutable reference to a value of the annotated type rather than an owned copied value. Call sites do not need a second `mut` marker when passing an already-mutable addressable place; the parameter type expresses the mutable-reference requirement and the checker enforces it:

```ard
fn update_person(mut person: Person) {
  person.age = 99
}

mut alice = Person{name: "Alice", age: 30}
update_person(alice) // mutates alice
```

A mutable reference has finite representation and is a sizedness boundary. Recursive type analysis should treat `mut T` as an indirection edge, allowing mutually recursive type references when every cycle crosses such a boundary:

```ard
struct A {
  b: mut B,
}

struct B {
  a: mut A,
}
```

The checker should still reject inline value cycles that do not cross a finite boundary:

```ard
struct A { b: B }
struct B { a: A }
```

Introduce an explicit copy operation for independent value copies. Prefer making this a standard-library operation, likely `ard/core::copy`, instead of a new expression keyword in the first design:

```ard
use ard/core

fn updated_person(person: Person) Person {
  mut updated = core::copy(person)
  updated.age = 99
  updated
}
```

`core::copy` should mean a deep Ard value copy matching today's copy semantics. The exact implementation strategy for generic deep copy can be deferred, but the user-facing meaning should be explicit: use `core::copy(value)` when identity should be broken.

Additional sugar, such as a copy parameter modifier, can be considered later, but copies should remain visible at the call site or in the function body so users can see where identity is intentionally broken.

Local mutable bindings still declare mutable local storage. The key semantic change is that `mut` no longer means “clone this argument for mutation” in parameter/call positions. Assignments or bindings that need an independent deep copy should use `core::copy` explicitly.

Mutable references may alias. If two `mut T` references are created from the same mutable place, both point to the same storage and mutation through either reference is visible through the other:

```ard
mut alice = Person{name: "Alice", age: 30}

let a: mut Person = alice
let b: mut Person = alice

a.age = 31
b.age = 32
// alice.age, a.age, and b.age are all 32
```

This is intentional. Ard should rely on explicit mutable-reference types rather than Rust-style exclusive mutable borrows for the first reference model.

Mutable references may escape ordinary lexical scopes through return values, struct fields, and closures. An escaping mutable reference requires stable storage; backends must heap-lift or box the referenced storage as needed so the reference cannot dangle:

```ard
struct Context {
  tree: mut ViewTree,
}

fn make_context() Context {
  mut tree = ViewTree{nodes: []}
  Context{tree: tree} // tree is lifted so Context.tree remains valid
}
```

Creating a mutable reference still requires an addressable mutable place. Passing immutable bindings, literals, or non-reference return values to `mut T` parameters should be rejected unless the language later defines a stable temporary/reference-return rule.

Fiber boundaries are different from ordinary lexical escape. Mutable references should not be captured by `async::start`, `async::eval`, or other cross-fiber closures until Ard defines explicit concurrency safety rules for shared mutable references.

## Consequences

- This is a breaking language change. Existing code that relied on `mut` parameters receiving defensive copies must migrate to explicit `core::copy`.
- Code that already intended mutable-reference behavior becomes more direct and less surprising.
- `mut T` becomes a representation boundary for recursive layout and same-module recursive type groups, unblocking retained graphs and cyclic object models that require identity.
- `mut T` is sufficient for the first reference feature. Do not add a separate immutable reference form as part of this decision; ordinary values remain the read-only/default access mode.
- Use `field: mut T` for mutable-reference fields and analogous type-position syntax for other reference positions.
- The checker must distinguish ordinary values from mutable references, require mutable-reference calls to pass addressable mutable places, and reject passing immutable bindings where mutation could occur.
- Explicit mutable-reference syntax is sufficient for the first safety model. Do not require Rust-style exclusive mutable borrows as part of this decision.
- Multiple mutable references may point to the same storage; mutations through one alias are visible through the others.
- Escaping mutable references require stable storage. The compiler/backends must heap-lift or box referenced locals when mutable references escape through returns, struct fields, or closures.
- AIR needs first-class mutable-reference types and operations such as reference load, field mutation through a reference, and reference capture.
- Backends need finite mutable-reference representations. Go can generally use pointers or pointer-backed wrappers; JavaScript can use object cells or object references where necessary.
- Async/fiber rules need to stay conservative. Mutable references should not be captured by fibers unless Ard defines safe sharing rules for those references.
- Documentation must be updated to replace the current “`mut` means copy” explanation with “`mut` means mutable access/reference” and separate explicit `core::copy` semantics.

## Deferred Work

- Define the implementation strategy and type coverage for generic deep copying in `ard/core::copy`.
- Define concurrency rules for mutable references that cross fiber boundaries.

## Related

- `docs/adrs/0020-support-recursive-struct-fields-through-indirection.md`
- `docs/adrs/0003-use-generic-fibers-for-async-eval.md`
- `README.md`
- `compiler/checker`
- `compiler/air`
- `compiler/go`
- `compiler/javascript`
