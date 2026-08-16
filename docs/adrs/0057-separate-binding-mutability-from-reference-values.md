# 0057: Separate Binding Mutability from Mutable Reference Values

## Status

Accepted

Surface syntax amended by ADR 0060. Postfix `.@` is canonical; prefix `deref`
is accepted only during the one-release migration window.

## Implementation status

Complete. Phase 1's semantic contracts are in place, and Phases 2–8 are
done. `.@` is parsed, formatted, and grammar-supported; the checker
implements first-class reference typing, capability judgments, explicit-borrow
classification (`ExistingReference` / `AddressablePlace` / `FreshValue`), the
uniform reference-destination policy, assignment target categories,
observational reads, pointer-identity equality, ordinary async closure
captures, and direct-Go boundary classification. AIR preserves the three
creation modes, explicit and
observational dereferences, canonical recursive `TypeReference` identity,
concrete-to-trait projection metadata, creation-time reference-handle captures,
and writable/address-taken slot captures. The Go backend lowers concrete and
descriptor references as pointer-copy handles, preserves slot rebinding and
escaping storage, represents mutable-trait references with sealed comparable
interface adapters, and projects exact pointer, descriptor, generic, `Any`, and
Go-interface ABIs.
All ADR 0057 Go runtime/FFI contracts pass. Diagnostics now distinguish
addressability, actual-reference destinations, explicit value materialization,
forbidden whole-referent writes, immutable slots, and reference-required
receivers. The variables, functions, structs, generics, async, and Go interop
guides document the new model and migration paths. Phase 8 migrated the
compiler samples, the Go backend fixture corpus, and the vaxis-demo example to
explicit references and `.@`; the full compiler test suite, formatter
verification, and the LSP harness pass. Downstream migration also hardened the
backend: fallback mutable-trait adapters dispatch through collision-proof
implementation methods instead of assuming a native Go interface,
foreign-method values project descriptor arguments through the exact Go ABI,
and mutable-trait wrapper machinery is not emitted for traits without any
reference use.
Contextual typing flows into container literals through `mut` at reference
destinations, including declaration-scope generics (`let out: mut [$T] =
mut []`); only unresolved call-inference variables still require an explicit
element type. Known deferred gap: generic `mut $T` parameters do not yet unify
with concrete foreign pointer values; this is deliberately deprioritized —
concrete `mut ffi::T` signatures cover the practical cases.

## Context

Ard currently uses `mut` in three related but distinct positions:

```ard
mut value = User{}        // a reassignable binding
let reference = mut value // an explicit mutable-reference expression
fn update(value: mut User) // a mutable-reference type
```

ADRs 0022 and 0045 intentionally connected these forms. A `mut` binding was
both a writable binding slot and an implicitly borrowable value, while a `let`
binding was treated as immutable storage that could not be referenced mutably.
Reference-valued expressions were also read through implicitly in ordinary
value contexts, so storing a reference in another unannotated binding copied
the referent rather than preserving the reference.

That model conflates two independent properties:

1. whether a binding slot may be assigned through its name; and
2. whether the value stored in that slot is itself a mutable reference.

It produces the wrong result for both directions. A `let` binding names stable,
addressable storage and should be explicitly borrowable even though direct
writes through that name are forbidden. Conversely, a `mut` binding containing
an ordinary `T` should not silently become a `mut T` merely because its slot is
writable.

The distinction becomes especially visible when a binding stores a reference:

```ard
let user = mut User{name: "Ada"}
user.name = "Grace"
```

The `user` binding is not reassignable, but its stored value is a reference, so
interior mutation of the referent is valid. Binding mutability must not erase or
override the type of the stored data.

This correction is deliberately broader than allowing `mut letValue` in one
checker branch. It changes reference flow, assignment, closure capture,
generic inference, Go lowering, interface conversion boundaries, diagnostics,
and migration behavior. Those consequences are specified here before the
implementation changes.

## Decision

Binding mutability and mutable-reference values are independent language axes.

- `mut name = value` makes the binding slot writable.
- `mut expression` creates or propagates a mutable-reference value.
- `mut T` in a type position requires an actual mutable-reference value.
- A mutable binding never implicitly creates a mutable reference.
- A `let` binding may store and use a mutable reference.

### Binding and value matrix

The four combinations have these capabilities:

| Declaration | Binding slot writable | Stored value is a reference | Interior mutable |
| --- | ---: | ---: | ---: |
| `let user = User{}` | no | no | no |
| `mut user = User{}` | yes | no | no |
| `let user = mut User{}` | no | yes | yes |
| `mut user = mut User{}` | yes | yes | yes |

A mutable ordinary binding permits slot writes, including scalar compound
assignment, but does not permit interior mutation:

```ard
mut user = User{name: "Ada"}
user = User{name: "Grace"} // allowed: binding-slot replacement
user.name = "Grace"        // rejected: User is not a reference
user.rename("Grace")       // rejected when rename mutates self

mut count = 1
count =+ 1                 // allowed: updates the writable count slot
```

Interior mutation requires an actual reference value:

```ard
let user = mut User{name: "Ada"}
user.name = "Grace"        // allowed
user.rename("Lin")         // allowed when rename mutates self
```

### No implicit borrowing

Every `mut T` destination requires a source expression whose value is already a
mutable reference. This rule applies uniformly to:

- function and closure parameters;
- bindings with an explicit `mut T` annotation;
- function returns;
- struct fields;
- assignment;
- `Maybe`, `Result`, lists, maps, channels, and other containers;
- generic inference and explicit generic arguments;
- function values and callbacks.

A writable ordinary binding is not sufficient:

```ard
fn take(user: mut User) {}

mut user = User{name: "Ada"}
take(user)     // rejected: user contains User
take(mut user) // allowed: explicit reference creation
```

The same requirement applies outside calls:

```ard
let user = User{name: "Ada"}
let reference: mut User = user     // rejected
let reference: mut User = mut user // allowed
```

This supersedes the implicit borrowing of `mut` bindings accepted by ADR 0022.
Mutation is visible either in the stored type or at the explicit `mut`
expression that creates the reference.

### Explicit borrowing depends on addressability, not binding mutability

An explicit `mut place` may reference stable, addressable storage regardless of
whether its binding is `let` or `mut`:

```ard
let user = User{name: "Ada"}
let reference = mut user // allowed
```

The checker must distinguish these operand modes:

1. **Existing reference** — copy an existing pointer-like reference handle.
2. **Addressable place** — create a reference targeting existing storage.
3. **Fresh value** — materialize stable storage and create a reference to it.
4. **Invalid place** — diagnose an expression that looks place-like but cannot
   be represented as stable addressable storage.

