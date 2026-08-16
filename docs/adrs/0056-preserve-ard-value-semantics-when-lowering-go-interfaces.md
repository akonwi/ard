# 0056: Preserve Ard Value Semantics When Lowering Go Interfaces

## Status

Accepted

Amended by ADRs 0057, 0060, and 0061. Implicit `mut T -> T` dereferencing at value destinations
(the Go equivalent of using `*p` where `p` has type `*T`) is removed, including
for concrete imported Go generic destinations. Ard now spells that operation
`reference.@`. Binding or passing the resulting value makes the target's
normal shallow value copy; this is not a deep copy operation. Concrete
references contribute `*T` at compatible boundaries. ADR 0061 supersedes ADR
0057's mutable-trait forwarding rules: `Trait` and `mut Trait` share one native
Go interface, mutable-to-ordinary trait conversion is representation-free, and
`.@` is invalid for mutable traits. `mut ForeignInterface` retains its separate
pointer-to-interface behavior. Later rebinding of the
source reference slot is not visible through a boundary value already created.
Ard-owned and foreign references use the same pointer-copy/rebind behavior.
Examples below that pass a reference to an ordinary value destination or assert
an implicitly dereferenced value are historical and not normative.

## Context

Ard distinguishes a binding slot from the value stored in that slot. A leading
`mut` on a binding makes the slot reassignable; it does not create a mutable
reference:

```ard
mut value: W = W{}
```

The expression `value` still has value type `W`. By contrast, `mut T` in a type
position is a mutable reference, and `mut <expression>` creates such a
reference:

```ard
let reference: mut W = mut W{}
```

The ordinary parameter rules are:

| Parameter | Argument | Result |
| --- | --- | --- |
| `T` | `T` | allowed |
| `T` | `mut T` | rejected; use `reference.@` to produce `T` |
| `mut T` | `T` | rejected |
| `mut T` | `mut T` | allowed while preserving reference identity |

Binding-slot reassignability does not change this matrix.

These semantics translate directly for ordinary Ard functions and most Go
values. A Go interface introduces a representation mismatch because Go
interface satisfaction depends on generated method sets. An Ard-owned type can
explicitly implement a Go interface with mutating methods:

```ard
use go:net/http

struct W {
  header: http::Header,
  body: [Byte],
}

impl http::ResponseWriter for W {
  fn header() http::Header {
    self.header
  }

  fn mut write(bytes: [Byte]) Int!Str {
    self.body = bytes
    Result::ok(bytes.size())
  }

  fn mut write_header(code: Int) {}
}
```

The explicit implementation belongs to the Ard type `W`. On the Go target,
however, mutating methods require pointer receivers:

```go
func (w W) Header() http.Header
func (w *W) Write([]byte) (int, error)
func (w *W) WriteHeader(int)
```

Consequently, generated Go `W` does not satisfy `http.ResponseWriter`, while
`*W` does.

Issue #338 exposed inconsistent checker behavior around this boundary. An Ard
function call can accept a source that a direct imported Go call rejects, even
when both destinations have the same foreign-interface parameter type. The
underlying question is not whether Ard's value/reference semantics should
change. It is how those semantics should be represented when Go requires a
pointer inside an interface value.

ADRs 0039 and 0045 currently make generated pointer method sets visible in Ard
call compatibility. They require mutable, addressable caller storage for an
interface implementation containing mutating methods. That rejects ordinary
Ard values even though the parameter is not declared as a mutable reference.

Go interfaces are exceptional value containers rather than ordinary concrete
value destinations. An interface carries an observable dynamic concrete type
and value. That dynamic value may own a concrete value or preserve reference
identity. The interop policy must therefore decide both eligibility and
ownership instead of forcing interfaces through the ordinary `T` parameter
matrix.

## Decision

Go interface conversion will accept both Ard values and Ard mutable references
when their nominal type has the required explicit implementation. The source
form determines ownership: a value contributes an owned copy, while a mutable
reference preserves its existing identity. Generated Go pointer requirements
remain a lowering concern and do not change source-level eligibility.

### Value interface parameters

A parameter declared as a foreign Go interface is a value destination:

```ard
fn consume_response_writer(writer: http::ResponseWriter) {}
```

An Ard-owned `W` may flow to that parameter when `W` has an explicit
`impl http::ResponseWriter`, regardless of whether generated implementation
methods require value or pointer receivers:

```ard
let value: W = W{header: headers, body: []}
mut reassignable_value: W = W{header: headers, body: []}
let reference: mut W = mut W{header: headers, body: []}

consume_response_writer(value)
consume_response_writer(reassignable_value)
consume_response_writer(reference)
```

All three calls are valid. The reassignability of the second binding has no
role in interface compatibility.

