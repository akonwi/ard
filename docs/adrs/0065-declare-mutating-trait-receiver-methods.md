# 0065: Declare Mutating Trait Receiver Methods

## Status

Accepted

## Context

Trait declarations describe parameter and return types, but previously could not
describe receiver mutation. An implementation could therefore mark a method
`fn mut` even when the trait declared an ordinary method:

```ard
trait Counter {
  fn set(value: Int)
}

impl Counter for Box {
  fn mut set(value: Int) {
    self.value = value
  }
}
```

The implementation lowers to a Go pointer-receiver method. Because ordinary and
mutable Ard trait values share one native Go interface, an ordinary `Counter`
containing `*Box` could call `set` and mutate the original value. Receiver
mutation was observable but absent from the source contract.

ADR 0061 intentionally erases `Trait` versus `mut Trait` from the generated Go
interface. That representation remains useful, but Ard's checker must preserve
the source-level capability distinction before lowering.

## Decision

Trait methods may declare receiver mutation with the same syntax as impl
methods:

```ard
trait Counter {
  fn mut set(value: Int)
  fn value() Int
}
```

Calling a mutating trait method requires receiver access that permits interior
mutation. A direct ordinary `Trait` value does not provide that capability; a
`mut Trait` value does. As with concrete mutating methods, a trait-valued field
reached through mutable reference-capable storage may be borrowed for the call.

Receiver mutation is an effect bound on implementations:

| Trait method | Implementation method | Conformance |
| --- | --- | --- |
| non-mutating | non-mutating | accepted |
| non-mutating | mutating | rejected |
| mutating | mutating | accepted |
| mutating | non-mutating | accepted |

A non-mutating implementation satisfies a mutating contract because it requires
less receiver capability and performs fewer effects than the contract permits.
Requiring exact equality would force false `fn mut` annotations and unnecessary
Go pointer receivers.

The parser preserves the existing keyword-name disambiguation:

- `fn mut()` declares a non-mutating method named `mut`;
- `fn mut mut()` declares a mutating method named `mut`.

When one concrete type implements multiple traits containing the same method
name, the checker and AIR preserve each trait implementation's method body and
receiver effect independently. Trait dispatch must not resolve those methods
through the concrete type's unqualified method-name table, where implementation
order could choose the wrong body. The Go backend uses its existing
collision-proof fallback interface methods for these cases. An unqualified call
on the concrete type is diagnosed as ambiguous; callers select behavior by
calling through the intended trait type.

AIR records receiver mutability on each trait method and validates both
implementation conformance and call-target capability. The capability is not
part of the emitted Go interface method signature. The implementation still
determines Go receiver shape: a mutating implementation uses `*T`, while a
non-mutating implementation uses `T`.

`Trait` and `mut Trait` continue to lower to one canonical native Go interface
under ADR 0061. This decision adds no marker method, wrapper, forwarding table,
or second runtime interface type.

## Consequences

- Trait declarations communicate whether a method may mutate its receiver.
- Ordinary trait values cannot dispatch mutating trait methods.
- A mutating implementation can no longer satisfy a trait method that omits
  `mut`.
- Existing traits with intentionally mutating implementations must add `mut` to
  those method declarations and pass `mut Trait` where the method is called.
- Read-only implementations may satisfy mutating contracts without changing to
  pointer receivers.
- Same-named methods from different trait implementations retain independent
  bodies and receiver effects through AIR and fallback Go interfaces;
  unqualified concrete calls to those names are ambiguous.
- Foreign Go interfaces remain governed by their Go method sets because Go has
  no receiver-effect annotation; this source guarantee applies to Ard traits
  and Ard implementations.
- Builtin `Error.error()` remains non-mutating and retains its required native
  Go `Error() string` method. If another trait defines `error` for the same
  type, that trait uses collision-proof fallback dispatch instead.
- Ard-defined `Error` implementations must not mutate their receiver.

## Alternatives Considered

### Keep receiver mutability implementation-owned

This preserves the previous behavior but allows mutation through an ordinary
trait value, making the trait declaration an incomplete and unsound source
contract.

### Require exact mutability equality

Exact equality is sound but rejects read-only implementations of a contract
that merely permits mutation. It also encourages unnecessary pointer receivers.

### Emit separate ordinary and mutable Go interfaces

A second interface or marker capability would expose Ard's source distinction in
the Go ABI and conflict with ADR 0061. The checker can enforce the restriction
without changing runtime representation.

## Related

- GitHub issue #416
- `docs/adrs/0009-support-traits-for-shared-behavior.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0057-separate-binding-mutability-from-reference-values.md`
- `docs/adrs/0061-lower-mutable-traits-as-native-go-interfaces.md`