Addressable places include local and module-level storage and fields reached
through an addressable or reference-shaped base. Addressability is independent
from permission to write through the source name.

`mut` is idempotent on an existing reference value: if `reference` has type
`mut T`, both `reference` and `mut reference` evaluate to a copied `mut T`
handle. `mut reference` does not borrow the binding slot that stores the handle
and does not construct `mut (mut T)`. Ard-owned nested references and pure-Ard
construction of direct-Go `**T` arguments are unsupported initially; exact
multi-level foreign pointers may flow only where an already compatible foreign
value exists.

Whole value expressions continue to create fresh storage:

```ard
let literal = mut User{name: "Ada"}
let result = mut make_user()
```

They are equivalent to materializing a hidden stable temporary and referencing
it. Copy-producing accessors also reference a fresh copy rather than underlying
container storage:

```ard
let item = mut items.at(0) // does not reference items' element storage
```

A selector on a non-addressable temporary base remains rejected initially:

```ard
let field = mut make_user().profile // rejected by the initial implementation
```

Supporting that form would require an explicit decision to materialize the
projection rather than implying that the original temporary has addressable
field storage.

### References are first-class stored values

Reference flow preserves reference identity unless a boundary explicitly says
otherwise:

```ard
let first = mut user
let alias = first // alias stores a mutable reference, not a User snapshot
```

Unannotated inference preserves `mut T`. An explicit or contextual ordinary `T`
destination does not read or copy the referent:

```ard
let snapshot: User = first // rejected
```

There is no implicit `mut T -> T` conversion. Code must use the explicit
postfix `.@` expression defined below when it needs a top-level `T` value.

### Explicit dereferencing

As amended by ADR 0060, postfix `.@` is the canonical explicit dereference
syntax:

```ard
let reference = mut user // T -> mut T
let snapshot = reference.@ // mut T -> T
```

During the one-release migration window, prefix `deref reference` parses with
identical semantics, produces a deprecation warning, and is rewritten to
`reference.@` by the formatter.

The operation accepts only an actual reference value, removes exactly one outer
`mut` layer, evaluates its operand once, and produces a non-place value. It is
the Ard equivalent of evaluating Go `*p` where `p` has type `*T`, then using the
result as a value. The resulting copy is shallow, not deep.

```ard
let snapshot: User = reference.@ // allowed
consume_value(reference.@)       // allowed when consume_value expects User

reference.@ = replacement        // rejected: dereference is not an assignment place
reference.@.name = "Grace"       // rejected: temporary ordinary value
```

Shallow dereferencing means:

- later rebinding of the source reference slot or top-level referent replacement
  is not visible through the resulting value;
- reference-valued fields shallow-copy their pointer-like handles and continue
  pointing at the same current storage;
- a list copies its descriptor and initially shares its existing element backing
  storage: writes to existing shared indices are visible through both values,
  while growth changes only the grown descriptor and may detach it when storage
  is reallocated;
- a map copies its descriptor and continues sharing the same map contents, so
  insertion, replacement, and deletion are visible through both values;
- channels and foreign handles retain their intrinsic sharing behavior;
- structs, fixed arrays, primitives, and other immediate values copy their
  current value representation.

`.@` has the same precedence as calls and member access. Postfix operations
compose from left to right, while arithmetic, comparison, and other binary
operators bind less tightly. Existing `not` behavior remains broad and consumes
the comparison expression. Therefore:

```ard
reference.@.field        // select from the materialized value
reference.field.@        // materialize the reference-valued field
reference.@ + value      // (reference.@) + value
not reference.@ == value // not ((reference.@) == value)

let shallow_copy = (mut value).@
let independent_reference = mut reference.@
```

The compiler parser and Tree-sitter grammar must produce these same trees. The
`.` and `@` are adjacent; bare `@` remains invalid and reserved. Dot-leading
multiline chains remain valid.

`(mut value).@` explicitly makes a shallow value copy. `mut reference.@`
materializes the current shallow value in fresh stable storage and returns a new
reference to that top-level storage. Neither form recursively breaks identity
inside reference-valued fields or shared descriptors.

Dereferencing removes only its operand's outer reference layer and is never
recursive. A `(mut T)?` must first be unwrapped to `mut T`. Dereferencing a nil
foreign Go pointer has Go's ordinary panic behavior and never manufactures a
zero value.

`unsafe::cast<T>(value: Any)` remains a separate explicit checked conversion. It
may accept a boxed `*T`, shallow-copy `*p`, and return `none` rather than panic
when that pointer is nil. This does not reintroduce contextual dereferencing:
the cast call explicitly requests value type `T`. For concrete and supported
descriptor types, `unsafe::cast<mut T>` recovers the pointer handle and follows
the same pointer-copy/rebind behavior as every other reference.
`unsafe::cast<mut Trait>` remains unsupported initially because an arbitrary
dynamic pointer does not identify the intended trait contract or whether its
source identity is concrete storage or a replaceable trait slot.

Dereferencing `mut Trait` copies the current dynamic concrete value into
compiler-owned storage before constructing the ordinary `Trait` value. The
resulting trait object may use a pointer to that owned copy when its Go method
set requires one, but it never points back to the original reference's pointee
and does not observe later top-level replacement or reference-slot rebinding.
Reference-valued fields inside the dynamic concrete value retain normal shallow
pointer sharing.

Applying `.@` before an `Any` or Go-interface conversion deliberately selects
the ordinary-value ownership path. Applying it before an explicitly
value-shaped imported Go generic destination supplies that concrete value;
without `.@`, reference-aware inference and projection follow the rules in this
ADR.

ADR 0022's proposed `core::copy` has been dropped entirely: `.@` is a shallow
one-layer reference load, and Ard provides no recursive deep-copy operation.
Programs that need independent deep copies construct them explicitly.

Every operation that currently relies on an implicit dereference must migrate
to explicit `.@` or produce a diagnostic.

### Observational reads remain implicit

A reference may be observed without materializing an independent `T` value.
The following operations resolve through the referent:

- field and property reads;
- interpolation;
- pattern matching;
- compiler-defined read-only operations;
- method calls.

A user-defined method call is itself an explicit operation and may return a
value according to its declared signature:

```ard
let snapshot: User = reference.snapshot()
```

This does not restore a general implicit `mut T -> T` conversion. The method
body and signature explicitly define the materialization boundary.

Reference equality is not an observational read. Following Go pointer behavior,
`==` and `!=` on references compare current pointer identity. They do not compare
referent values:

```ard
let a = mut user
let b = a
let c = mut user

a == b // true: copied pointer handle
a == c // true: both explicit borrows point at the same user storage
```