Unlike an ordinary concrete `T` destination, a Go interface destination
preserves an existing `mut W` reference. The interface is an existential
container capable of carrying reference identity; it is not a request to read
the source as a concrete `W` value.

The ownership rule is:

| Ard source | Interface payload | Mutation target |
| --- | --- | --- |
| `W` value | compiler-owned copy | owned copy |
| reassignable slot containing `W` | compiler-owned copy | owned copy |
| existing `mut W` | existing reference | original referent |
| explicit `mut <place>` | explicit reference | original place |

Under ADR 0057, a caller with `mut W` requests the ordinary-value ownership
path by dereferencing explicitly:

```ard
let value: W = reference.@
consume_response_writer(value)
```

This is a shallow value dereference; Ard provides no deep-copy operation.

### Pointer-required Go representation

When generated Go `W` directly satisfies the target interface, a `W` source is
stored as dynamic `W`. A `mut W` source is stored as dynamic `*W`, which also
has `W`'s value-receiver method set and preserves the existing reference.

When only generated `*W` satisfies the interface, the backend materializes
compiler-owned storage for a `W` source and stores a pointer to that copy in the
interface:

```go
valueCopy := value
consumeResponseWriter(&valueCopy)
```

For a `mut W` source, lowering preserves the existing pointer:

```go
consumeResponseWriter(reference)
```

Mutations through an interface created from a `W` value affect only the
interface-owned copy. Mutations through an interface created from `mut W`
affect the original referent. If Go retains the interface, ordinary Go escape
analysis gives either storage form an appropriate lifetime. Owned boxes have no
copy-back or writeback phase.

Both pointer-required forms have observable Go dynamic type `*W`. Go reflection
and type assertions cannot distinguish their ownership, and do not need to;
ownership and aliasing are explicit in the Ard source type. This representation
does not constitute an implicit Ard conversion from `W` to `mut W`.

### Observable Go interface behavior

Interface boxing is not an invisible storage optimization. When a `W` source
is boxed as `*W`, Go observes dynamic type `*W`: assertions to `*W` succeed,
assertions to `W` fail, reflection reports `*W`, and interface equality uses
pointer identity. Two independent boxes of equal `W` values therefore normally
compare unequal. These effects are accepted consequences of adapting Ard's
explicit implementation to Go's pointer method set.

Existing interface-to-interface conversions preserve the concrete dynamic type
and value rather than nesting one interface inside another. Nil interfaces,
typed nil pointers, comparison panics for uncomparable dynamic values, and
constraint-only general interfaces continue to follow Go's own rules. The
supporting details are recorded in `docs/go-interface-semantics.md`.

### Mutable-reference destinations

ADR 0057 makes the explicit-reference matrix authoritative:

```ard
fn take_value(value: W) {}
fn take_reference(value: mut W) {}

let value: W = W{}
let reference: mut W = mut W{}

take_value(value)               // allowed
take_value(reference)           // rejected
take_value(reference.@)     // allowed by explicit shallow dereference
take_reference(value)           // rejected
take_reference(reference)       // allowed, pointee identity preserved
```

Addressable foreign-interface storage may itself be referenced explicitly with
`mut interface_value`. The resulting `mut ForeignInterface` is a pointer-like
reference to that interface storage and follows ADR 0057's copy/rebind rules.
`.@` copies the current interface descriptor; it does not follow later
reference-slot rebinding, while its dynamic payload retains normal Go sharing.
A bare `mut ForeignInterface` cannot satisfy the corresponding value interface,
because Go `*Interface` has no interface method set; use `.@`. Passing the
bare reference to `Any` stores the pointer-to-interface handle. Exact imported
Go `*Interface` parameters remain unsupported by ADR 0039. This is distinct
from converting a `mut W` source into an ordinary foreign interface value, which
stores the current concrete pointer as its dynamic value.

### Go-owned sources

Go-owned concrete values continue to satisfy Go-owned interfaces according to
Go assignability, as established by ADR 0039. This decision changes the adapter
policy for Ard-owned explicit implementations; it does not replace Go's own
assignability rules for Go-owned values.

### `Any` and named empty Go interfaces

Ard `Any` and named empty Go interfaces are runtime interface destinations and
use the same ownership rule. An ordinary `T` value contributes its value, while
an existing `mut T` contributes its reference identity. No explicit `impl` is
required because these interfaces have no method contract:

```ard
fn store(value: Any) {}

fn forward(value: mut $T) {
  store(value) // the interface contains the existing reference
}
```

A Go generic parameter constrained by `any` is different. An inferred reference
argument to `func accept[T any](value T)` contributes its selected static
boundary representation: a concrete/descriptor pointer, or under ADR 0061 the
same native Go interface for `Trait` and `mut Trait`. If `T` is explicitly
fixed to the concrete value type, the caller must pass `reference.@`; bare
`mut T` is rejected. If `T`
is explicitly or contextually resolved to Ard `Any`, the destination is an
actual interface and preserves reference identity.

