# 0085: Keep Go Interface Values Out of Equality

## Status

Accepted. Builds on ADR 0042, which provides the alternative.

## Context

While porting Cooper's signal watcher to Ard, a check for a terminal resize
was written the way Go code writes it:

```ard
fn is_resize(sig: os::Signal) Bool {
  sig == syscall::SIGWINCH
}
```

The checker rejects it. `os::Signal` is a Go interface and
`syscall::SIGWINCH` is a `syscall.Signal`, a named integer that implements
it. Ard's `==` and `!=` accept primitives, nullable primitives, enums, Go
named scalars, and pointers and references by identity (ADR 0031, ADR 0057,
ADR 0073). Go interfaces, traits, `Error`, `Any`, and structs (Ard or Go) are
not comparable, and two operands of different types never are.

Go itself allows the comparison, with these rules:

- Comparing an interface value with a non-interface value of a comparable
  type `X` that implements the interface is always safe. If the dynamic type
  is not `X`, the result is `false`.
- Comparing two interface values panics at run time when both hold the same
  dynamic type and that type is not comparable, for example a struct with a
  slice field. Many Go values that look like constants are interface-typed:
  `os.Interrupt` is an `os.Signal` and `io.EOF` is an `error`. So
  `sig == os::Interrupt` is in this category.

Two Ard-specific facts also matter:

- An Ard value stored in a Go interface is boxed as a pointer (ADR 0056). Go
  interface equality then compares pointer identity, so two separately boxed
  equal Ard values compare unequal.
- A type pattern in `match` (ADR 0042) already answers "does this interface
  hold this value?" without a panic path:

  ```ard
  fn is_resize(sig: os::Signal) Bool {
    match sig {
      syscall::Signal(s) => s == syscall::SIGWINCH,
      _ => false,
    }
  }
  ```

  The pattern narrows to the concrete type, and that type's own `==` is
  statically valid. This idiom was not documented, so the port fell back to
  comparing `sig.String()` with `syscall::SIGWINCH.String()`, which conflates
  distinct types whose strings happen to be equal.

Three options were considered:

1. **Allow `==` like Go.** This matches Go, but a plain `==` could then panic,
   and comparisons involving boxed Ard values would quietly test identity.
2. **Allow only the forms that cannot panic.** Accept `iface == x` when `x`
   is a Go-owned, strictly comparable, non-interface type that implements the
   interface. This is sound, but it adds a rule users must learn, and the
   common interface-to-interface case (`sig == os::Interrupt`) stays rejected
   anyway.
3. **Keep `==` rejected and point to `match`.** No language change. The
   diagnostic explains the alternative.

## Decision

Option 3. `==` and `!=` stay unavailable on Go interface values, in every
operand combination. Testing an interface's contents is a dynamic type test,
and Ard spells dynamic type tests with match type patterns (ADR 0042), whose
required catch-all arm makes the "some other type" case explicit.

The equality diagnostic carries a hint whenever an operand is a non-empty Go
interface:

- When the other operand is a concrete Go type that the interface can hold,
  that a type pattern can name (not a pointer, since ADR 0042 defers pointer
  patterns), and that Ard can compare, the hint gives the exact match:

  ```
  error: Values cannot be compared
   --> main.ard:5:10
    |
  5 |   sig == syscall::SIGWINCH
    |          ^^^^^^^^^^^^^^^^^ right operand has type `syscall::Signal`
   --> main.ard:5:3
    |
  5 |   sig == syscall::SIGWINCH
    |   ^^^ left operand has type `os::Signal`
    |
    = `os::Signal` is a Go interface, which does not support `==`. To test for a `syscall::Signal`, match on the dynamic type: `match value { syscall::Signal(v) => v == other, _ => false }`
  ```

  For `!=` the hint uses `v != other` and a `true` fallback.
- Otherwise, including interface-to-interface comparisons such as
  `sig == os::Interrupt`, the hint describes the type pattern in general
  terms.

Empty Go interfaces map to `Any` and are outside this rule. `Error` is also
outside it. Sentinel checks such as `err == io::EOF` compare two `error`
values, and Go's `errors.Is` is the right tool there, because it also follows
wrapped errors.

## Consequences

- Equality has no new panic path, and `==` on non-generic values never
  panics.
- Ported Go code that compares interfaces gets a diagnostic that names the
  replacement. The match form is longer than Go's `==`, but it reads as what
  it is: a test of the dynamic type, with its fallback written out.
- Comparing an interface with a Go value that has no pattern form, such as a
  pointer (`w == os::Stdout` with `w: io::Writer`), has no direct
  replacement until pointer type patterns exist. `unsafe::cast<mut T>` with
  pointer identity remains available.
- A generic function whose body uses `==` on `$T` can still be instantiated
  with a Go interface, a trait, or `Any`, and then compares with Go's
  interface equality, including its panic (ADR 0061). This decision does not
  close that path. Restricting such instantiations is a separate change.
- Structural equality on Go structs (`image::Point == image::Point`) remains
  unsupported. It is unrelated to interfaces.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0042-support-dynamic-foreign-type-tests.md`
- `docs/adrs/0056-preserve-ard-value-semantics-when-lowering-go-interfaces.md`
- `docs/adrs/0061-lower-mutable-traits-as-native-go-interfaces.md`
- `docs/go-interface-semantics.md`
- `compiler/checker/go_interface_equality.go`