To compare referent values, dereference explicitly:

```ard
a.@ == other.@
```

References do not support relational ordering.

### Binding assignment, interior mutation, and reference rebinding

Assignment behavior depends on the target category.

#### Ordinary binding slot

A `mut` binding containing `T` may replace its slot with another `T`. A `let`
binding may not.

#### Referent interior

A stored `mut T` permits field mutation and mutating method calls regardless of
whether the binding holding the reference is `let` or `mut`.

#### Reference copying and rebinding

References follow Go pointer-copy behavior. Copying a reference copies its
current pointer-like handle; it does not copy the referent:

```ard
mut a = mut first
let alias = a

a.name = "Changed"
alias.name // "Changed": both still point at first
```

Assigning another `mut T` to a writable reference-valued binding or field
replaces only that destination slot's handle. Existing copies are unaffected:

```ard
let second_reference = mut second
a = second_reference

a.name     // observes second
alias.name // still observes first
```

A `let` binding containing `mut T` cannot be rebound through that name. A `mut`
binding containing `mut T` can be rebound because its binding slot is writable.
Assignment never merges reference identities or changes other bindings that
previously copied the old handle.

Independent explicit borrows of the same place and ordinary copies of one
reference have the same observable pointer behavior: all initially point at the
same storage, pointee mutation is visible through all of them, and later
rebinding of any reference-valued slot affects only that slot.

This differs from replacing the value in the target storage itself:

```ard
mut user = User{name: "First"}
let reference = mut user
user = User{name: "Second"}
reference.name // "Second": reference still points at the user storage slot
```

Ard-owned references are non-nil when created. Foreign pointers may carry Go
`nil`; copying and rebinding them still follows the same pointer-value rule.

#### Whole-referent assignment

Ard source does not permit replacing a referent with an ordinary `T` through a
reference:

```ard
let reference = mut user
reference = User{name: "Grace"} // rejected
```

The restriction also applies to:

- `mut T` parameters;
- mutable receivers and `self`;
- reference-valued fields when the right-hand side is `T`;
- direct assignment through scalar references;
- compound assignment through scalar references;
- whole-list assignment through a list reference.

A scalar reference can be passed onward, including to FFI, but Ard source has no
direct scalar write through it:

```ard
let count_reference = mut count
count_reference = 10 // rejected
count_reference =+ 1 // rejected
```

This is a source-language restriction, not a capability guarantee across FFI.
Foreign code receiving a pointer may replace its pointee arbitrarily.

### Sanctioned interior operations

Operations defined as interior mutation remain valid through references even
when their backend implementation updates a descriptor:

- struct field mutation;
- mutating Ard methods;
- list `push`, `prepend`, `set`, `swap`, `sort`, and corresponding operations;
- map insertion and deletion;
- channel send/receive/close operations according to their existing type rules;
- `Maybe.set` and `Maybe.clear`;
- supported named Go slice/map methods and foreign mutating methods.

For example:

```ard
let values = mut [1, 2]
values.push(3) // allowed
values = [9]   // rejected: whole-referent assignment
```

ADR 0040's pointer-shaped native list representation remains useful because
sanctioned list methods must make descriptor growth visible. That representation
does not imply that arbitrary whole-list assignment is legal.

### Intrinsic channel handles

`Chan<T>`, `Sender<T>`, and `Receiver<T>` are intrinsic reference-like handles,
not mutable-reference values. Their existing operations remain available through
ordinary `let` bindings according to ADR 0019:

```ard
let channel = Chan::new<Int>()
channel.send(1) // allowed by channel-handle semantics
```

Sending, receiving, narrowing, and closing a channel do not require wrapping the
handle in `mut channel`. Binding mutability controls only replacement of the
channel handle itself. This is a named builtin capability exception, not a
general rule that descriptor-shaped ordinary values are interior-mutable.

### Reference-valued fields

A reference field stores an actual reference value:

```ard
struct Holder {
  current: mut User,
}
```

Interior access through the field follows the stored reference. Rebinding the
field requires writable access through its subject and a reference-valued
right-hand side:

```ard
let holder = mut Holder{current: mut first}
holder.current.name = "Ada"       // allowed interior mutation
holder.current = mut second       // replaces only the field's reference handle
holder.current = User{name: "X"} // rejected
```

A reference-valued field reached through an ordinary `let Holder` cannot be
rebound because the containing value is not interior mutable.

### Comparability and map keys

References are identity-comparable and hash by the address of their current
target storage, matching Go pointers. Pointee mutation does not change that
identity or hash. Backend adapter boxing must not create a different
source-level identity for two references to the same storage.
Therefore:

- `mut T` may be a map key;
- `mut T` satisfies Ard and Go `comparable` constraints;
- a reference counts as a comparable component under existing recursive map-key
  and compound-type eligibility rules;
- this decision does not add structural `==` for structs or change existing
  exclusions for `Maybe`, `Result`, unions, lists, maps, or `Any`;
- copying a reference preserves equality and hash identity;
- rebinding a writable reference slot changes the key represented by that slot
  but does not alter keys copied or inserted previously.

Trait and descriptor reference representations must carry the target storage's
stable comparable identity even when their forwarding payload is not itself
Go-comparable. Ard-owned and foreign references follow the same equality and
hashing rule.

Ard accepts Go's zero-sized pointer behavior: pointers to distinct zero-sized
variables may compare equal. The backend does not add padding solely to force
unique identity for `Void`, empty structs, or other zero-sized referents.

### Captures and escaping references

Referenced storage may escape and must remain stable. Backends must heap-lift or
otherwise preserve that storage as needed.

Closure lowering must distinguish:

1. copying an existing reference handle when the closure only observes or
   mutates its current pointee;
2. capturing a binding slot by reference when the closure assigns that slot,
   including rebinding a `mut` slot that stores a reference, or when
   `mut captured_value` is classified as `AddressablePlace`;
3. ordinary by-value capture.

These modes are not interchangeable. Copying a reference handle preserves its
current pointee but intentionally does not observe later rebinding of the outer
reference slot. If `mut captured_value` is classified as `ExistingReference`, the closure copies
its current handle and must not capture the slot merely because `mut` appears.
If the closure itself rebinds the captured reference binding and that write must
affect the outer binding, it must capture the slot rather than a copy of the
handle. Marking an ordinary value local as reference-shaped would change the
local's type.

By-reference slot requirements must propagate through nested closure capture
chains, and closure inlining must consume final capture metadata rather than
deciding before address-taken, slot-write, and rebinding uses are known.