### Explicit implementation remains required for nonempty interfaces

An Ard-owned value still requires an explicit foreign-interface implementation
to satisfy a named nonempty Go interface:

```ard
impl http::ResponseWriter for W { ... }
```

Matching generated method shapes without an explicit implementation does not
establish Ard-level conformance. `Any` and named empty interfaces are exempt
because every Go value satisfies their empty method set.

### Contextual conversion pipeline

Foreign-interface conversion should participate in a shared contextual
conversion pipeline rather than being reimplemented by individual call paths.
At declared destinations, the checker should:

1. contextually construct literals and closures that require an expected type;
2. check the source expression without converting it;
3. collect and solve generic evidence from the unconverted source;
4. substitute solved source and destination types;
5. enforce exact foreign ABI restrictions where applicable;
6. classify interface sources before value conversion so an existing `mut T`
   can preserve its current pointee identity;
7. reject bare references in non-interface value contexts, accepting only an
   explicit checked `.@` expression as a shallow referent value;
8. plan owned-box or existing-reference interface representation;
9. apply function or other adapters;
10. validate the final destination type.

The pipeline must distinguish:

- value type;
- storage type;
- mutable-reference identity;
- binding-slot reassignability;
- fresh compiler-owned storage;
- exact foreign ABI contexts.

Binding-slot reassignability must never substitute for `mut T`.

Conversion planning must not begin by calling the current broad
`areCompatible`, because that relation already accepts some operations that
require explicit representation, including `.@` and some foreign
conversions.

Every representation-changing conversion should be explicit before backend
lowering. Foreign-interface conversion must distinguish an owned value box from
an existing mutable reference so the Go backend does not rediscover ownership
from expression shape or types.

### Relationship to ADRs 0039 and 0045

This decision supersedes ADR 0039's requirement that every pointer-receiver
implementation source be mutable and addressable. It retains ADR 0045's
identity-preserving explicit-reference behavior while also permitting ordinary
values through owned boxing. As amended by ADR 0057, ordinary concrete value
destinations reject bare `mut T`; `reference.@` explicitly produces `T`.
Go interfaces remain an existential conversion boundary that may instead
preserve an actual reference's identity.

The difference is summarized below:

| Question | ADRs 0039/0045 today | This decision |
| --- | --- | --- |
| Can immutable `W` flow to a value interface when implementation needs pointer receivers? | No | Yes |
| Does a reassignable `W` slot grant interface conversion? | Effectively yes when treated as mutable/addressable | No special role |
| Does `mut W` passed to a value interface preserve caller identity? | Yes | Yes |
| Why does Go receive `*W`? | Source supplies caller-owned mutable storage | Value sources are boxed; reference sources supply existing storage |
| Can interface mutation update the original caller value? | Only through accepted mutable/addressable sources | Yes for `mut W`; no for `W` |
| Does generated pointer method-set shape affect Ard call compatibility? | Yes | No |
| Is compiler-owned boxing required? | No | Yes for `W` when only `*W` satisfies the interface |

ADR 0039 should be amended to replace:

> If any required impl method is mutating and therefore lowers with a pointer
> receiver, the value must be mutable and addressable at the upcast/call site.

with the value-interface conversion policy in this decision.

ADR 0045 should be amended so its `http::Handler` example no longer implies
that explicit `mut <place>` is required to make an Ard-owned value eligible for
a value interface parameter. Explicit `mut` continues to control whether the
interface preserves caller identity; an ordinary value instead contributes an
owned box.

## Alternatives Considered

### Require `mut T` for pointer-receiver interface implementations

Under this policy:

```ard
fn consume(writer: http::ResponseWriter) {}

consume(value)     // rejected
consume(reference) // allowed, original identity preserved
```

This matches the current ADR 0039/0045 interface rule and avoids hidden boxing,
but it lets generated Go receiver representation override the declared Ard
value destination. It makes a value interface parameter behave differently
from other value parameters and was rejected for that reason.

### Accept mutable binding slots as addressable values

Under this policy, `mut value: W` could be passed where an implementation needs
`*W`, while `let value: W` could not. This conflates slot reassignability with
mutable-reference identity and contradicts Ard's binding/type distinction. It
was rejected.

### Snapshot both `T` and `mut T`

This would box a `T` value copy and first snapshot a `mut T` source into another
owned box. It gives all conversions ownership semantics, but discards reference
identity that is explicit in Ard source. It was rejected because Go interfaces
can carry references and Go code does not need to distinguish whether Ard owns
or borrows the pointed-to storage.

### Generate value-receiver methods that mutate copies

