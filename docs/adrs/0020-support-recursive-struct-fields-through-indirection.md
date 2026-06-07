# 0020: Support Recursive Struct Fields Through Indirection

## Status

Accepted

## Context

Ard needs to support recursive data structures without requiring users to write manual forward declarations or opaque wrapper types for common cases. A tree type should be able to mention its own type name in a field:

```ard
struct Node {
  children: [Node],
}
```

Recursive value types also need a sound representation rule. A direct inline field such as:

```ard
struct Node {
  child: Node,
}
```

has infinite size in value-oriented targets such as Go and should not be accepted as ordinary inline storage. In Go, the equivalent `child Node` field is invalid, while recursive references through fixed-size descriptors such as slices, maps, pointers, or interfaces are valid.

Ard has source-level type constructors that can act as representation boundaries for recursive shapes:

```ard
struct Node {
  parent: Node?,
  children: [Node],
  children_by_id: [Str:Node],
}
```

ADR 0022 extends this set by making `mut T` an explicit mutable-reference representation. That means `mut T` is also a sizedness boundary:

```ard
struct Node {
  parent: mut Node,
}
```

The same rule generalizes from self-recursive structs to same-module recursive type groups. The checker hoists same-module type names before resolving fields and signatures, then rejects only cycles whose value layout remains inline all the way around.

## Decision

Allow same-module type declarations to reference each other and themselves in type positions without user-written forward declarations.

Accept recursive references when every cycle crosses at least one finite representation boundary. Supported boundaries include:

- mutable references, for example `mut Node`
- list values, for example `[Node]`
- map values, for example `[Str:Node]`
- direct nullable fields, for example `Node?`
- trait object positions, for example `View`
- extern/opaque types
- function descriptors, for example `fn() Node`

Map keys do not count as an indirection boundary for recursive layout. A recursive map key can make the map key type itself infinitely recursive or otherwise invalid for the target, so those cycles should still be rejected.

Reject direct inline recursive fields because they have infinite value size:

```ard
struct Node {
  child: Node, // invalid
}
```

Also reject mutually recursive groups with no boundary:

```ard
struct A {
  b: B,
}

struct B {
  a: A,
}
```

The checker should produce a clear diagnostic explaining that the recursive field has infinite size and should be placed behind a supported indirection boundary:

```text
Recursive field Node.child has infinite size. Put the recursive reference behind mut, list, map, nullable, trait, extern, or function indirection.
```

Recursive nullable fields are part of the supported surface when the nullable value is directly stored as the recursive struct field. Backends must represent those fields with finite storage. For the Go target, recursive nullable struct fields should lower as pointer-backed optional fields, such as `*Node`, instead of inline `Maybe<Node>`. The backend must adapt source-level `Maybe` operations on those fields so `none` maps to `nil`, `some(value)` stores an allocated value, absence checks compare with `nil`, and unwrapping performs a nil check before dereferencing.

Nested nullable references inside inline containers such as `Result` or union payloads are not accepted as layout-breaking until backends can represent those nested positions with finite storage.

Lists and maps of recursive structs should lower to the target's ordinary finite container representations where possible. On the Go target, `[Node]` can lower to `[]Node` and `[Str:Node]` can lower to `map[string]Node` because slices and maps are fixed-size descriptors that point to separately allocated storage.

Do not add user-visible forward declarations or a first-class `Box`/`Ref` type as part of this decision. Existing Ard type syntax should work directly and idiomatically for supported recursive layouts.

## Consequences

- Ard can model common recursive data structures such as trees, parent-linked nodes, retained UI trees, and recursive capability graphs directly.
- The checker must hoist same-module type names before resolving fields, trait method signatures, enum declarations, extern types, and aliases.
- The checker must validate recursive type groups and reject inline cycles that do not pass through an accepted representation boundary.
- `mut T` is a mutable-reference boundary for recursive layout, matching ADR 0022.
- Backends must preserve finite representations for all accepted recursive shapes.
- Direct nullable recursive struct fields require special care because a generic inline `Maybe<T>` representation would be infinitely recursive for `T == Node`.
- Direct recursive value fields and boundary-free mutual cycles remain invalid, preserving soundness and avoiding target compiler failures.
- User-visible forward declarations are unnecessary for same-module recursive type groups.

## Related

- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0012-represent-optional-values-with-maybe.md`
- `docs/adrs/0009-support-traits-for-shared-behavior.md`
- `compiler/checker`
- `compiler/go`
