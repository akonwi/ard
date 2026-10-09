# 0073: Adopt Explicit Pointer Types

## Status

Accepted

## Context

ADR 0057 separated binding mutability from reference values. Ard now has three
uses of `mut`:

```ard
mut user = User{}            // a reassignable binding slot
let reference = mut user     // create a mutable reference
fn update(user: mut User) {} // a mutable-reference type
```

The second and third forms still couple two independent properties:

1. **identity**: whether a value refers to storage shared with other values;
2. **mutation permission**: whether code may write through that value.

Every Ard reference is mutable, and every mutable access to caller-owned
storage requires a reference. There is no way to share storage read-only, and
no way to state mutation permission without also stating identity.

The coupling also obscures the Go target. A mutable reference to a concrete
type is a Go pointer, but the source spelling `mut T` does not communicate
that. Go programmers reading `fn update(user: mut User)` must learn that `mut`
means `*`, that `mut user` means `&user`, and that `.@` means `*p`. Meanwhile
the most Go-familiar operations remain restricted:

```ard
let reference = mut user
reference.@ = replacement // rejected: whole-referent writes are forbidden
```

ADR 0057 also rejects in-place writes to an ordinary `mut` binding:

```ard
mut user = User{name: "Ada"}
user.name = "Grace" // rejected: User is not a reference
```

That rule is surprising. The write only affects storage owned by the writable
slot; no other value observes it.

### Prior art

Ard draws on Go for its runtime model and on Rust, Zig, and Odin for syntax.
ADR 0060 surveyed dereference syntax across these languages.

| Language | Pointer type | Read-only form | Address-of | Dereference |
| --- | --- | --- | --- | --- |
| Go | `*T` | none | `&x`, `&T{}` | prefix `*p` |
| Rust | `&mut T` | `&T` (default) | `&x`, `&mut x` | prefix `*r` |
| Zig | `*T` | `*const T` | `&x` | postfix `p.*` |
| Odin | `^T` | none | `&x` | postfix `p^` |
| Swift | `inout T` | n/a | `&x` at call sites | `.pointee` (unsafe) |

Observations:

- `&x` is universal for taking an address, and `&T{...}` is idiomatic Go.
- Only Rust and Zig encode pointee mutability in the pointer type. Rust's
  separation of `let mut` from `&mut` matches Ard's goal of independent axes.
- Prefix dereference has a precedence trap (`*p.f` is `*(p.f)`). ADR 0060
  rejected prefix `deref` for that reason; Zig and Odin use postfix forms.

## Decision

Ard adopts explicit pointer types and address-of expressions. `mut` no longer
denotes references.

| Concept | Syntax | Meaning |
| --- | --- | --- |
| Writable binding | `mut user = User{}` | The slot and its inline storage may be written. |
| Read-only pointer | `*User`, `&user` | Shared identity; no writes through this pointer. |
| Writable pointer | `*mut User`, `&mut user` | Shared identity; writes through this pointer allowed. |
| Dereference | `pointer.*` | The value at the pointer. |
| Mutable trait value | `mut View` | A trait value that may call `fn mut` methods. |

Pointer syntax always denotes a real pointer. On the Go target, both `*T` and
`*mut T` lower to Go `*T`.

### Pointer types

`*T` is a read-only pointer and `*mut T` is a writable pointer:

```ard
fn show(user: *User) Str { user.name }
fn rename(user: *mut User, name: Str) { user.name = name }
```

Read-only is a property of the pointer, not of the pointee. Like C's
`const T*` and Zig's `*const T`, a `*T` forbids writes through itself but does
not guarantee that no other pointer writes the same storage.

`*mut T` coerces implicitly to `*T`. The reverse is rejected. The coercion
applies at the top level only; containers are invariant because list and map
copies share storage (see "Lists and maps"), so `[*mut T]` is not a `[*T]`.

Pointer type modifiers are prefix and bind more loosely than postfix type
forms, matching existing `mut T` parsing:

```ard
*User?    // pointer to User?
(*User)?  // optional pointer
*mut [Int]
*$T
```

Pointers to pointers remain unsupported for Ard-owned storage, as in ADR 0057.
Exact multi-level foreign pointers may flow only where an already compatible
foreign value exists.

### Address-of expressions

