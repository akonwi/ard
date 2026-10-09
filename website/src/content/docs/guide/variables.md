---
title: Variables
description: Learn about bindings, pointers, dereferencing, and type inference in Ard.
---

## Declaration keywords

Ard uses two declaration keywords:

- `let` creates a binding whose storage cannot be written.
- `mut` creates a binding whose storage can be written: the whole value can be replaced, and its fields can be assigned.

```ard
let name = "Ada"
// name = "Grace" // Error: the binding is immutable

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

A pointer keeps its pointer type during inference:

```ard
struct User { name: Str }

mut user = User{name: "Ada"}
let pointer = &mut user // inferred as &mut User
let alias = pointer     // also &mut User; copies the pointer
```

## Mutable bindings

A `mut` binding owns its storage, including the fields of a struct stored inline:

```ard
struct User { name: Str }

mut user = User{name: "Ada"}
user.name = "Grace"        // OK: writes user's own storage
user = User{name: "Lin"}   // OK: replaces the whole value

let fixed = User{name: "Ada"}
// fixed.name = "Grace"    // Error: fixed is not writable
```

Writing to a binding never affects other values. Copies of a struct are independent:

```ard
mut first = User{name: "Ada"}
let second = first
first.name = "Grace" // second.name is still "Ada"
```

Lists and maps are the exception: copies share storage, so in-place operations such as `push` and `set` need a pointer even on a `mut` binding. See [Lists and maps](#lists-and-maps).

## Pointers

A pointer shares storage. Ard has two pointer types:

| Type | Created with | Meaning |
| --- | --- | --- |
| `&T` | `&value` | Read-only pointer: reads through the pointer only. |
| `&mut T` | `&mut value` | Writable pointer: reads and writes through the pointer. |

```ard
struct User { name: Str }

mut user = User{name: "Ada"}

let reader = &user     // &User
let writer = &mut user // &mut User

writer.name = "Grace"  // writes user
reader.name            // "Grace"
```

Field access and method calls go through a pointer implicitly. A `&mut T` can be used wherever a `&T` is expected; the reverse is rejected.

`&` works on any addressable place, but `&mut` requires a **writable place**: a `mut` binding, a field of one, or a place reached through a `&mut` pointer. Taking `&mut` of a fresh value creates new storage:

```ard
let fixed = User{name: "Ada"}
let a = &fixed                    // OK: read-only pointer
// let b = &mut fixed             // Error: fixed is not writable

let fresh = &mut User{name: "Lin"} // &mut User to new storage
```

`&T` is read-only only through that pointer. Other `&mut T` pointers to the same storage can still write it.

A pointer binding declared with `let` cannot be rebound, but it can still write through to its pointee. A `mut` pointer binding can also point somewhere else:

```ard
mut first = User{name: "First"}
mut second = User{name: "Second"}
mut current = &mut first
let alias = current

current = &mut second // rebinds only current
alias.name = "One"    // still writes first
current.name = "Two"  // writes second
```

## Pointer parameters

Parameters are immutable bindings. To let a function change the caller's value, take a `&mut T` parameter:

```ard
fn rename(user: &mut User, name: Str) {
  user.name = name
}

mut user = User{name: "Ada"}
rename(&mut user, "Grace")
```

A function that only needs its own writable copy shadows the parameter instead:

```ard
fn renamed(user: User, name: Str) User {
  mut user = user
  user.name = name
  user
}
```

## Dereferencing with `.*`

Postfix `.*` gives the value at a pointer. Reading it produces a shallow copy:

```ard
mut user = User{name: "Ada"}
let pointer = &mut user
let snapshot: User = pointer.*
```

`pointer.*` is also a place. Through a `&mut T` pointer it can be assigned, replacing the whole pointee:

```ard
pointer.* = User{name: "Grace"} // user is now Grace
pointer.*.name = "Lin"          // same as pointer.name = "Lin"
```

`.*` evaluates its operand once and composes left to right with calls and member access: `load().*.name`.

The copy is **shallow**:

- structs, fixed arrays, and primitive values copy their current value;
- pointer-valued fields keep copied pointers;
- lists initially share their existing backing storage, although later growth may detach one descriptor;
- maps continue sharing map contents;
- channels and foreign handles retain their intrinsic sharing behavior.

Ard does not provide a deep copy. Programs that need one construct it explicitly.

Pointers compare by identity. Compare pointee values explicitly when their types support equality:

```ard
let same_place = pointer == &user

mut count = 1
let count_pointer = &mut count
let same_value = count_pointer.* == count
```

## Lists and maps

Copying a list copies its descriptor and shares its backing storage. Copying a map shares its contents. To keep that sharing visible, in-place list and map operations such as `push`, `set`, `swap`, and `delete` require a `&mut` pointer. A `mut` binding can only replace the whole value:

```ard
mut values = [1, 2]
values = [3]          // OK: replaces the value
// values.push(4)     // Error: requires &mut [Int]

let items = &mut [1, 2]
items.push(3)         // OK
```

## Pointer-valued fields

Struct fields can store pointers. Writing through a pointer field targets the pointee, so it does not need the containing value to be writable:

```ard
struct Tree { value: Int }
struct Context { tree: &mut Tree }

let context = Context{tree: &mut Tree{value: 1}}
context.tree.value = 2                   // OK: writes the Tree

// context.tree = &mut Tree{value: 3}    // Error: writes context's own storage
```

## Migrating from `mut` references

Earlier releases spelled pointers with `mut`. That syntax still works but reports deprecation warnings, and it will be removed in a future release:

| Before | Now |
| --- | --- |
| `user: mut User` | `user: &mut User` |
| `mut user` | `&mut user` |
| `mut pointer` (already a pointer) | `pointer` |
| `pointer.@` | `pointer.*` |

`mut Trait` is unchanged; see [Traits](/advanced/traits/).

`ard migrate <path>` rewrites the mechanical cases in place. Use `ard migrate --check <path>` to list files that still need migration without changing them. Some uses need a manual change and are reported instead. One example is borrowing a parameter, because `&mut` requires a writable place: shadow the parameter with `mut name = name` first.

## Shadowing

Redeclaring a name in the same scope creates a new binding:

```ard
let x = 5
let x = x + 1
let x: Str = "hello"
x.size()
```
