# Unreleased — draft release notes

Draft notes staged for the next version that ships ADR 0057. Fold into the
GitHub release when the version tag is cut (see `.agents/skills/release-notes`),
then remove or reset this file.

## Overview

`mut` no longer means three things at once. Binding reassignability, reference
values, and interior mutation are now separate, explicit concepts, and a new
`deref` expression materializes a shallow value from a reference. This is a
breaking change with a mechanical migration path.

## New Features

### Explicit mutable references and `deref`

- `mut name = …` declares a reassignable binding slot.
- `mut expression` creates or copies a mutable **reference** to storage.
- `deref expression` produces a shallow value from a reference.

```ard
let user = User{name: "Ada"}
let reference = mut user     // a reference; mutations are shared
reference.name = "Grace"
let snapshot: User = deref reference   // an independent shallow value
```

References compare by pointer identity; compare referents with `deref`.

### Contextual typing for `mut` container literals

A concrete reference destination types a fresh empty container literal, so no
separate annotation is needed:

```ard
let rows: mut [Int] = mut []
let items = mut List::new<Int>()   // via the List prelude alias
```

## Improvements

### Go-like async reference sharing

Task closures passed to `async::start` follow ordinary closure capture rules and
may share and mutate references across goroutines. Synchronization and data-race
safety are the program's responsibility, exactly as in Go.

## Breaking Changes

`let` no longer implies deep immutability, and `mut` on a binding no longer
grants interior mutation or satisfies a `mut T` reference destination. Code that
relied on implicit mutable borrowing, interior mutation through an ordinary
`mut` value, implicit reference-to-value reads, or whole-referent writes must be
updated to use explicit `mut` and `deref`.

## Migration Guide

### Reference semantics

```ard
// Reference parameter
// before: update(user)
update(mut user)

// Value binding
// before: let copy: User = reference
let copy: User = deref reference

// Value argument
// before: consume(reference)
consume(deref reference)

// Value return
fn snapshot(reference: mut User) User { deref reference }

// Value field or container element
let holder = Holder{user: deref reference}
let users: [User] = [deref reference]

// Explicitly value-shaped Go generic call
ffi::Identity<User>(deref reference)

// Referent-value comparison through a comparable field
let equal_name = (deref left).name == (deref right).name
```

Use an existing reference directly when identity should flow through a binding,
field, parameter, return, container, or inferred generic.

### Go interop

```ard
// Go pointer/slice/map parameter
// before: ffi::Mutate(value)
ffi::Mutate(mut value)

// Go value destination from a pointer result
// before: let item: ffi::Item = ffi::NewItem()
let pointer = ffi::NewItem()
let item: ffi::Item = deref pointer

// Explicitly value-shaped Go generic
let copied = ffi::Identity<ffi::Item>(deref pointer)

// Interface value instead of pointer identity
ffi::SaveReader(deref pointer)
```

Pass `pointer` without `deref` when Go should receive or infer the pointer
itself.
