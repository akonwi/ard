---
title: Variables
description: Learn about bindings, mutable references, explicit dereferencing, and type inference in Ard.
---

## Declaration keywords

Ard uses two declaration keywords:

- `let` creates a binding whose slot cannot be reassigned.
- `mut` creates a binding whose slot can be reassigned.

The keyword controls the **binding slot**, not whether the stored value has mutable interior access.

```ard
let name = "Ada"
// name = "Grace" // Error: the binding slot is immutable

mut count = 1
count = 2
count =+ 1
```

## Type inference and annotations

Types are normally inferred, but annotations are available:

```ard
let name: Str = "Bob"
let temperature: Float64 = 98.6
let items: [Int] = [1, 2, 3]
let labels: [Str: Int] = ["a": 1, "b": 2]
```

A reference value keeps its reference type during inference:

```ard
struct User { name: Str }

let user = User{name: "Ada"}
let reference = mut user // inferred as mut User
let alias = reference    // also mut User; copies the reference handle
```

## Binding mutability and reference values

Binding mutability and mutable-reference values are independent:

| Declaration | Slot can be reassigned | Stored value is a reference | Interior mutation |
| --- | ---: | ---: | ---: |
| `let user = User{name: "Ada"}` | no | no | no |
| `mut user = User{name: "Ada"}` | yes | no | no |
| `let user = mut User{name: "Ada"}` | no | yes | yes |
| `mut user = mut User{name: "Ada"}` | yes | yes | yes |

A mutable ordinary binding permits whole-slot replacement, but not interior mutation:

```ard
struct User { name: Str }

mut user = User{name: "Ada"}
user = User{name: "Grace"} // OK: replaces the binding slot
// user.name = "Lin"       // Error: user stores an ordinary User value
```

Create an actual reference to mutate a value's interior. The source storage may be declared with either `let` or `mut`:

```ard
let user = User{name: "Ada"}
let reference = mut user
reference.name = "Grace"
```

A `let` reference cannot be rebound, but it can mutate its pointee. A `mut` reference binding can also replace its own stored handle:

```ard
let first = User{name: "First"}
let second = User{name: "Second"}
mut current = mut first
let alias = current

current = mut second // rebinds only current
alias.name = "One"   // still mutates first
current.name = "Two" // mutates second
```

## Explicit reference destinations

`mut T` in a type position means “a mutable reference to `T`.” Such a destination requires an actual reference value; a writable ordinary binding is not borrowed implicitly.

```ard
fn rename(user: mut User, name: Str) {
  user.name = name
}

let user = User{name: "Ada"}
rename(mut user, "Grace")

let reference = mut user
rename(reference, "Lin")
```

`mut expression` has three useful behaviors:

- borrowing addressable local, field, or module storage;
- copying an existing reference handle (`mut reference` is idempotent);
- creating stable fresh storage for a value expression such as a literal or call result.

Copy-producing accessors still produce fresh storage rather than a reference into the container.

## Explicit shallow values with `deref`

References remain references during ordinary value flow. Use `deref` when a destination needs the current `T` value:

```ard
let user = User{name: "Ada"}
let reference = mut user
let snapshot: User = deref reference
```

`deref` removes exactly one outer reference layer and evaluates its operand once. The copy is **shallow**:

- structs, fixed arrays, and primitive values copy their current value;
- reference-valued fields keep copied reference handles;
- lists initially share their existing backing storage, although later growth may detach one descriptor;
- maps continue sharing map contents;
- channels and foreign handles retain their intrinsic sharing behavior.

`deref` is not a deep-copy operation, and Ard does not provide one. Programs that need an independent deep copy construct it explicitly.

References compare by pointer identity. Compare referent values explicitly when their value types support equality:

```ard
let same_place = reference == mut user

let count = 1
let count_reference = mut count
let count_snapshot = deref count_reference
let same_value = deref count_reference == count_snapshot
```

## Reference-valued fields

Struct fields can store references:

```ard
struct Tree { value: Int }
struct Context { tree: mut Tree }

let tree = Tree{value: 1}
let context = mut Context{tree: mut tree}
context.tree.value = 2

let other = Tree{value: 3}
context.tree = mut other // rebinds the field's reference slot
```

The containing value must itself be reached through a reference to rebind a reference-valued field. Reading or mutating the referenced tree does not require the field's binding slot to be reassignable.

## Migrating older reference code

Older Ard code could implicitly borrow mutable bindings or silently turn references into values. Make both boundaries explicit:

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

Use an existing reference directly when identity should flow through a binding, field, parameter, return, container, or inferred generic.

## Shadowing

Redeclaring a name in the same scope creates a new binding:

```ard
let x = 5
let x = x + 1
let x: Str = "hello"
x.size()
```
