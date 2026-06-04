# 0020: Support Recursive Struct Fields Through Indirection

## Status

Accepted

## Context

Ard currently resolves struct field types while the struct being declared is not yet available as a type name. This prevents intuitive recursive data structures such as trees:

```ard
struct Node {
  children: [Node],
}
```

The checker reports `Unrecognized type: Node` for the field type. This is an implementation limitation rather than a durable language rule users should have to model around.

Recursive value types also need a sound representation rule. A direct inline field such as:

```ard
struct Node {
  child: Node,
}
```

has infinite size in value-oriented targets such as Go and should not be accepted as ordinary inline storage. In Go, the equivalent `child Node` field is invalid, while recursive references through fixed-size descriptors such as slices, maps, pointers, or interfaces are valid.

Ard already has source-level type constructors that can act as representation boundaries for common recursive shapes:

```ard
struct Node {
  parent: Node?,
  children: [Node],
  children_by_id: [Str:Node],
}
```

Lists and maps are naturally indirect/container-backed. Nullable recursive fields should also be treated as an indirection boundary so parent pointers and linked structures can be expressed without requiring a separate user-visible `Box` or `Ref` type for common cases.

This decision is separate from broader declaration-order cycles between different type declarations, such as a `Node` field containing `[View]` while the `View` trait also mentions `Node`. Those cycles are out of scope for this decision. The immediate goal is sound support for explicit self-reference in struct fields.

## Decision

Allow a struct declaration to reference its own type name within its field types.

Accept self-recursive field references when the recursive occurrence is beneath one of Ard's supported indirection boundaries:

- list values, for example `[Node]`
- map values, for example `[Str:Node]`
- nullable values, for example `Node?`

Reject direct inline self-recursive fields because they have infinite value size:

```ard
struct Node {
  child: Node, // invalid
}
```

The checker should produce a clear diagnostic explaining that the recursive field has infinite size and should be placed behind a supported indirection boundary such as a list, map, or nullable field:

```text
Recursive field Node.child has infinite size. Put the recursive reference behind a list, map, or nullable type.
```

Recursive nullable fields are part of the supported surface, not a follow-up feature. Backends must represent nullable self-recursive fields with finite storage. For the Go target, recursive nullable struct fields should lower as pointer-backed optional fields, such as `*Node`, instead of inline `Maybe<Node>`. The backend must adapt source-level `Maybe` operations on those fields so `none` maps to `nil`, `some(value)` stores an allocated value, absence checks compare with `nil`, and unwrapping performs a nil check before dereferencing.

Lists and maps of recursive structs should lower to the target's ordinary finite container representations where possible. On the Go target, `[Node]` can lower to `[]Node` and `[Str:Node]` can lower to `map[string]Node` because slices and maps are fixed-size descriptors that point to separately allocated storage.

Do not add user-visible forward declarations, mutually recursive declaration groups, or a first-class `Box`/`Ref` type as part of this decision. Those features may still be useful later, but self-recursive struct fields through existing indirection forms should work directly and idiomatically.

## Consequences

- Ard can model common recursive data structures such as trees and parent-linked nodes directly.
- The checker must register a struct's own type name before resolving that struct's fields, then complete the struct definition after field resolution.
- The checker must validate recursive field paths and reject inline cycles that do not pass through an accepted indirection boundary.
- Backends must preserve finite representations for all accepted recursive shapes.
- Nullable recursive fields require special care because a generic inline `Maybe<T>` representation would be infinitely recursive for `T == Node`.
- Direct recursive value fields remain invalid, preserving soundness and avoiding target compiler failures.
- This does not solve cross-type declaration-order cycles. Broader same-module type hoisting or explicit recursive declaration mechanisms are out of scope.

## Related

- `docs/adrs/0012-represent-optional-values-with-maybe.md`
- `docs/adrs/0009-support-traits-for-shared-behavior.md`
- `compiler/checker`
- `compiler/go`
