# 0045: Support Explicit Mutable Reference Expressions

## Status

Accepted

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

The operand is either a *mutable place* or a value expression:

- a `mut` binding;
- a field reached through mutable access (a `mut` binding's field, or a `mut T` field's referent);
- a place whose storage is already `mut T`: the result is another reference to the *same referent*. Because reads through references produce referent values, `mut` is the only spelling that aliases — it is meaningful, not redundant;
- a value expression (`mut Foo{...}`, `mut f()`): the value becomes fresh mutable storage and the expression evaluates to a reference to it, equivalent to binding the value to a `mut` local first.

Rejected with a specific diagnostic:

- `let` bindings and immutable fields: taking `mut` of immutable storage.

List elements and map values are not places: Ard has no element-access syntax, and accessor methods such as `xs.at(0)` return copies. `mut xs.at(0)` is therefore legal but takes fresh storage of the copy — it never references element storage.

### Typing

- `mut <expression>` creates a `mut T` where `T` is the expression's type.
- For a foreign Go named type, the result is the pointer-shaped foreign form (`*pkg.T`), consistent with `mut pkg::T` in type positions.
- References may alias, matching the existing model: writes through one reference are visible through all references to the same storage.

### Call sites for `mut T` parameters

Existing implicit passing remains valid: `update_person(alice)` where `alice` is a `mut` binding continues to check. `update_person(mut alice)` is now also valid and means the same thing.

The explicit spelling is recommended style, and the formatter and documentation should prefer it. Requiring it is deferred: it would break every existing call site, and enforcement can be revisited (as a lint or a formatter rewrite) once the syntax has settled.

### Dereferencing

There is no dereference operator. Creating an alias is the visibility-worthy act and gets syntax; reading through or copying out of a reference is safe and stays lightweight:

- Reads see through references implicitly: field access, method calls, and operators on a `mut T` operate on the referent.
- Writes through a `mut T` place assign the referent; they do not rebind the reference.
- A reference binding cannot be rebound: assigning another reference through it is rejected (`References cannot be rebound; assign the value directly`), and whole-value assignment through a descriptor-backed reference is rejected because the alias shares element storage, not the original binding slot.
- A value copy is produced whenever a reference flows into a value context — a `T` parameter, a binding, or a `T` field. Reads dereference, so binding a reference-typed place copies the referent:

```ard
let snapshot = person_ref  // snapshot: Person — a copy
takes_value(person_ref)    // T parameter receives a copy
```

- Aliasing is always explicit. To bind another reference to the same storage, use `mut`:

```ard
let alias = mut person_ref // alias: mut Person — same storage
```

This keeps the pairing uniform: copies are quiet, aliases are always spelled `mut`.

- Equality on references compares referent values wherever the referent type defines equality (`mut Int == mut Int` compares the integers). Reference *identity* comparison is not expressible; if a real need appears it should arrive as an explicit, named operation rather than an overload of `==`. This codifies existing behavior.

### Lowering (Go backend)

- `mut <place>` lowers to `&place` for value-form storage.
- `mut <value expression>` lowers to a fresh temporary plus `&tmp` (for struct literals, Go's `&Foo{...}` form directly).
- `mut <place>` where the place is already reference-shaped lowers to the reference itself — aliasing copies the pointer, never adding indirection.
- Interface satisfaction needs no backend changes: the operand is already the pointer form when it reaches the boundary.

Not every Ard reference is a Go pointer (ADR 0040): descriptor-backed types (lists, maps, channels) already share storage by value, so `mut` of those lowers to the descriptor itself. The backend's existing per-representation decision applies; `&` is emitted only where the representation requires a pointer.

## Consequences

- Mutable access becomes visible at call sites, opt-in today, with room to strengthen later.
- Addressable mutable values can satisfy Go interfaces with pointer-receiver methods, closing the interop gap without adopting Go's implicit addressability.
- `let r = mut x` introduces named aliases to mutable storage. This is already expressible through struct fields and parameters, so it adds no new aliasing power, but it makes aliasing easier to write; documentation should cover it.
- The checker gains an addressability judgment for expressions. Its rules are attached to explicit syntax, so violations produce local, teachable errors.
- Aliasing is uniformly loud: every expression that creates or propagates a reference is spelled `mut`, while dereferencing copies stay quiet.
- List elements and map values are deliberately not addressable; this is a smaller, stricter surface than Go's.

## Related

- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0040-decouple-mutability-from-go-pointer-lowering.md`
- `docs/adrs/0044-use-a-shared-go-type-universe.md`
- GitHub issue #257
