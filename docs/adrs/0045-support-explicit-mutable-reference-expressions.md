# 0045: Support Explicit Mutable Reference Expressions

## Status

Accepted

Superseded in part by ADR 0057 and amended by ADR 0060. Explicit borrowing now
depends on addressable storage rather than a `mut` binding, all reference
destinations require actual reference values, and reference flow no longer
dereferences implicitly; use `reference.@` to produce the shallow referent
value. Reference copies share the current pointee, while rebinding affects only
the writable destination slot. Whole-referent assignment remains rejected.
Snapshot examples below without `.@` are historical.

## Context

Ard uses `mut` in type positions to mean mutable access to caller-owned storage (`0022-use-mut-for-mutable-references.md`). A `mut T` parameter receives a mutable reference, and for foreign Go types the pointer-shaped form `mut pkg::T` is the Go pointer `*pkg.T` (`0040-decouple-mutability-from-go-pointer-lowering.md`).

Today references are created only implicitly, at two boundaries:

- passing a `mut` binding to a `mut T` parameter;
- assigning a `mut` binding into a `mut T` struct field.

There is no expression that produces a `mut T` value. This leaves two gaps:

1. **Mutable access is invisible at call sites.** `update_person(alice)` gives the callee write access to `alice`, but nothing at the call site says so. Ard's philosophy is that mutation should be visible in source.

2. **Addressable values cannot satisfy pointer method sets.** Go includes the pointer method set when an addressable value is used where an interface is expected, inserting `&v` implicitly. Ard has no equivalent, so a mutable value form cannot satisfy a Go interface whose methods have pointer receivers:

```ard
use go:net/http
use go:net/http/httputil

mut proxy = httputil::ReverseProxy{}
http::ListenAndServe(":0", proxy) // rejected: ServeHTTP lives on *ReverseProxy
```

The workaround is to obtain a pointer form from a Go constructor, which does not exist for every type.

Adopting Go's implicit addressability rule was considered and rejected: it creates invisible long-lived aliasing at call sites and imports Go's addressability taxonomy as unwritten rules. Ard should make taking a reference explicit instead. This is tracked as issue #257.

## Decision

Add `mut <place>` as an expression. It evaluates to a mutable reference to the named storage:

```ard
mut counter = 0
let r = mut counter    // r: mut Int
bump(mut counter)      // mutable access is visible at the call site
```

For a foreign Go type, `mut <place>` produces the pointer form, so pointer method sets and Go interface satisfaction follow from existing rules with no new special cases:

```ard
use go:net/http
use go:net/http/httputil

mut proxy = httputil::ReverseProxy{}
http::ListenAndServe(":0", mut proxy) // mut proxy: mut httputil::ReverseProxy (*httputil.ReverseProxy)
```

### Operand rules

As amended by ADR 0057, the operand is an addressable place, an existing
reference, or a value expression:

- a `let` or `mut` binding provides addressable storage regardless of slot-write
  permission;
- a field reached through an addressable or reference-shaped base may provide a
  place;
- an existing reference is propagated by copying its current pointer-like
  handle, without adding indirection;
- a value expression (`mut Foo{...}`, `mut f()`) is materialized in fresh stable
  storage and the expression evaluates to a reference to it.

A place-like expression without stable addressable storage is rejected. List
elements and map values are not places: accessor methods such as `xs.at(0)`
return copies, so `mut xs.at(0)` takes fresh storage of that copy and never
references element storage.

### Typing

As amended by ADR 0057:

- `mut <expression>` creates `mut T` when an ordinary `T` place or fresh value is
  referenced;
- when the expression is already `mut T`, `mut` copies and returns the same
  `mut T` handle without adding another reference layer;
- Ard-owned `mut (mut T)` and pure-Ard construction of direct-Go `**T` are not
  supported initially;
- for a foreign Go named value type, referencing it produces the pointer-shaped
  foreign form (`*pkg.T`);
- copied references share pointee mutation, while rebinding affects only the
  writable destination slot.

### Call sites for `mut T` parameters

Superseded by ADR 0057: every `mut T` destination requires an actual reference
value. A mutable ordinary binding is not implicitly borrowed:

```ard
update_person(alice)     // rejected when alice contains Person
update_person(mut alice) // allowed: explicit reference creation
```

### Dereferencing

Superseded by ADR 0057 and amended by ADR 0060: postfix `.@` is the explicit
shallow reference-to-value operator. Observational reads still resolve through
the referent, but a concrete `T` destination does not dereference implicitly:

```ard
let alias = person_ref             // copies reference handle, same pointee
let snapshot: Person = person_ref  // rejected
let snapshot: Person = person_ref.@ // allowed shallow value
```

Reference copying follows pointer-value behavior. Pointee mutation is visible
through every copied handle; assigning another reference to a writable binding
or field replaces only that destination slot and does not affect earlier copies.
Whole-referent assignment remains rejected. As further amended by ADR 0057,
reference `==`/`!=` compares pointer identity; compare `.@` results when
referent-value equality is intended.

### Lowering (Go backend)

- `mut <place>` lowers to `&place` for value-form storage.
- `mut <value expression>` lowers to a fresh temporary plus `&tmp` (for struct literals, Go's `&Foo{...}` form directly).
- `mut <place>` where the place is already reference-shaped lowers to the reference itself — aliasing copies the pointer, never adding indirection.
- Runtime interface conversion follows ADR 0056. An explicit reference preserves caller identity; an ordinary value contributes an owned value or boxed copy. The backend therefore carries explicit interface conversion metadata rather than inferring ownership from pointer shape.

Concrete Ard references lower to Go pointers where possible. Descriptor and
trait references may need specialized pointer-like handles: native mutable list
references are pointer-shaped because sanctioned list methods must update the
referenced descriptor, while exact foreign Go slice/map ABI remains unchanged.
Channel handles are intrinsic values rather than `mut T` references.

## Consequences

- Mutable access becomes visible at call sites, opt-in today, with room to strengthen later.
- Explicit references can satisfy Go interfaces with pointer-receiver methods while preserving caller identity. ADR 0056 additionally permits ordinary values through interface-owned boxing without adopting Go's implicit addressability.
- `let r = mut x` introduces named aliases to mutable storage. This is already expressible through struct fields and parameters, so it adds no new aliasing power, but it makes aliasing easier to write; documentation should cover it.
- The checker gains an addressability judgment for expressions. Its rules are attached to explicit syntax, so violations produce local, teachable errors.
- Initial reference creation is explicit with `mut`; subsequent reference flow
  copies the pointer-like handle, and shallow value extraction is explicit with
  `.@`.
- List elements and map values are deliberately not addressable; this is a smaller, stricter surface than Go's.

## Related

- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0040-decouple-mutability-from-go-pointer-lowering.md`
- `docs/adrs/0044-use-a-shared-go-type-universe.md`
- GitHub issue #257