`&` creates a pointer. It is a prefix operator that binds more loosely than
postfix operations, so `&user.profile` is `&(user.profile)`.

```ard
let user = User{name: "Ada"}
let reader = &user // *User

mut account = Account{balance: 0}
let writer = &mut account // *mut Account

let fresh = &mut User{name: "Ada"} // *mut User to fresh storage
```

ADR 0057's operand modes are retained:

- **Addressable place**: a binding, module global, or field reached through an
  addressable base or a pointer.
- **Fresh value**: any other value expression, such as `&User{}` or
  `&mut make_user()`, materialized in fresh stable storage.
- **Invalid place**: a selector on a non-addressable temporary, such as
  `&make_user().profile`, remains rejected.

Accessor results are values, not places: `&mut items.at(0)` points at a fresh
copy.

The idempotent `mut reference` rule of ADR 0057 is removed. Applying `&` to a
pointer-valued place would create a pointer to a pointer and is rejected for
Ard-owned storage. Copy a pointer by ordinary assignment:

```ard
let alias = writer
```

#### Writable places

`&mut place` requires a **writable place**:

- a `mut` binding or writable module global;
- a field of a writable place;
- any place reached through a `*mut` pointer, including `pointer.*`.

```ard
let user = User{}
let a = &user     // allowed: read-only pointers work on any addressable place
let b = &mut user // rejected: `user` is not writable
```

This restriction is required because `*mut T` permits whole-value replacement.
Without it, `(&mut user).* = other` would reassign a `let` binding.

A fresh value is always a writable place.

### Dereference

Postfix `.*` dereferences a pointer. It replaces ADR 0060's `.@`, keeping its
left-to-right composition with calls and member access:

```ard
pointer.*         // the value at pointer
pointer.*.field   // select from the pointee
load().*          // dereference a returned pointer
pointer.* * 2     // (pointer.*) * 2
```

The `.` and `*` must be adjacent.

Reading `pointer.*` produces a shallow copy, as `.@` does under ADR 0057.
Unlike `.@`, `pointer.*` is a place:

```ard
mut user = User{name: "Ada"}

let writer = &mut user
writer.* = User{name: "Grace"} // allowed: replace the whole pointee
writer.*.name = "Lin"          // allowed: same as writer.name = "Lin"

let reader = &user
reader.* = User{}              // rejected: reader is read-only
```

Field access and method calls dereference implicitly, as in Go and as ADR
0057's observational reads already do:

```ard
writer.name             // reads through the pointer
writer.name = "Grace"   // writes through the pointer
writer.rename("Grace")  // fn mut method through *mut
reader.display()        // non-mutating method through *T
```

Dereferencing a nil foreign Go pointer panics, as today.

### Bindings and inline storage

Binding mutability governs the binding's own storage, including inline fields.
This reverses ADR 0057's rejection of field writes on ordinary `mut` bindings:

```ard
mut user = User{name: "Ada"}
user.name = "Grace"       // allowed: writes user's own storage
user = User{name: "Lin"}  // allowed: replaces the slot
```

Pointer-typed fields are writable through regardless of how the containing
value is bound, because the write targets the pointee:

```ard
let user = User{profile: &mut Profile{name: "Ada"}}
user.profile.name = "Grace" // allowed: writes the Profile pointee
user.profile = &mut other   // rejected: writes user's own storage
```

#### Lists and maps

Lists and maps keep their current Go-style sharing: copying a list copies its
descriptor and shares backing storage, and copying a map shares its contents
(ADR 0057). Allowing in-place list or map mutation through an ordinary `mut`
binding would make that sharing observable, including Go `append` aliasing:

```ard
mut b = a
mut c = a
b.push(3)
c.push(4) // could overwrite b's appended element
```

Therefore list and map in-place operations (`push`, `prepend`, `set`, `swap`,
`sort`, map `set`/`delete`, and similar) require the list or map to be reached
through a `*mut` pointer. A `mut` binding may only replace the whole value:

```ard
mut xs = [1, 2]
xs = [9]           // allowed: replaces the slot
xs.push(3)         // rejected: requires *mut [Int]

let ys = &mut [1, 2]
ys.push(3)         // allowed
```

