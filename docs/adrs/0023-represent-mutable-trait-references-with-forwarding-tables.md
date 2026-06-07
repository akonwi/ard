# 0023: Represent Mutable Trait References with Forwarding Tables

## Status

Accepted

## Context

ADR 0022 defines `mut T` in type positions as a mutable reference. For concrete types, backends can usually represent that reference with a pointer, cell, or equivalent stable storage handle.

Trait objects need a different representation. A trait value is intentionally opaque: callers know the trait's methods, but not the concrete storage shape. A mutable trait reference therefore should not be modeled as a pointer to an interface-typed temporary. That representation can preserve mutations during a single call with copy-in/copy-out, but it is unsound when the mutable reference escapes.

For example:

```ard
trait View {
  fn value() Int
}

struct Leaf {
  n: Int,
}

impl View for Leaf {
  fn value() Int { self.n }
}

struct Node {
  view: mut View,
}

fn store(mut child: View) Node {
  Node{view: child}
}

mut leaf = Leaf{n: 0}
let node = store(leaf)
leaf.n = 7
node.view.value() // should observe 7
```

If `store(leaf)` creates a temporary interface value and stores a pointer to that temporary, `node.view` observes stale copied state rather than the current `leaf` storage. This violates mutable-reference aliasing semantics.

## Decision

Represent `mut Trait` as a trait-specific forwarding table over an underlying storage location, not as a pointer to an interface value.

For each trait used in a mutable-reference position, the backend should be able to produce a stable trait-shaped reference whose operations forward to the original mutable storage. Conceptually:

```go
type mutViewRef struct {
  load   func() any      // backend-private: read the current trait-shaped value
  assign func(value any) // backend-private: assignment through the mutable reference
  value  func() int      // trait method forwarder
}
```

The visible capability is still trait-specific method forwarding. Backend-private load/assign hooks are only for whole-value operations required by mutable-reference semantics, such as assigning through a `mut View` slot or coercing a `mut View` back to an immutable `View` value.

For a concrete mutable `Leaf` place, the forwarding table captures the original storage location:

```go
ref := mutViewRef{
  load:   func() any { return leaf },
  assign: func(value any) { leaf = value.(Leaf) },
  value:  func() int { return leaf.value() },
}
```

For trait-typed storage, the method forwarders should dispatch against the current stored value each time so the storage can contain different concrete implementations over time.

Mutating trait methods should forward writes to the same original storage. Non-mutating trait methods should read the current value from that storage. Assignment through the mutable trait reference should update the captured storage when the storage can accept the assigned value. Trait-typed storage can accept any value for that trait. Concrete-backed storage can only accept values whose current concrete representation is assignable to that concrete storage type; assigning a different implementation through such a reference is invalid and may be rejected statically or trapped by the backend. If the forwarding table escapes through a return value, field, or closure, its captured storage must remain stable according to the existing mutable-reference escape rules.

Immutable trait objects may continue to use the backend's ordinary trait-object representation. The special forwarding-table representation is for mutable trait references (`mut Trait`) where alias preservation matters.

## Consequences

- Escaped `mut Trait` fields preserve aliasing with the original concrete mutable value.
- Mutations through the original concrete value are visible through escaped mutable trait references.
- Mutations through the mutable trait reference update the original concrete value.
- Assignment through a mutable trait reference updates the captured storage rather than replacing the forwarding table itself.
- Whole-value assignment through concrete-backed `mut Trait` references is limited to values compatible with the concrete storage type.
- Backends need trait-specific mutable-reference representations and upcast paths for concrete-to-`mut Trait` conversions.
- Copy-in/copy-out temporary interface wrappers are not a sound representation for escaping mutable trait references.
- This decision is an implementation strategy for ADR 0022's mutable-reference semantics; it does not change ordinary immutable trait-object behavior.

## Related

- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `compiler/go`
- `compiler/javascript`