An existing reference to a mutable ordinary slot observes later replacement of
that slot:

```ard
mut user = User{name: "First"}
let reference = mut user
user = User{name: "Second"}
reference.name // "Second"
```

### Async and goroutine boundaries

`async::start` applies the same capture rules as any other Ard closure; it does
not introduce a reference-isolation boundary:

- capturing an existing reference copies its current handle, so the task and
  caller share the pointee;
- explicitly borrowing outer addressable storage captures the required outer
  binding slot;
- rebinding an outer writable reference binding captures and updates that slot;
- nested closures propagate those capture requirements normally.

This follows ADR 0033's Go-like concurrency model. Go places no static
restriction on pointer access from goroutines. A pointer may be captured by a
goroutine or sent to one; when pointed-to local storage escapes, Go's escape
analysis arranges storage with a sufficient lifetime and the garbage collector
keeps it alive.

Lifetime handling does not provide synchronization. Concurrent access with at
least one write must be ordered by a happens-before edge, for example through a
channel, mutex, or atomic operation; otherwise the generated Go program has a
data race. Ard does not reject or implicitly synchronize such sharing. Race
safety remains the program's responsibility and can be checked with Go's race
detector.

### Go and FFI boundaries

#### `Any` and Go interfaces

A concrete `mut T` converted to `Any` or a named Go interface contributes its
current `*T` pointer. This preserves Go ABI compatibility, dynamic type `*T`,
pointer method-set satisfaction, and the mutation behavior required by ADR 0056
and issue #344. A descriptor reference similarly contributes its pointer-shaped
handle when the destination is `Any`.

A `mut Trait` wrapper is an internal Ard ABI value rather than Go `*Trait`. At
an `Any` or named empty-interface boundary, its projection method returns the
current dynamic concrete pointer, so Go observes dynamic `*Concrete`; the
interface copies that pointer and does not retain the wrapper adapter.

A general first-class `mut Trait` is rejected at named nonempty Go interface
destinations because conformance cannot depend on its runtime implementation.
The checker may accept an immediate concrete-to-trait expression only while it
retains static concrete provenance and can prove the concrete pointer's explicit
implementation and exact method set. Once the value has flowed through a trait-
typed binding, field, parameter, return, container, or generic, that proof is
lost and the conversion is rejected. A future fallible runtime interface cast
may relax this rule.

A `mut ForeignInterface` is different: it points to storage whose value is
already a Go interface descriptor. Passing it to that value interface requires
`interface_reference.@`, because Go `*Interface` does not implement
`Interface`. Passing it to `Any` without `.@` stores the pointer-to-interface
handle as its dynamic value. Exact imported Go `*Interface` parameters remain
unsupported as specified by ADR 0039.

Every interface conversion copies its selected current pointer or value. Later
rebinding of the Ard reference slot is not visible through an interface value
already created, while interior mutation of the shared pointee remains visible
through both sides.

ADR 0056's interface ownership rule remains in force, but its broader implicit
reference-to-value dereference rules are superseded. An ordinary concrete `T`
destination requires `reference.@`, including imported Go generic calls.

#### Imported Go generic parameters

A reference passed to an inferred Go generic parameter contributes its selected
static boundary representation rather than a referent value. For example, a
concrete Ard-owned `mut User` passed to `func Use[T any](T)` infers Go `T` as
`*User` and
passes its current pointer handle. Later rebinding of the Ard reference slot is
not visible through the Go value. If the Go type argument is explicitly or
contextually fixed to value `User`, passing bare `mut User` is rejected and
`reference.@` supplies the value.

This is distinct from an actual `Any`/interface destination, which performs the
interface conversion above while preserving dynamic type `*User`.

For a bare unconstrained Go generic parameter, `mut Trait` contributes its
static sealed wrapper-interface type because Go must choose `T` at compile time;
it cannot infer the runtime concrete pointer type. That wrapper is comparable,
retains its concrete/storage adapter through generic results and writable
generic containers, and exposes natural forwarding methods for compatible Go
method constraints. If the instantiated destination is an actual interface,
the interface conversion rule above projects the dynamic concrete pointer
instead.
A `mut ForeignInterface` similarly contributes its pointer-to-interface type to
a bare generic and requires `.@` when the instantiated destination is the
interface value type itself.

Generic descriptor parameters use this precedence:

1. Classify a parameter whose syntax or constraint is structurally slice/map
   shaped (`[]T`, `map[K]V`, `S ~[]E`, or the map equivalent) as an
   explicit-reference-required descriptor boundary before argument inference.
   Infer its generic arguments from the reference's referent shape and project
   the current descriptor, not a pointer to the descriptor.
2. For a bare unconstrained type parameter such as `T any`, an inferred
   reference argument contributes its selected static boundary representation:
   a concrete or descriptor pointer, a sealed `mut Trait` wrapper, or a
   pointer-to-interface for `mut ForeignInterface`. Thus `mut [Int]` infers
   `T = *[]int` and passes the current pointer-shaped target.
3. Explicit type arguments instantiate the signature first. If bare `T` is
   explicitly `[]int` or `map[K]V`, the resulting parameter is a descriptor
   boundary: it accepts an actual reference and projects the descriptor. If it
   is explicitly value-shaped `User`, `mut User` is rejected.

The same precedence applies to named slice/map types and their representable Go
constraints. For other bare constrained parameters:

- a reference satisfies Go `comparable` using its current pointer identity;
- a method/interface-constrained bare parameter infers the selected static
  boundary representation and validates that representation against the Go
  constraint: concrete references validate their pointer type, while `mut Trait`
  validates the natural methods exposed by its wrapper interface;
- a mixed type set containing both descriptor and non-descriptor alternatives
  is not preclassified as a descriptor boundary. It uses the argument's selected
  static boundary representation and ordinary constraint validation. A caller
  may explicitly choose
  a slice/map type argument to obtain descriptor projection after
  instantiation.

Only a parameter or constraint whose admissible shape is exclusively slice/map
is preclassified as a descriptor boundary. This ordering keeps generic
inference deterministic while preserving the exact instantiated Go ABI.

#### Exact direct-Go pointer and descriptor parameters

An imported Go signature keeps its exact ABI. Its Ard-facing boundary mode is
classified as follows:

- A representable single-level Go `*T` whose `T` is not an interface maps to a
  reference destination and requires an actual Ard or foreign reference. The
  current concrete pointer handle is passed directly. Go `*Interface` remains
  unsupported, and multi-level pointers require an already exact compatible
  foreign value because Ard cannot construct them.