The same applies to list and map fields of a writable binding: `u.tags = [...]`
is allowed, `u.tags.push(...)` is not. A place reached through `*mut`, such as
`self.tags` inside a `fn mut` method, permits in-place operations as it does
today.

Value semantics for lists and maps, which would lift this restriction, is a
separate project.

### Parameters

Parameters remain immutable bindings. Neither `fn f(mut user: User)` nor any
other parameter-level mutability form is accepted. Code that needs a writable
local copy shadows the parameter:

```ard
fn normalize(user: User) User {
  mut user = user
  user.name = user.name.trim()
  user
}
```

Caller-visible mutation requires a pointer parameter:

```ard
fn rename(user: *mut User, name: Str) {
  user.name = name
}

mut user = User{name: "Ada"}
rename(&mut user, "Grace")
```

The parser diagnostic for `fn f(mut user: User)` changes to suggest
`user: *mut User` for caller-visible mutation or `mut user = user` for a local
writable copy.

Method receivers keep the `fn mut method` form. A `fn mut` method requires a
receiver reached through `*mut`; a non-mutating method accepts any receiver.
Calling a `fn mut` method does not take the receiver's address implicitly:

```ard
mut user = User{name: "Ada", tags: []}
user.name = "Grace"         // allowed: field write on a writable binding
user.rename("Lin")          // rejected: requires a *mut receiver
(&mut user).rename("Lin")   // allowed
```

Code that mutates through methods should hold a pointer:

```ard
let user = &mut User{name: "Ada", tags: []}
user.rename("Grace")
user.tags.push("x")
```

Implicit address-taking is not a safety boundary: any writable place can be
explicitly addressed with `&mut`. The rule determines where mutation through
shared storage must be visible. A `fn mut` method may perform in-place list or
map operations on its receiver's fields, so implicit address-taking would
bypass the list and map rule above. Both rules may be relaxed together when
lists and maps gain value semantics.

### Traits

Pointer syntax is not used for trait values. ADR 0061 lowers `Trait` and
`mut Trait` to the same Go interface and gives `mut Trait` existential meaning:
some concrete `T` held as a `*T`. Spelling that `*mut Trait` would misread as
Go's pointer-to-interface, whose semantics differ in slot rebinding, method
calls, equality, and whole-value writes. In particular, `pointer.* = other`
through a pointer to a hidden concrete type cannot be checked statically.

`mut Trait` remains the only type-level use of `mut`. It means a trait value
whose holder may call `fn mut` methods:

```ard
let box = &mut Box{value: 1}
let view: mut View = box // *mut Box widens to mut View
view.bump()              // fn mut method allowed
let plain: View = view   // drops mutation permission; no runtime change
```

A `mut Trait` value is obtained by widening a `*mut T` whose `T` implements the
trait, by copying another `mut Trait`, or through explicit `unsafe::cast`.
`*Trait` and `*mut Trait` are rejected. A future decision may admit them as real
pointers to trait-typed slots lowering to Go `*Trait`.

`mut Trait` is not a pointer, so `.*` does not apply to it. Converting
`mut Trait` to `Trait` preserves the current dynamic object.

ADR 0061's `.@` snapshot of a mutable trait value, which used reflection to
copy the hidden concrete value, is removed without replacement. Code that needs
an independent copy dereferences the concrete pointer before widening, or the
trait declares its own copy method:

```ard
let box = &mut Box{value: 1}
let snapshot: View = box.* // a View holding a copy of the Box
```

The Go runtime's `TraitSnapshot` helper is removed, and trait values no longer
use reflection.

`mut T` for any non-trait type is rejected, with a diagnostic suggesting
`*mut T`.

### Equality

Pointers compare by identity, as in ADR 0057. `*T` and `*mut T` with the same
pointee type are comparable with each other. Pointers do not support relational
ordering.

### Captures and async

Pointers are ordinary values. Closures capture them by copying the pointer;
`mut` bindings are captured as slots, as today. Pointers may cross
`async::start` boundaries under ADR 0033's Go-like model.

### Recursive types

`*T` and `*mut T` are sizedness boundaries for recursive types, replacing
`mut T` in ADR 0020 and ADR 0022:

```ard
struct Node {
  value: Int,
  parent: *Node,
}
```

### Go interop

