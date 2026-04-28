# ADR: Go Target Trait Lowering

## Status

Accepted

## Context

Ard traits are one of the more promising Ard→Go mappings because Go already has interfaces.

The main concerns are not whether Go has a matching host concept, but how to handle:

- method sets
- trait-typed parameters, returns, locals, and collections
- coercions/conversions
- dispatch shape
- interaction with generated stdlib, unions, and FFI

The Go target still needs to stay aligned with the broader long-term goal of:

- emitting ordinary readable Go
- avoiding `ardgo` runtime helpers for ordinary language features
- keeping the emitter straightforward and mostly mechanical

The lessons from the `match` and `result/maybe` decisions apply here too:

- keep semantic shaping in backend IR and type lowering where possible
- keep the emitter simple
- avoid inventing runtime abstractions where the host language already has a good fit

Current Ard trait definitions appear to be plain method-set declarations rather than richer `mut`-receiver trait contracts, so the practical problems are more about coercion and representation interaction than about the trait declaration model itself.

## Decision

Traits will lower directly to Go interfaces.

This means:

- Ard traits become ordinary Go interfaces
- Ard impls satisfy those interfaces structurally through emitted methods
- trait-typed parameters, returns, locals, and collections lower to interface-typed Go values
- richer trait coercion points normalize earlier in backend IR so emission stays straightforward

This makes direct trait-to-interface lowering the accepted default model for ordinary trait usage.

## Rationale

### Why direct Go interfaces are the right host model

For current Ard trait usage, Go interfaces are the clearest and most natural target representation.

They support:

- ordinary method dispatch
- storing heterogeneous implementers behind a shared method-set type
- trait-typed parameters and returns
- trait-typed collections
- readable generated code

This aligns well with the long-term goal that ordinary Ard code should compile to ordinary Go without relying on a large target-specific helper runtime.

### Why coercion should be handled in backend IR

The main complexity is not interface dispatch itself, but coercion/value-shaping in more complex expression contexts.

Examples include:

- choosing between two concrete values in a branch and returning a trait-typed result
- assigning a concrete value into a trait-typed temp
- flowing values through other generated representations such as unions

Those are best treated as backend-IR or earlier lowering problems, not as emitter-local tricks.

### Why wrapper-heavy approaches are rejected as the default

A wrapper/coercion layer can sometimes make edge cases explicit, but as a general strategy it:

- adds indirection
- makes generated Go harder to read
- works against the goal of mostly mechanical emission
- risks recreating a runtime-style abstraction layer for ordinary language features

### Why mixed handling is not the primary design

A mixed approach can be useful as a migration or edge-case technique, but it should not become the main model.

The default should remain:

- direct interface lowering
- IR normalization for complex coercion points

## Consequences

### Positive

- traits map to a familiar and host-native Go concept
- generated Go remains readable
- trait dispatch does not require helper runtime support
- trait-typed values compose naturally with ordinary Go interface usage
- the emitter can stay straightforward if coercion shaping happens earlier

### Negative

- complex coercion points still require careful backend IR normalization
- interactions with unions and other generated representations still need deliberate design
- FFI-exposed Go types may sometimes need explicit adapters when their API shape does not line up exactly with Ard trait expectations

## Implementation guidance

### Preferred lowering shape

Trait definitions should emit as ordinary Go interfaces.

Ard:

```ard
trait ToString {
  fn to_str() Str
}
```

Go shape:

```go
type ToString interface {
	ToStr() string
}
```

Trait implementations should lower to ordinary methods that structurally satisfy the interface.

Ard:

```ard
struct User {
  name: Str,
}

impl ToString for User {
  fn to_str() Str {
    self.name
  }
}
```

Go shape:

```go
type User struct {
	Name string
}

func (self User) ToStr() string {
	return self.Name
}
```

Trait-typed parameters should lower directly to interface-typed Go parameters.

Ard:

```ard
fn render(value: ToString) Str {
  value.to_str()
}
```

Go shape:

```go
func Render(value ToString) string {
	return value.ToStr()
}
```

Trait-typed collections should lower to interface-typed collections.

Ard:

```ard
fn render_all(values: [ToString]) [Str] {
  mut out: [Str] = []
  for value in values {
    out.push(value.to_str())
  }
  out
}
```

Go shape:

```go
func RenderAll(values []ToString) []string {
	out := []string{}
	for _, value := range values {
		out = append(out, value.ToStr())
	}
	return out
}
```

### Preferred handling of coercion points

When an Ard expression produces a trait-typed value from concrete values, normalize that coercion earlier in backend IR.

Ard:

```ard
fn choose(flag: Bool, left: User, right: User) ToString {
  if flag {
    left
  } else {
    right
  }
}
```

Possible normalized backend IR idea:

```text
bind __result_trait ToString
if flag {
  __result_trait = left
} else {
  __result_trait = right
}
return __result_trait
```

Possible emitted Go shape:

```go
func Choose(flag bool, left User, right User) ToString {
	var result ToString
	if flag {
		result = left
	} else {
		result = right
	}
	return result
}
```

## Rejected alternatives

### Wrapper/coercion layer as the default model

Rejected because it adds indirection and makes generated Go less readable, while solving problems that are usually better handled in backend IR.

### Mixed static/dynamic handling as the primary design

Rejected because it risks inconsistent generated shapes and makes the backend harder to reason about.

It may still be acceptable as a temporary migration or narrow edge-case technique, but it is not the target architecture.

## Follow-up questions

- Which parts of trait coercion should remain visible in emitted Go, and which should normalize earlier in backend IR?
- How should unions and other generated representations interact with trait-typed values?
- What should FFI-exposed trait-compatible Go types look like from Ard?
- Do any trait cases require special value-shaping steps that should become explicit backend IR instructions?
