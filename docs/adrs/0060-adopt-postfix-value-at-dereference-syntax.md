# 0060: Adopt Postfix Value-At Dereference Syntax

## Status

Accepted

Amended by ADR 0061: `mut Trait` is no longer represented by a Go pointer layer
and is therefore not a valid `.@` operand. Mutable-to-ordinary trait conversion
is an ordinary representation-free source conversion.

## Context

ADR 0057 introduced `deref expression` as Ard's explicit operation for
materializing the value behind a reference. Its semantics are sound, but the
prefix keyword composes awkwardly with Ard's call and member syntax:

```ard
deref reference.field
(deref reference).field
deref load()
```

The reader must use precedence or parentheses to distinguish dereferencing a
reference-valued field from selecting a field on the materialized value.
Repeated one-layer dereferences are similarly verbose.

Postfix dereference has established precedent. Zig uses `pointer.*`, Odin and
the Pascal family use postfix `^`, Nim uses `pointer[]`, Ada uses
`pointer.all`, and Swift exposes `pointer.pointee`. A postfix operation matches
Ard's left-to-right call and member chains.

The candidates considered most closely were:

- `reference.*`, following Zig;
- `reference.@`, read as “the value at reference”;
- `reference.~`, suggesting removal of one indirection layer.

`.*` is familiar, but commonly implies an addressable pointee and has an
unrelated pointer-to-member meaning in C++. `.@` is distinctive and directly
expresses Ard's value-at operation. Ard has no source expression for writable
whole-referent access, so the new syntax does not need to denote an assignment
place.

## Decision

The canonical explicit dereference syntax is postfix `.@`:

```ard
let reference = mut user
let snapshot = reference.@
```

`reference.@` has **exactly** the semantics assigned to `deref reference` by
ADR 0057. This ADR changes syntax only. The operation:

- accepts only an actual reference value;
- removes exactly one outer reference layer;
- evaluates its operand once;
- produces a shallow, non-place value;
- preserves existing trait, generic, FFI, and foreign nil-pointer behavior;
- remains distinct from compiler-defined observational reads.

### Grammar and composition

`.@` is a dedicated postfix operator at the same precedence as calls and member
access. Postfix operations compose from left to right:

```ard
reference.@.field // materialize, then select the field
reference.field.@ // select a reference-valued field, then materialize
reference.@.@     // materialize two reference layers
load().@          // call, then materialize the returned reference
```

It composes with the prefix `mut` reference expression as follows:

```ard
(mut value).@ // create a reference, then materialize its value
mut reference.@ // materialize, then reference fresh top-level storage
```

The `.` and `@` must be adjacent. A newline may precede `.@` through Ard's
existing dot-leading chain formatting:

```ard
let name = reference
  .@
  .profile
  .name
```

Bare `@` remains invalid and reserved. This decision does not establish a
general family of dot-sigil operations.

### Assignment behavior

The postfix spelling does not make dereferencing addressable:

```ard
let snapshot = reference.@       // allowed
reference.@ = replacement        // rejected
reference.@.name = "Grace"      // rejected: the materialized value is a temporary
```

Ard source continues to expose interior mutation through reference operations,
not whole-referent assignment through a dereference expression.

### Migration

The syntax changes over two minor releases:

1. In the introduction release, both `reference.@` and `deref reference` parse
   to the same dereference expression. Prefix `deref` produces a deprecation
   warning. The formatter always emits `.@`, providing an automatic migration.
2. In the following minor release, prefix `deref` is removed and `deref`
   becomes available as an ordinary identifier.

The formatter preserves operation order while migrating precedence-sensitive
forms:

```ard
deref reference.field   // becomes reference.field.@
(deref reference).field // becomes reference.@.field
deref mut value         // becomes (mut value).@
mut deref reference     // becomes mut reference.@
```

During the transition, `deref` remains reserved in expression-level identifier
positions. Existing member names such as `reader.deref()` remain valid.

## Consequences

- Explicit materialization reads left to right with calls and member access.
- One-layer and repeated dereferences are visually concise.
- The unique `.@` sigil communicates “value at” without promising an
  addressable place.
- Existing code has a formatter-driven migration path and one release of
  compatibility.
- The lexer, parser, formatter, Tree-sitter grammar, highlighting, diagnostics,
  documentation, samples, and downstream code must agree on the new canonical
  spelling.
- A later release must remove the compatibility parser branch, deprecation
  warning, and `deref` keyword reservation.

## Alternatives Considered

### Zig-style `.*`

This has the strongest direct postfix-dereference precedent and similarly good
chaining behavior. `.@` was selected because it is unique to Ard's value-at
semantics and avoids expectations of writable pointee access.

### Postfix `^`

Odin, Pascal, Delphi, and Oberon use this form. It is compact, but integrates
less clearly with Ard's dot-led postfix chains and consumes a symbol commonly
used for XOR.

### `.~`

This is grammatically clean and can suggest peeling away a reference layer, but
it has little dereference precedent and is commonly associated with complement
or destruction.

### Named `.value` or `.pointee`

A named property is explicit but verbose and resembles an ordinary member that
could conflict with user APIs. A dedicated operator better communicates a
language-level operation.

## Related

- `docs/adrs/0045-support-explicit-mutable-reference-expressions.md`
- `docs/adrs/0057-separate-binding-mutability-from-reference-values.md`