| Ard | Go |
| --- | --- |
| `*T`, `*mut T` | `*T` |
| `*mut [T]` | `*[]T` |
| `*mut [K: V]` | `*map[K]V` |
| `*mut pkg::T` | `*pkg.T` |
| `Trait`, `mut Trait` | the trait's Go interface |

- A Go `*T` result imports as `*mut T`, which coerces to `*T` where needed.
- A Go `*T` parameter imports as `*mut T`, because Go may write through it.
  Unlike Go slice and map parameters, a `*T` parameter can change the caller's
  own storage, so admitting `*T` would let Go code modify a `let` binding.
  Go structs passed by pointer should be created as pointers:

  ```ard
  let cfg = &mut tls::Config{MinVersion: tls::VersionTLS12}
  tls::Client(conn, cfg)
  ```
- Named Go pointer types previously spelled `mut pkg::T` become `*mut pkg::T`.
- Generic Go `*T` parameters bind to Ard `*mut $T`.
- Converting a pointer to `Any` or a Go interface exposes the dynamic Go
  pointer, as in ADR 0056 and ADR 0057.
- `unsafe::cast<*mut T>` recovers a pointer from `Any`.

#### Go slice and map parameters

Go `[]T` and `map[K]V` parameters accept ordinary `[T]` and `[K: V]` values:

```ard
let ys = ["a", "b"]
strings::Join(ys, ",")

mut values = [3, 1, 2]
sort::Ints(values)
```

This replaces ADR 0057's rule that every Go slice or map parameter requires an
explicit mutable reference. That rule required a writable binding under this
ADR's `&mut` restriction, even for read-only Go APIs such as `strings.Join`.

Go has no read-only slices or maps, so a Go callee may write elements or
entries. Those writes are visible through the caller's list or map and any
copies sharing its storage. Go code is an explicit trust boundary: Ard's checker
constrains Ard source, not foreign callees. Ard already shares list and map
storage between copies (see "Lists and maps").

Pointers remain accepted at these parameters and are projected to the
descriptor value. Writing `sort::Ints(&mut values)` documents intended
mutation at the call site. Parameters whose Go type is a pointer to a slice or
map (`*[]T`, `*map[K]V`) require `*mut [T]` or `*mut [K: V]`.

Variadic parameters keep ADR 0062's list-forwarding rules.

## Migration

Existing syntax maps as follows:

| Current | New |
| --- | --- |
| `name: mut T` (concrete `T`) | `name: *mut T` |
| `name: mut Trait` | unchanged |
| `mut place` | `&mut place` |
| `mut Value{...}`, `mut f()` | `&mut Value{...}`, `&mut f()` |
| `mut reference` (already a reference) | `reference` |
| `reference.@` | `reference.*` |

Some rewrites depend on types: distinguishing trait from concrete referents,
and removing the idempotent `mut reference` form. The migration therefore uses
a checker-backed rewrite tool rather than the syntax-only formatter.

Some programs need manual changes:

- `&mut` of a `let` binding is rejected. The diagnostic suggests changing the
  binding to `mut`.
- `mut` applied to a trait-typed place, which captured the current interface
  value under ADR 0061, has no direct replacement. Store a `mut Trait` value
  instead.

Following ADR 0060, the migration spans two releases:

1. Accept both syntaxes. Old forms produce deprecation warnings with the
   replacement, and the rewrite tool performs mechanical migrations.
2. Remove the old forms. `mut T` in type position is accepted only for traits;
   `mut expression` and `.@` are rejected with migration hints.

Unchanged syntax:

- `mut name = value` bindings;
- `fn mut method(...)` receivers;
- `mut Trait` types.

## Consequences

- Each axis has one spelling: `mut` bindings for writable slots, `*`/`&` for
  identity, and `mut` within pointer types for write permission through a
  pointer.
- Read-only sharing becomes expressible.
- Generated Go and Ard source use the same pointer vocabulary.
- Field writes on writable bindings behave as in Go and Rust.
- Whole-value replacement through `*mut T` is allowed.
- `&mut` of `let` bindings, previously allowed by ADR 0057, is rejected.
- List and map in-place operations still require pointers until list/map value
  semantics are designed.
- `*` gains a type-position and postfix meaning alongside multiplication.
- The lexer, parser, formatter, Tree-sitter grammar, highlighting, LSP,
  checker, AIR, Go backend, diagnostics, documentation, samples, standard
  library, and examples must adopt the new syntax.

