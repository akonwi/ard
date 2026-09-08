# 0070: Canonicalize AIR Type Identity

## Status

Accepted

## Context

AIR is the typed backend boundary after checking (ADR 0002). Expressions and
type metadata refer to compact `TypeID`s, and backend caches and validation rely
on one ID denoting one semantic type identity.

Type interning currently has two paths:

- checker-derived types use recursively rendered checker-type keys;
- types synthesized during lowering use keys built from child `TypeID`s.

Those keys do not share an identity domain. The same `Result<Void, Error>` can
therefore be entered once as `result<Void,Error>` and again as `result:1:10`.
Issue #466 exposes the split when a local of that type is wrapped in `Maybe`:
the payload and `Maybe` element are semantically equal but have different IDs,
so AIR validation correctly rejects the constructor.

A related problem is that contextual AIR lowering uses `Void` both as Ard's
concrete unit type and as a stand-in for an unresolved generic component.
Consequently, every `Maybe<Void>` and every `Result` containing `Void` is treated
as weak and may be re-inferred. This conflicts with ADR 0031, which explicitly
requires `Void` to remain a real value and generic argument.

Expected-type-directed lowering is still required for representation choices
such as literals, closures, foreign named values, and explicit conversions. It
must be distinguished from semantic type inference. The checker is the semantic
authority; AIR lowering should consume complete checked expression types rather
than reconstruct them.

## Decision

### Use one AIR type identity model

Every semantic type in one `Program` has exactly one canonical `TypeID`,
regardless of whether it originated in a checker node, generic substitution, or
a lowering-created composite.

Structural types are identified by their kind and canonical child IDs:

- list, slice, maybe, channel direction, and reference: kind plus element;
- fixed array: element plus length;
- map: key plus value;
- result: success plus error;
- function: ordered parameters, return, and variadicity.

Display names do not participate in structural identity. Checker-derived and
lowering-created structural types use the same narrow interner constructors.

Nominal types are identified by declaration ownership rather than shape:

- Ard declarations use canonical module and declaration identity;
- generic applications use generic definition ID plus ordered argument IDs, as
  required by ADR 0054;
- type parameters use their owning generic definition and parameter position;
- foreign named types use target, canonical package namespace, symbol,
  pointer/value form, and ordered generic arguments.

Fields, variants, visibility, source aliases, and rendered names are metadata,
not identity. Repeated observations of one nominal identity must use canonical
declaration metadata; conflicting metadata is an internal lowering error.

### Reserve only nominal identities

Structural types are interned child-first. Valid recursive structural paths in
Ard cross a nominal anchor, so no anonymous structural placeholder is needed.

Nominal definitions and applications may be recursive. Their interner entries
have an explicit lifecycle: building, complete, or failed. A building entry may
supply its reserved ID for a recursive back-edge, but semantic metadata cannot
be read until completion. A failed lowering aborts that interner; IDs are never
renumbered or reused.

Transient lifecycle state belongs to the lowerer and is not serialized in AIR.
Finalization rejects every entry that is not complete.

### Require resolved checker input for executable AIR

Every value-producing executable expression crossing the checker-to-AIR boundary
has a complete checked result type. That type is either closed or contains only
type parameters owned by the generic definition currently being lowered.

The checker represents a non-returning expression such as `panic` with an
explicit checker-only `Never` type. AIR lowering replaces that bottom type with
the surrounding expected representation, or `Void` when the expression is used
only as a statement. It cannot be confused with unresolved call inference and
never becomes an AIR `TypeID`. A non-returning expression does not invent a
source-level value type merely to satisfy the resolved-input rule.

Bound checker inference variables are dereferenced. Unresolved call-owned or
provisional inference variables are rejected before AIR is finalized.
Declaration-owned generic variables become owner-scoped AIR `TypeParam`s.

`Void` always means Ard's unit type. It is never used as an unresolved marker.
The checker may choose concrete `Void` for an unobservable generic result, but
that is a completed language-level inference decision.

AIR retains expected-type-directed representation lowering, but removes its
independent weak-type inference after checked block and expression type metadata
is preserved through lowering. In particular, discarded final expressions keep
their checked expression type even though their produced value is discarded.

No inference-hole type is added to finalized AIR. If a temporary migration
representation becomes necessary, it remains lowering-local and must be fully
resolved before type interning.

### Keep AIR validation strict

Constructor payloads, locals, signatures, and aggregate elements continue to
require canonical exact IDs where AIR promises one type identity. Validation is
not relaxed to structural equivalence to hide producer defects.

Responsibility is split at the relevant boundary:

- checker-to-AIR conversion rejects unresolved call-owned inference variables;
- interner finalization rejects building or failed nominal entries;
- `air.Validate` checks only observable finalized AIR: duplicate canonical
  identities, invalid generic relationships, and exact expression identities.

Transient interner state and checker variables are not serialized into AIR, so
`air.Validate` does not attempt to reconstruct or inspect them. Language and
foreign assignability remain separate from type identity.

## Implementation sequence

1. Characterize checker completeness and add regressions for issue #466.
2. Introduce canonical structural constructors and route both existing
   structural production paths through them.
3. Preserve checked block/expression typing, including discarded final values,
   and model non-returning expressions with an explicit checker-only bottom type
   while retaining representation-directed expected-type lowering.
4. Remove `Void`-based weak contextual inference and reject unresolved
   call-owned variables at the boundary.
5. Consolidate nominal definitions, generic applications, and owner-scoped type
   parameters behind reserve/complete interner operations.
6. Canonicalize foreign named identity separately, with generated-Go import and
   alias coverage.
7. Enable canonical uniqueness and completion validation after every producer
   has migrated.

The sequence intentionally keeps structural canonicalization reviewable and
allows existing contextual lowering to call the canonical constructors while
the checker-to-AIR contract migration is in progress.

## Consequences

- `Maybe<Result<Void, E>>`, `Result<T, Void>`, and other concrete uses of `Void`
  no longer split AIR type identity.
- Backend caches and strict validation can rely on canonical IDs instead of
  compensating with structural scans.
- Recursive nominal and generic types retain stable IDs without using incomplete
  semantic `TypeInfo` as if it were final.
- The checker/AIR responsibility boundary becomes explicit: checking decides
  types; AIR selects target-neutral representations and preserves those types.
- Type allocation order may change because structural children are interned
  first. Tests and generated names must not depend on incidental numeric IDs.
- The migration is broader than issue #466 and requires focused recursion,
  generic, foreign identity, determinism, and full backend regression coverage.

## Related

- GitHub issue #466
- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0054-represent-generic-structs-as-nominal-applications.md`
- `docs/adrs/0055-use-call-local-constraints-for-ard-generic-inference.md`
- `docs/adrs/0057-separate-binding-mutability-from-reference-values.md`