- Every named or unnamed Go slice/map parameter conservatively requires an
  actual list/map reference because Go has no read-only slice/map parameter
  type. The backend projects the pointed-to descriptor value; this source
  requirement does not change `[]T` or `map[K]V` into a pointer ABI.
- Fresh literals remain usable only through explicit fresh reference syntax,
  such as `sort::Ints(mut [3, 1, 2])`.
- Go function and channel parameters remain ordinary values. Channel handles
  retain the intrinsic semantics defined above.
- `Any` and named Go interface parameters follow the specialized preceding
  matrix for concrete references, `mut Trait`, and `mut ForeignInterface`; they
  are not covered by the ordinary-descriptor rule.
- Pointer-to-descriptor Go parameters remain reference destinations and project
  the exact pointer shape required by the signature.

Every projection copies the current ABI value. Later rebinding of the Ard
reference slot is not visible to the Go callee or to a Go value retained from
that call. Mutation visibility follows Go's exact parameter shape: map-content
and slice-element
mutations can remain visible through shared backing storage, while replacing a
slice header is visible to Ard only when the Go ABI receives a pointer to that
header. These are explicit direct-Go boundary effects, not implicit Ard
reference-to-value conversions.

The importer/checker must represent the explicit-reference-required boundary
mode separately from raw Go ABI shape. It imposes the same source rule as a
`mut T` destination—only an actual reference qualifies—but lowering still emits
the exact descriptor value required by Go. A writable ordinary Ard binding never
qualifies implicitly.

#### Unified pointer-copy behavior

Ard-owned references and foreign Go pointers use the same copy/rebind rule:

```ard
mut pointer = ffi::first()
let alias = pointer
pointer = ffi::second()
```

`alias` keeps the original pointer. The same is true when `pointer` was created
with `mut value` instead of returned by FFI. Go fields, globals, parameters, and
returns retain their exact ABI forms; Ard adds no shared retargeting layer.

#### FFI trust boundary

Ard's source restriction against whole-referent writes cannot constrain foreign
code. A Go function receiving `*T` may replace `*p`, including for scalar
references. Such mutation is part of the explicit FFI trust boundary.

## Representation and compiler model

The compiler must stop deriving reference behavior from binding mutability or
backend pointer shape.

### Checker capabilities

Use separate judgments for:

- writable binding slot;
- reference-valued expression;
- permitted interior mutation;
- reference-destination compatibility;
- explicit-borrow addressability;
- explicit-dereference operand validity;
- writable reference-valued field slot;
- stable comparability.

The existing broad `isMutable` judgment must not continue to answer all of
these questions.

### First-class expression typing

A variable or field whose stored type is `mut T` retains that reference type as
an expression. It must not globally report `T` and rely on contextual recovery.
Observational operations resolve against the referent. Materializing `T`
without an explicit `.@` remains a type error.

Generic inference preserves reference types. A generic may bind to `mut T`, and
compound types such as `Maybe<mut T>` retain that shape through checker, AIR,
and target lowering.

### Storage provenance

Symbols and checked expressions must retain enough provenance to distinguish:

- local storage;
- value parameters;
- module-level and imported global storage;
- function declarations;
- function-valued storage;
- constants;
- foreign assignable variables;
- foreign values;
- addressable and non-addressable projections.

Type alone cannot decide addressability. Provenance must survive `ModuleSymbol`
and corresponding AIR lowering.

### Checker and AIR reference modes

Replace the current `MutableRefExpr.Fresh`/AIR boolean distinction with an
explicit mode at both layers:

```text
ExistingReference
AddressablePlace
FreshValue
```

Invalid places are diagnosed before AIR. The Go backend lowers by mode instead
of rediscovering ownership from expression shape.

`.@` is represented separately as a checked/AIR dereference expression, not
as another mutable-reference creation mode and not as an implicit contextual
conversion. Its result type is the operand's referent type and its assignment-
target classification is always non-place.

### Comparable trait and descriptor handles

Concrete references use their Go pointer directly as identity. List/map and
other descriptor references use a pointer to the referenced descriptor slot.
Mutable trait references use a sealed trait-owned interface with one of two
dynamic adapter shapes:

```text
ConcreteAdapter[T]{Target: *T}     // borrow of concrete storage
StorageAdapter{Target: *Trait}     // borrow of native-interface trait storage
StorageAdapter{Target: *any}       // borrow of fallback trait storage
```

Each adapter contains only one pointer and is therefore comparable. Go interface
comparison and hashing compare the canonical adapter type plus that pointer.
Independent conversions of the same concrete target instantiate the same
generic adapter type and pointer; independent borrows of the same trait slot use
the same storage-adapter type and pointer. Borrowing the concrete pointee and
borrowing a trait slot that currently contains it remain distinct identities,
as required by the source storage model.

The wrapper interface exposes sealed identity, shallow-load, and `Any`
projection methods plus the trait's natural Go methods when representable.
Concrete adapters forward through `*T`; storage adapters load the current trait
value on each call, so replacing a trait-typed slot remains observable. Native
traits dispatch through their ordinary Go interface. Fallback traits use a
trait-owned dispatch interface and collision-proof generated receiver methods,
which also avoids naming private implementation types across package boundaries.

Dereferencing a concrete adapter copies `*T` directly. Dereferencing storage
uses reflection to shallow-copy the current dynamic value. `Any` projection
returns the concrete pointer; when trait storage currently contains a value
implementation, the storage adapter promotes it to compiler-owned pointer
storage and writes that pointer back before returning it. There is no generated
function table, registry, or registration initializer.

### Assignment targets

The checker must represent or classify assignment targets as:

- ordinary binding slot;
- reference-valued binding slot;
- reference-valued field slot;
- referent interior;
- unsupported whole-referent target.

This distinction drives ordinary value replacement, reference-handle slot
replacement, interior mutation, and diagnostics.

## Implementation plan

### Phase 1: Lock semantics with tests

Add failing checker tests for the full four-state matrix:

- `let T`;
- `mut T`;
- `let mut T`;
- `mut mut T`.

Cover:

- ordinary slot assignment and scalar compound assignment;
- direct field mutation and mutating receiver calls;
- explicit borrowing from `let` and `mut` storage;
- idempotent `mut` on `ExistingReference`, rejected Ard-owned nested references,
  and unsupported pure-Ard construction of direct-Go `**T`;
- rejection of every implicit borrow into a `mut T` destination;
- reference flow through bindings, parameters, returns, fields, containers,
  `Maybe`, generics, callbacks, and function values;
- rejection of contextual `mut T -> T` materialization without `.@`;
- acceptance of `.@` at value bindings, arguments, returns, fields,
  containers, trait/interface boundaries, and explicitly value-shaped Go
  generic destinations;
