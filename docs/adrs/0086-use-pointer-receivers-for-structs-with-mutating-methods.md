# 0086: Use Pointer Receivers for Structs with Mutating Methods

## Status

Accepted. Amends the Receivers and Traits sections of ADR 0031.

## Context

ADR 0031 lowered each method's receiver from its own mutability: a `fn mut`
method got a pointer receiver and every other method a value receiver.

```go
func (self *Box) Close() { ... }
func (self Box) Label() string { ... }
```

A call to a non-mut method through a `&mut Box` therefore lowered to
`(*b).Label()`, which copies the whole struct. The copy reads every field,
including fields the method never uses. When another goroutine writes one of
those fields, even under a mutex, the copy is an unsynchronized read, and
`ard run --race` reports it (issue #622). A runtime written in Ard hit this
when read-only `stats()` calls on worker goroutines raced with a `fn mut
shutdown()` that set a `closed` flag under a lock. Cooper hit it too, and
worked around it by making read-only methods `fn mut`.

The copy is also a correctness problem. A non-mut method can lock a mutex it
reaches through a pointer field:

```ard
fn stats() Int {
  self.lock.Lock()
  let n = self.count
  self.lock.Unlock()
  n
}
```

With a value receiver, `count` is read when the receiver is copied, before
the lock is taken, so the method can return a stale value. A struct that
holds a `sync::Mutex` by value is copied together with its lock on every
non-mut call, which `go vet` reports as copylocks.

The ADR 0078 operand ordering made this worse: when a later argument
contained a call, the receiver was read into a temporary first
(`ard.Read(*b).Lookup(key())`), which copies the struct even where Go would
have passed the pointer.

Three lowerings were considered:

1. Pointer receivers for every struct method. This fixes every case, but it
   moves every struct's method set to `*T`. A plain record passed to Go as
   `any` would then lose its `String()`, and the change touches every
   generated method.
2. Pointer receivers for every method of a struct that has at least one
   `fn mut` method. Other structs keep value receivers.
3. Keep value receivers, and emit each method body a second time as a
   standalone function taking a pointer, for `&mut` call sites to call. This
   doubles every method and contradicts ADR 0031's rule against duplicated
   standalone helpers.

## Decision

A struct with at least one `fn mut` method, in any of its impls, uses pointer
receivers for all of its Go methods. A struct without `fn mut` methods keeps
value receivers. Enums and unions keep value receivers. This follows Go's own
convention that if some methods of a type need a pointer receiver, all of
them should have one, and it covers the types that carry shared mutable
state, because those are the types with mutating methods.

The decision depends only on the type's own declarations, never on how other
code uses the type. For a generic struct it is made on the generic
definition, so every instantiation agrees.

One exception keeps Go interop unchanged: a method that implements an
imported Go interface (an explicit `impl fmt::Stringer for T`, or the builtin
`Error` contract's `Error()` method) keeps the receiver its own mutability
gives it. Go checks some interfaces dynamically, for example `fmt` finds
`String()` on a value passed as `any`, so moving such a method to `*T` would
silently change behavior. These methods still copy their receiver when called
through a pointer.

```go
func (self *Box) Close() { ... }
func (self *Box) Label() string { return self.Name }
func (self Box) String() string { ... } // implements fmt.Stringer
```

### Receivers that are values in Ard

A non-mut method's receiver is still a value in Ard: the checker treats
`self` as immutable, and nothing about the source language changes. The
lowering keeps that meaning:

- A field read goes through the pointer: `self.Name`.
- `self` used as a value, such as returning it, passing it to a function, or
  capturing it in a closure, dereferences the pointer, so the callee or
  closure gets its own copy: `return *self`, `describe(*self)`.
- A call through `&mut T` passes the pointer: `b.Label()`.
- A call on an addressable value (a local, a parameter, or a field of one)
  lets Go take its address: `v.Label()`.
- A receiver that Go cannot address, such as a call result or a composite
  literal, is first copied into a temporary: `_tmp := make(); _tmp.Label()`.
  This is the copy a value receiver would have made.
- A trait object built from a value holds a pointer to a copy, `Trait(&tmp)`.
  This is the path ADR 0056 already uses for implementations with pointer
  receivers. A trait object built from a `&mut T` holds that pointer.
- Under ADR 0078 an addressable receiver is ordered as the storage it names,
  as a `fn mut` receiver is, rather than read into a copy before later
  arguments.

### Observable change

For these types, a non-mut method reads its receiver when it runs, through
the caller's storage, as a `fn mut` method and Go already do. Before, it read
a copy taken at the call. The difference is visible only when the receiver is
changed while the call is in progress. That can happen in three ways: an
argument mutates it (`c.plus(c.next())` now sees the increment), the method
runs code that writes the same storage through another reference, or another
goroutine writes it. In the goroutine case, the method now races only on the
fields it actually reads, and those can be protected with a lock.

## Consequences

- Calling a non-mut method through a shared `&mut T` no longer reads fields
  the method does not use, so locking only the fields a method touches is
  enough under `--race`. Large structs are no longer copied per call.
- A struct that gains its first `fn mut` method moves its Go method set
  (except Go-interface methods) to `*T`. Ard code is unaffected. Go code that
  calls these methods on a `T` value needs an addressable value or a pointer.
- Structs without `fn mut` methods are unchanged, including the case where a
  struct with no mutating methods is written through `&mut` field assignment
  on one goroutine while another calls its methods. Such code can add a
  `fn mut` method or keep the mutable state behind a pointer field.
- Methods implementing imported Go interfaces keep copying when called
  through a pointer.
- Generated Go changes for affected types. Their receivers become `*T`,
  `(*p).M()` calls become `p.M()`, and inside their methods, field reads that
  a later call could change are now ordered with `ard.Read` (ADR 0078),
  because `self` is no longer a private copy.
- Methods cannot be used as function values in Ard, so no method expression
  of type `func(*T)` has to stand in for a `func(T)`. The backend reports an
  internal error if one is ever requested.

## Related

- `0022-use-mut-for-mutable-references.md`
- `0031-go-backend-lowering-contract.md`
- `0033-async-is-goroutines-and-channels.md`
- `0056-preserve-ard-value-semantics-when-lowering-go-interfaces.md`
- `0065-declare-mutating-trait-receiver-methods.md`
- `0073-adopt-explicit-pointer-types.md`
- `0078-evaluate-operands-in-source-order.md`