## Implementation notes

The checker currently represents references through two mechanisms:
`MutableRef` for Ard references and `ForeignType{Pointer: true}` for named Go
pointers. This ADR should be implemented with one canonical pointer type
carrying the pointee type and a mutability flag, with foreign pointer shape
recorded as boundary metadata. Checker judgments for writable slots, writable
places, pointer write permission, and trait mutation permission should be
separate predicates.

AIR keeps a single `TypeReference` kind and adds pointee mutability. Both
mutability forms share one Go representation, so backend changes are limited
to the new dereference place and whole-value writes.

Suggested phases:

1. Lock semantics with checker and Go-target tests.
2. Add lexer, parser, formatter, and Tree-sitter support for `*T`, `*mut T`,
   `&`, `&mut`, and `.*`, accepting old forms with deprecation warnings.
3. Introduce the canonical checker pointer type, read-only pointers, writable
   place rules, and binding field writes.
4. Restrict type-level `mut` to traits and update trait widening.
5. Lower `.*` places and whole-value writes.
6. Add the checker-backed migration tool and migrate the standard library,
   samples, tests, examples, and documentation.
7. Remove old forms in the following release.

## Deferred work

- **List and map value semantics.** Copying a list or map into or out of a
  writable place would copy its storage, removing observable sharing. That
  would allow in-place list and map operations and implicit address-taking for
  `fn mut` calls on writable bindings.
- **Read-only Go pointer parameters.** A per-API opt-in, such as an annotation,
  a curated set of read-only standard-library APIs, or a Go shim taking values,
  could admit `*T` arguments where Go only reads.
- **Pointers to trait-typed slots.** `*Trait` and `*mut Trait` could later
  denote real pointers to trait-typed storage, lowering to Go `*Trait`.
- **Trait value snapshots.** A named operation could restore snapshotting of
  `mut Trait` values if a need arises.

## Supersessions

- **ADR 0020**: `mut T` sizedness boundaries become `*T` and `*mut T`.
- **ADR 0022**: `mut T` no longer denotes a mutable reference. Pointers are
  spelled `*T` and `*mut T`.
- **ADR 0030**: Go `*T` maps to `*mut T` rather than `mut T`.
- **ADR 0040**: Mutable access representation is chosen from pointer types,
  not `mut`.
- **ADR 0045**: `mut place` becomes `&mut place`, and `&mut` requires a
  writable place.
- **ADR 0057**: Retained: separate binding and reference axes, reference
  modes, pointer-copy behavior, identity equality, observational reads, and
  list/map sharing. Superseded: reference syntax, `mut reference`
  idempotence, borrowing of `let` bindings for mutation, the rejection of field
  writes on `mut` bindings, the rejection of whole-referent writes, and the
  explicit-reference requirement for Go slice and map parameters.
- **ADR 0058**: Writable slice views use `*mut Slice<T>`.
- **ADR 0060**: `.*` replaces `.@`, and dereference becomes a place.
- **ADR 0061**: `mut Trait` is retained with its representation. Its creation
  syntax becomes widening from `*mut T`, mutable capture of trait-typed places
  is removed, and `.@` snapshotting and its reflective runtime helper are
  removed.
- **ADR 0062**: Spreading a pointer to a list uses `&mut args` or `&args`;
  `&mut` requires a writable place.

## Related

- `docs/adrs/0020-support-recursive-struct-fields-through-indirection.md`
- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0030-use-direct-go-struct-values-and-fields.md`
- `docs/adrs/0040-decouple-mutability-from-go-pointer-lowering.md`
- `docs/adrs/0045-support-explicit-mutable-reference-expressions.md`
- `docs/adrs/0056-preserve-ard-value-semantics-when-lowering-go-interfaces.md`
- `docs/adrs/0057-separate-binding-mutability-from-reference-values.md`
- `docs/adrs/0058-represent-list-slices-as-fixed-length-shared-views.md`
- `docs/adrs/0060-adopt-postfix-value-at-dereference-syntax.md`
- `docs/adrs/0061-lower-mutable-traits-as-native-go-interfaces.md`
- `docs/adrs/0065-declare-mutating-trait-receiver-methods.md`