- one-layer typing, single evaluation, postfix precedence/composition, non-place
  assignment rejection, and deterministic shallow field/collection sharing;
- observational fields, operators, interpolation, patterns, and methods;
- reference-valued field rebinding;
- whole-referent and scalar-reference write rejection;
- list whole-assignment rejection and sanctioned list method mutation;
- pointer-identity equality and inequality for copied, independently borrowed,
  and rebound concrete, descriptor, and trait references;
- reference map keys, stable hashes across pointee mutation, zero-sized Go
  pointer behavior, and comparable generic constraints for Ard-owned and foreign
  references.

Add parser, formatter, and Tree-sitter tests for `.@` as a postfix operator. Add
AIR tests for the three reference-creation modes, the dedicated dereference
expression, and reference-shaped generic and compound types.

Add runtime/backend tests for:

- local, parameter, field, global, and imported-global storage;
- fresh literals, calls, and copy-returning accessors;
- references escaping through returns, fields, and retained closures;
- nested and sibling captures;
- mutable captured slots containing ordinary values and references;
- copied references sharing pointee mutation while reference-slot rebinding
  affects only the rebound slot;
- later ordinary target-slot replacement observed through existing references;
- reference-valued field copying and rebinding;
- concrete-to-trait reference projections, canonical comparable adapter/target
  identities, copied wrappers, independent trait-reference rebinding, and
  escape;
- `mut Trait` conversion to `Any`/empty interfaces as dynamic `*Concrete`,
  immediate statically proven versus flowed named-nonempty interface conversion,
  bare generic wrapper inference, writable generic round trips, and rejected
  `unsafe::cast<mut Trait>`;
- sanctioned list, map, `Maybe`, and channel operations;
- rejected whole replacement;
- async reference-handle capture, outer-storage borrowing, reference-slot
  rebinding, and channel-synchronized mutation;
- `Any`, named Go interfaces, issue #344 behavior, and target-pointer snapshots;
- exact non-generic Go pointer, slice, map, channel, and pointer-to-descriptor
  parameter projections and their mutation visibility;
- generic `[]T`/`map[K]V`, exclusively slice/map-constrained type parameters,
  bare inferred `T any`, pointer-identity `T comparable`, method-constrained
  pointers, mixed descriptor type sets, and explicitly instantiated descriptor
  precedence;
- direct-Go field writes and value/pointer receiver method calls through
  ordinary bindings versus actual references;
- foreign pointer copy/rebind and explicit dereference behavior, including nil
  panic behavior;
- shallow dereference of structs, arrays, lists/maps, traits, `Any`, and Go
  interfaces;
- `unsafe::cast<T>` checked pointer materialization versus direct `.@` nil
  behavior, supported concrete/descriptor `unsafe::cast<mut T>` pointer-copy
  behavior, and rejected `unsafe::cast<mut Trait>`;
- explicit references to addressable foreign-interface storage, required
  `.@` at value-interface destinations, `Any` pointer-to-interface behavior,
  and rejection of exact imported `*Interface` parameters;
- FFI mutation of scalar pointees.

### Phase 2: Add dereference syntax and separate checker concepts

- Add parser, AST, formatter, and Tree-sitter support for `.@` at the same
  postfix-expression level as calls and member access.
- Add a checked dereference expression that accepts only actual references,
  strips one layer, evaluates once, and always produces a non-place value.
- Replace broad mutability checks with the capability judgments above.
- Centralize one reference-destination policy used by every contextual checker
  path.
- Make reference types first-class in expression typing.
- Add storage and declaration provenance to symbols and module symbols.
- Add explicit-borrow operand classification.
- Rewrite assignment checking around target categories.
- Add precise diagnostics before changing lowering.

### Phase 3: Introduce explicit reference modes

- Replace checker `Fresh` metadata with `ExistingReference`, `AddressablePlace`,
  and `FreshValue`.
- Preserve the mode in AIR.
- Add a dedicated AIR dereference node and lower it as a shallow read of the
  current referent, never as an addressable assignment target.
- Ensure reference-returning calls and fields copy existing reference handles
  rather than adding indirection.
- Add explicit checker/AIR conversion metadata for concrete-to-trait reference
  projections; do not encode them as an ordinary concrete pointer copy.
- Keep generic argument and compound-type identity reference-aware.

### Phase 4: Lower pointer-copy reference values

- Use direct Go pointer values for concrete Ard references wherever the exact
  referent representation permits it.
- Heap-lift or otherwise stabilize address-taken storage that escapes.
- Copy the current pointer-like handle when references flow through bindings,
  fields, parameters, returns, containers, and generics.
- Lower reference-to-reference assignment as ordinary destination-slot handle
  replacement; never update previously copied handles.
- Generate sealed trait wrapper interfaces with generic concrete adapters that
  capture `*Concrete` and storage adapters that capture the stable `*Trait` or
  `*any` slot; preserve pointer-copy/rebind behavior without function tables.
- Give every concrete, trait, and descriptor reference handle the target
  storage's stable comparable pointer identity for `==`, map keys, and generic
  constraints; adapter boxing must not change identity.
- Preserve pointer-shaped native list references where sanctioned list methods
  must update the referenced descriptor.
- Do not introduce union/find, shared retargeting cells, or reference-group
  merging.

### Phase 5: Correct closure and async handling

- Add capture modes for copied reference handles, address-taken/writable binding
  slots, and ordinary values.
- Capture a mutable reference binding slot when the closure rebinds it; copy the
  handle when the closure only accesses its current pointee.
- For `mut captured_value`, capture the outer slot only when operand
  classification is `AddressablePlace`; `ExistingReference` copies the handle.
- Propagate capture requirements through nested closures.
- Make inlining depend on finalized capture metadata.
- Apply those ordinary capture modes unchanged to `async::start` tasks; do not
  add an async-specific reference-isolation boundary.

### Phase 6: Lower boundary exceptions

- Lower concrete references as current `*T` pointers at `Any`, pointer-shaped Go
  generic, and compatible Go interface boundaries.
- Lower `mut Trait` as dynamic `*Concrete` at `Any`/empty-interface boundaries
  and as its static sealed comparable wrapper at bare generic destinations;
  reject named nonempty interface conversion after static concrete provenance
  has been lost.
- Lower `mut ForeignInterface` to `Any` as pointer-to-interface, require `.@`
  for value-interface conversion, and continue rejecting exact imported
  `*Interface` parameters.
- Reject bare references at explicitly value-shaped Go generic destinations and
  accept `reference.@` as the explicit concrete-value source.
- Confirm later reference-slot rebinding does not affect existing boundary
  values.