Emitting mutating Ard implementation methods as Go value receivers could make
Go `W` satisfy the interface directly, but mutations would target a receiver
copy and could silently disappear. It would also violate Ard mutating-method
semantics. It was rejected in favor of an explicit interface-boxing adapter.

## Consequences

- Go interfaces are an explicit exception to ordinary concrete `T` destination
  normalization because they can carry owned values or reference identity.
- Generated Go pointer method sets no longer leak into Ard call compatibility.
- Ard `Any`, named empty Go interfaces, and named nonempty Go interfaces share
  the same value-versus-reference ownership policy.
- Go generic parameters constrained by `any` remain concrete destinations after
  inference: bare references infer their selected static boundary representation,
  while explicitly value-shaped destinations require `.@`.
- Ard calls and direct imported Go calls use the same interface conversion
  policy.
- A value-interface conversion may allocate compiler-owned storage when only a
  generated pointer type satisfies the Go interface.
- Mutations through an interface created from `W` affect its owned box; mutations
  through one created from `mut W` affect the original referent.
- Interface values retained by Go may cause the boxed storage to escape to the
  heap.
- Reassignable `W` bindings no longer imply reference semantics; callers use an
  actual `mut W` or explicit `mut <place>` when identity must be preserved.
- The checker, AIR, and Go backend need explicit owned-box and existing-reference
  foreign-interface conversion modes.
- ADRs 0039 and 0045 are amended to defer runtime interface ownership and representation to this decision.
- Other ordinary Go value, pointer, scalar, collection, and callback boundaries
  retain their existing Ard value/reference rules. The special adaptation is
  narrowly required by Go interface method-set representation.

## Implementation Plan

1. Correct issue #338's regression coverage to distinguish reassignable `W`
   storage from actual `mut W` references.
2. Add checker tests proving `W` and `mut W` both satisfy a value interface when
   an explicit implementation exists.
3. Add negative tests for missing explicit implementations and for ordinary
   `T -> mut T` parameter conversion.
4. Introduce a contextual conversion classifier that separates identity,
   conversion-required compatibility, and final validation.
5. Reject contextual `mut T -> T` reads and recognize ADR 0057's explicit
   `.@` expression as the ordinary-value source before interface conversion.
6. Add explicit checker and AIR conversion modes for compiler-owned interface
   boxing and existing-reference interface conversion.
7. Lower pointer-required `W` conversions through stable temporary storage with
   no copy-back, and `mut W` conversions through their existing reference.
8. Preserve existing concrete dynamic values for interface-to-interface
   conversions rather than boxing or nesting the source interface.
9. Route direct Go calls, Ard calls, function values, annotated bindings,
   assignments, declared returns, and typed fields through the shared policy.
10. Preserve separate Go generic inference and exact foreign ABI validation.
11. Amend ADRs 0039 and 0045 when this ADR is accepted.
12. Validate the full compiler suite and downstream projects using Go
    interfaces, including Dram and Maestro.

## Validation

At minimum, implementation must cover:

- `W -> W`;
- rejection of bare `mut W -> W` and acceptance of `reference.@`;
- rejection of `W -> mut W`;
- identity preservation for `mut W -> mut W`;
- immutable and reassignable `W` bindings flowing to a value Go interface;
- `mut W` flowing to a value Go interface while preserving identity;
- explicit implementations containing only non-mutating methods, including
  dynamic `W` for a `W` source and dynamic `*W` for a `mut W` source;
- explicit implementations containing mutating methods and requiring boxed
  pointer representation;
- interface mutation affecting a boxed `W` copy but not caller storage;
- interface mutation through `mut W` affecting the original referent;
- `.@` of `mut W` selecting ordinary-value owned boxing;
- interface-to-interface conversion preserving dynamic type, typed nils, and
  identity without nesting or re-boxing;
- a Go function retaining the resulting interface beyond the call;
- direct imported Go calls and equivalent Ard function calls selecting the same
  conversion plan;
- calls through imported and Ard function values;
- annotated bindings, assignments, returns, and typed fields;
- generic calls where interface conversion occurs after inference;
- exact interface-method ABI declarations rejecting adapters that would change
  the required Go method signature;
- missing explicit implementation diagnostics;
- no conversion decision depending on binding-slot reassignability;
- checker, AIR, and generated Go conversion-shape parity.

## Related

- GitHub issue #338
- `docs/go-interface-semantics.md`
- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0039-support-explicit-go-interface-interop.md`
- `docs/adrs/0040-decouple-mutability-from-go-pointer-lowering.md`
- `docs/adrs/0045-support-explicit-mutable-reference-expressions.md`
- `docs/adrs/0055-use-call-local-constraints-for-ard-generic-inference.md`
- `compiler/checker/checker.go`
- `compiler/checker/nodes.go`
- `compiler/air/nodes.go`
- `compiler/air/lower.go`
- `compiler/go/lower.go`