- Classify imported Go pointers and all named/unnamed slice/map parameters as
  explicit-reference-required boundaries; apply structural descriptor
  classification before generic inference and explicit-instantiation
  classification afterward; keep functions, channels, and interfaces on their
  dedicated rules.
- Project exact non-generic Go pointer and descriptor parameters according to
  that boundary mode and test Go-visible mutation.
- Keep the source reference requirement separate from raw Go ABI shape.
- Preserve exact foreign pointer ABI and Go pointer-copy behavior.
- Keep `unsafe::cast<mut Trait>` unsupported until forwarding-table
  reconstruction has an open-world design.
- Retain ADR 0056's owned-value versus existing-reference interface conversion
  modes.

### Phase 7: Migrate diagnostics and documentation

- Replace "mutable reference to immutable value" diagnostics with addressability,
  actual-reference-required, forbidden-whole-referent-write, and immutable-slot
  diagnostics as appropriate.
- Update the variables, functions, structs, generics, async, and Go interop
  guides.
- Document `.@` as the explicit shallow value operation and keep it distinct
  from any deep-copy notion.
- Add migration examples using `.@` for every formerly implicit
  reference-to-value conversion.

### Phase 8: Validate and migrate downstream code

Run:

- targeted checker, AIR, formatter, and Go backend tests;
- `go test ./...` from `compiler/`;
- formatter verification for every touched Ard fixture;
- `gopls` diagnostics;
- website build;
- downstream reference-heavy projects.

Audit existing source for:

- implicit passing of mutable bindings to `mut T`;
- direct interior mutation through ordinary `mut T-value` bindings;
- implicit dereferences from reference expressions that must become explicit
  `.@` expressions;
- whole-referent writes through references;
- referent-value equality on references that must become `a.@ == b.@`.

Because these semantics are intentionally breaking, migration should be explicit
rather than hidden in compatibility coercions.

## Corrections and supersessions

This ADR supersedes or clarifies the following prior decisions and proposals.

### ADR 0022: Use `mut` for Mutable References

Superseded:

- a `mut` binding implicitly satisfies a `mut T` parameter;
- mutable-reference creation requires a binding marked `mut`;
- immutable bindings are rejected merely because their slots are not writable;
- ordinary value contexts implicitly read/copy a reference;
- whole-referent assignment through a mutable reference is generally available.

Retained:

- `mut T` is a finite reference/indirection boundary;
- references may escape and require stable storage;
- explicit reference creation makes the initial alias to storage visible;
- copying and rebinding references follows ordinary pointer-value behavior;
- references may cross goroutine boundaries under ADR 0033's Go-like
  synchronization and data-race responsibilities;
- explicit deep-copy semantics remain desirable but deferred; shallow
  reference-to-value conversion is now the `.@` expression.

### ADR 0023: Represent Mutable Trait References with Forwarding Tables

Superseded or clarified:

- concrete or trait values do not implicitly borrow into `mut Trait`;
- backend-private whole-value `assign` and implicit mutable-to-immutable trait
  coercion are not source-level reference operations;
- whole-referent assignment through `mut Trait` is rejected in Ard source;
- mutable trait wrapper adapters use the same pointer-copy and destination-slot
  rebinding rules as concrete references.

A concrete-to-trait reference conversion is a type-widening boundary. Upcasting
`mut Leaf` to `mut View` creates a comparable concrete adapter that captures the
reference's current `*Leaf` storage. Copying that trait reference copies the
adapter and preserves the same pointee. Rebinding either the original concrete
reference slot or a trait-reference slot does not change wrappers copied
previously. Rebinding a writable `mut View` slot may select another `View`
implementation without changing narrower `mut Leaf` aliases.

Retained:

- mutable trait references require trait-specific adapter forwarding for
  interior method dispatch;
- forwarding must reach stable original storage and remain sound when escaping;
- copy-in/copy-out pointers to temporary interface values are unsound;
- foreign and immutable trait-object representations remain separate concerns.

The implementation test matrix must cover concrete-to-trait upcast, copied trait
wrappers, later concrete-reference rebinding, heterogeneous trait-slot
rebinding, escaping adapters, and pointee mutation through every copy.

### ADR 0030: Use Direct Go Struct Values and Fields

This proposed ADR is superseded where it states that a Go `*T` result implicitly
dereferences into an annotated value `T` or cannot be stored, wrapped, captured,
or rebound.
Under ADR 0057:

- a Go pointer result remains a foreign reference value;
- ordinary `T` destinations require an explicit `.@`;
- reference-preserving bindings, fields, returns, containers, captures, and
  generic contexts may store it when their types accept the foreign reference;
- mutable bindings may rebind foreign pointers;
- copies and later rebinding retain normal Go pointer-copy semantics;
- binding a direct-Go value in a `mut` slot does not permit field mutation or
  pointer-receiver calls; those operations require an actual foreign
  pointer/reference, which may be created explicitly from addressable `let` or
  `mut` storage;
- direct-Go field access and exact ABI behavior remain as specified by ADR 0030
  where they do not conflict with these rules.

### ADR 0031: Go Backend Lowering Contract

Superseded or clarified:

- mutable-reference arguments must be actual references and fresh list/map
  values do not implicitly satisfy them;
- closures require explicit capture metadata when a writable/address-taken slot
  cannot be represented by copying its current value;
- mutable trait references retain the forwarding representation required by ADR
  0023 and this ADR rather than relying only on ordinary Go interfaces;
- references and transitively comparable reference-containing values use stable
  pointer identity for map keys and Go `comparable` lowering.

Retained:

- AIR remains the backend contract boundary;
- generated output should use idiomatic Go wherever it can preserve Ard
  semantics;
- ordinary values, package/module mapping, exact direct-Go ABI forms, `Maybe`,
  `Result`, and non-reference control flow keep their existing lowering rules;
- synthetic helpers require explicit semantic justification and must not hide
  unfinished lowering.

### ADR 0036: Define `Any` Casting Policy

Clarified:

- `unsafe::cast<T>(value)` is an explicit checked conversion and may continue to
  shallow-dereference a boxed non-nil `*T` into `T`;
- unlike direct `.@`, a nil boxed pointer returns `none` because the cast is
  fallible by contract;
- `unsafe::cast<mut T>` recovers the existing pointer handle for concrete and
  supported descriptor targets, and copying or rebinding that result follows
  ADR 0057's pointer-value rules;
- `unsafe::cast<mut Trait>` remains unsupported until forwarding metadata can be
  reconstructed soundly from open-world dynamic values;
- ordinary contexts outside this explicit cast never dereference a reference
  implicitly.

### ADR 0039: Support Explicit Go Interface Interop

Clarified:

- Ard may internally reference addressable foreign-interface storage as
  `mut ForeignInterface`;
- the reference can flow to `Any` as a pointer-to-interface handle and can be
  explicitly `.@`ed into the current interface descriptor;
- a bare `mut ForeignInterface` does not satisfy the corresponding value
  interface because Go `*Interface` has no interface method set;
- this internal Ard capability does not add direct imported Go `*Interface` ABI
  support, which remains deferred.

### ADR 0040: Decouple Mutability from Go Pointer Lowering

Superseded or clarified:

- binding mutability must not choose reference representation;
- descriptor-backed representation does not imply implicit borrowing;
- native list pointer shape supports sanctioned interior methods, not
  whole-list assignment through a reference;
- concrete Ard references should use direct pointer-value lowering where
  possible, while descriptor references preserve the indirection needed to
  update their target descriptor.

Retained:

- exact direct-Go ABI shapes;
- maps, channels, foreign slices/maps, and pointers follow their documented Go
  boundary representations;
- descriptor-only foreign parameters cannot replace caller descriptors unless
  an explicit supported slot reference exists.

### ADR 0045: Support Explicit Mutable Reference Expressions

Superseded:

- only `mut` bindings and fields reached through mutable access are valid
  explicit-reference places;
- `let` bindings are rejected as immutable storage;
- implicit passing from `mut` bindings remains valid;
- ordinary binding/value contexts silently snapshot referents;
- reference bindings cannot be rebound regardless of binding mutability;
- whole-list writes through mutable references are allowed.

Retained:

- `mut expression` is the explicit reference-producing syntax;
- whole value expressions may materialize fresh stable storage;
- existing references propagate by copying their pointer-like handle without
  accidental extra indirection;
- reference copying follows pointer-value behavior;
- list/map accessor results are copies rather than element places;
- Go lowering must preserve stable escaping storage.

### ADR 0052: Adopt Structured Labeled Diagnostics

The structured-diagnostic architecture remains accepted. The checklist item and
diagnostic family described as "mutable references to immutable values" is
superseded. Diagnostics must instead distinguish:

- non-addressable explicit-reference operands;
- non-reference operands supplied to `.@`;
- ordinary values supplied to reference destinations;
- immutable binding-slot assignment;
- forbidden whole-referent writes;
- forbidden scalar writes through references;
- rebinding an immutable reference-valued slot;
- invalid relational comparison of references.

### ADR 0056: Preserve Ard Value Semantics When Lowering Go Interfaces

Retained:

- ordinary `T` values contribute values or interface-owned boxes;
- actual mutable references contribute their existing target identity;
- `Any`, named empty interfaces, and named nonempty interfaces use explicit
  conversion metadata;
- Go generic concrete destinations remain distinct from interface
  destinations.

Superseded or clarified:

- a writable ordinary binding is not an existing reference and cannot be
  treated as one without explicit `mut` syntax;
- implicit `mut T -> T` dereferences in bindings, parameters, returns, fields,
  and other value contexts are rejected; `reference.@` is the explicit
  shallow value operation;
- passing a reference to an inferred imported Go generic contributes its
  selected static boundary representation: concrete pointer, descriptor pointer,
  mutable-trait wrapper interface, or pointer-to-interface;
- a generic explicitly fixed to value `User` rejects bare `mut User` and accepts
  `reference.@`;
- concrete references convert to compatible interfaces by copying `*T`;
  `mut Trait` and `mut ForeignInterface` follow the specialized static/dynamic
  boundary matrix defined above;
- future reference-slot rebinding is not reflected through a boundary value
  already created;
- Ard-owned and foreign references follow the same pointer-copy/rebind rule.

## Consequences

- Binding mutability has one meaning: permission to write the binding slot.
- A reference value carries mutable interior access and a pointer-like handle;
  copying it preserves the current pointee while rebinding affects only the
  destination slot.
- Mutation at reference destinations becomes explicit and type-directed.
- `let` no longer promises deep immutability; it prevents writes through that
  binding name while references may mutate reachable storage.
- Existing source that relies on implicit mutable borrowing will break.
- Existing source that mutates fields or calls mutating methods through an
  ordinary `mut T-value` binding will require an explicit reference.
- Existing implicit reference-to-value conversions must migrate to explicit
  `.@` expressions.
- Concrete references can generally lower to raw Go pointers; trait and
  descriptor references need only the representation required for forwarding
  or descriptor update, not shared retargeting cells.
- Ard and Go references share the same pointer-copy/rebind intuition, while
  exact FFI ABI and nil behavior remain documented boundary concerns.
- Async tasks may share references and outer binding slots exactly like ordinary
  closures; Ard adds no data-race protection or implicit synchronization.
- References use pointer identity for equality, map keys, hashing, and
  comparable constraints; referent-value comparison requires `.@`.
- Diagnostics become more precise because slot writability, addressability,
  reference type, and interior mutation are no longer conflated.

## Deferred work

- Deep-copy semantics (ADR 0022's proposed `core::copy`) were considered and
  dropped: `.@` is deliberately shallow, and programs that need independent
  deep copies construct them explicitly.
- Consider whether temporary selectors should materialize fresh projected
  storage.
- Consider optional concurrency diagnostics or race-tooling integration without
  restricting legal Go-style reference sharing. Plumbing Go's race detector
  through the CLI is backlogged as issue #351.
- Auto-deref at ordinary value destinations was considered and rejected
  (issue #348): Go never dereferences implicitly where a value is expected,
  and the explicit `.@` keeps mutation-contract changes visible at every
  call site. Implicit reads remain limited to the observational set (fields,
  non-mutating methods, arithmetic, interpolation, match subjects, and
  conditions).

## Related

- `docs/adrs/0019-use-typed-channels-for-fiber-communication.md`
- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0023-represent-mutable-trait-references-with-forwarding-tables.md`
- `docs/adrs/0030-use-direct-go-struct-values-and-fields.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0036-define-any-casting-policy.md`
- `docs/adrs/0039-support-explicit-go-interface-interop.md`
- `docs/adrs/0040-decouple-mutability-from-go-pointer-lowering.md`
- `docs/adrs/0045-support-explicit-mutable-reference-expressions.md`
- `docs/adrs/0052-adopt-structured-labeled-diagnostics.md`
- `docs/adrs/0055-use-call-local-constraints-for-ard-generic-inference.md`
- `docs/adrs/0056-preserve-ard-value-semantics-when-lowering-go-interfaces.md`
- `docs/go-interface-semantics.md`
