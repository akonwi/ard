# ADR: Go Target Union Representation

## Status

Accepted

## Context

Ard unions are one of the clearest places where the language needs a value representation that Go does not provide directly.

Unlike traits, which map naturally to Go interfaces, unions are a closed Ard concept. They need a compiler-owned representation that works well with:

- early-desugared `match` lowering
- payload access after branch refinement
- trait interaction without overloading Go interface semantics
- collection/storage as ordinary values
- explicit FFI boundary handling

The broader backend direction established by the other Go target decisions still applies here:

- keep semantic shaping in backend IR
- keep the emitter straightforward and mostly mechanical
- avoid runtime-helper machinery for ordinary language features

## Decision

Unions will lower to **tagged generated structs**, not to Go interface-based open hierarchies.

Within tagged representations, the chosen payload layout is **B3: inline non-pointer payload storage**.

That means the preferred generated shape is:

- one generated Go struct per Ard union
- one explicit `Tag` field
- one inline payload field per variant
- inactive payload fields hold Go zero values
- `Tag` is the sole source of truth for which payload is active

The backend will:

- lower union `match` expressions early in backend IR into explicit branching
- make payload access explicit only within the branch corresponding to the active tag
- emit named tag constants rather than raw integer literals

## Rationale

### Why tagged structs are preferred over interface-based unions

A Go interface-based union model is superficially attractive because it looks more host-native, but it is not the best long-term fit for Ard unions.

The main issue is semantic ownership.

Traits already use Go interfaces as the host model for behavioral abstraction. If unions also use Go interfaces, then a closed Ard data concept starts depending too much on Go’s open interface semantics.

That creates awkward questions such as:

- how unions interact with trait/interface typing
- whether union values themselves should behave like interface hierarchies
- how much host-language interface behavior leaks into Ard semantics

Tagged structs avoid that by keeping unions as explicit compiler-owned data.

### Why B3 is preferred over B1

The chosen tagged layout is:

```go
const (
	shapeTagSquare = iota
	shapeTagCircle
)

type Shape struct {
	Tag    int
	Square Square
	Circle Circle
}
```

B3 is preferred over pointer-per-variant storage because it:

- keeps the representation explicit and compiler-owned
- avoids `any`-style dynamism
- avoids pointer-population invariants for valid values
- lets inactive variants simply hold Go zero values
- keeps `Tag` as the single source of truth

This makes the representation simpler to reason about and easier to scale.

### Why B2 (`Tag + any`) is rejected

A `Tag + any` layout would reintroduce host-side dynamism and weaken the compiler-owned representation. It would also work against the goal of straightforward lowering and predictable payload access.

### Why early IR desugaring matters here too

Union semantics are not just a representation problem; they also affect control flow.

A union `match` should not be solved ad hoc in the emitter.

Instead:

- backend IR should make branch refinement explicit
- payload access should become explicit after the branch/tag check
- the emitter should mainly render switches, assignments, and field access

That keeps union lowering aligned with the accepted `match` direction.

## Consequences

### Positive

- unions remain a closed, compiler-owned data representation
- the chosen model works well with early `match` desugaring
- trait/interface semantics stay separate from union representation semantics
- union values remain ordinary storable values in generated Go
- the emitter can stay straightforward

### Negative

- generated Go is less idiomatic than a host-interface-based approach
- inactive payload fields exist physically even when not active
- the backend must clearly maintain the tag invariants
- FFI code must construct and consume the tagged representation explicitly

## Representation invariants

### 1. Every union value has an explicit tag

Each emitted union struct has a `Tag` field whose value identifies the active variant.

Example:

```go
const (
	shapeTagSquare = iota
	shapeTagCircle
)

type Shape struct {
	Tag    int
	Square Square
	Circle Circle
}
```

Rules:

- tag values are compiler-assigned and stable within the generated package
- the backend emits named tag constants rather than raw integer literals
- matching and payload access always branch on `Tag`, not on field zero-ness

### 2. Payload storage must be consistent with the active tag

For a valid union value, the payload field corresponding to the active tag is the meaningful payload.

Example valid values:

```go
Shape{Tag: shapeTagSquare, Square: Square{Size: 10}, Circle: Circle{}}
Shape{Tag: shapeTagCircle, Square: Square{}, Circle: Circle{Radius: 5}}
```

Rules:

- generated code constructs only valid values
- emitted branch/payload access may assume the tag invariant holds after a tag check
- the tag, not incidental zero-ness of payload fields, is the source of truth for the active variant

### 3. The zero value is not a valid Ard union value by default

Unless a specific future design says otherwise, the zero value of the emitted Go struct is considered invalid/uninitialized.

Example zero value:

```go
Shape{}
```

Rules:

- generated Ard code does not rely on zero-value unions as meaningful values
- if a union value must be created, emitted code should do so through generated constructors or explicit literals that satisfy the invariants
- match lowering should still defend against impossible/default states with a panic when necessary

### 4. Constructors should enforce the invariant shape

The backend should prefer generating constructors or constructor-like literals that make valid union formation explicit.

Example constructor-like shapes:

```go
func NewShapeSquare(value Square) Shape {
	return Shape{Tag: shapeTagSquare, Square: value}
}

func NewShapeCircle(value Circle) Shape {
	return Shape{Tag: shapeTagCircle, Circle: value}
}
```

If constructors are not emitted as named funcs, emitted literals should still follow the same invariant shape.

### 5. Match lowering should make payload access explicit after tag checks

Payload access should only happen inside the branch that corresponds to the checked tag.

Example Ard:

```ard
struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle

fn fmt_area(shape: Shape) Str {
  match shape {
    Square => it.size.to_str(),
    Circle => it.radius.to_str()
  }
}
```

Possible normalized backend IR idea:

```text
bind __match_subject = shape
bind __match_result = ""
switch __match_subject.Tag {
  SquareTag:
    bind it = __match_subject.Square
    __match_result = it.size.to_str()
  CircleTag:
    bind it = __match_subject.Circle
    __match_result = it.radius.to_str()
  default:
    panic non-exhaustive
}
return __match_result
```

Possible emitted Go shape:

```go
func FmtArea(shape Shape) string {
	var result string
	switch shape.Tag {
	case shapeTagSquare:
		result = strconv.Itoa(shape.Square.Size)
	case shapeTagCircle:
		result = strconv.Itoa(shape.Circle.Radius)
	default:
		panic("non-exhaustive union match")
	}
	return result
}
```

Rules:

- backend IR makes the relationship between tag check and payload access explicit
- the emitter does not need extra defensive dynamic checks in ordinary happy-path code beyond the branch structure and default panic path

### 6. Trait interaction happens outside the union representation itself

Because traits already lower to Go interfaces, unions remain compiler-owned tagged data rather than trying to reuse Go interface semantics for the union representation itself.

Rules:

- a union value is not modeled as a Go interface-based open hierarchy
- if a union participates in trait-typed flows, any required coercion/value shaping happens in backend IR or through explicit generated adaptation steps

### 7. FFI treats unions as explicit generated data

At the Go boundary, unions are treated as explicit generated structs with known tags and payload fields.

Rules:

- FFI code constructs and consumes the generated union representation deliberately
- no implicit `any`-style host union encoding is the default for ordinary Go-target interop

## Implementation guidance

### Example: simple union representation

Ard:

```ard
struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle
```

Go shape:

```go
const (
	shapeTagSquare = iota
	shapeTagCircle
)

type Shape struct {
	Tag    int
	Square Square
	Circle Circle
}
```

### Example: name match

Ard:

```ard
fn get_name(shape: Shape) Str {
  match shape {
    Square => "Square",
    Circle => "Circle"
  }
}
```

Go shape:

```go
func GetName(shape Shape) string {
	var result string
	switch shape.Tag {
	case shapeTagSquare:
		result = "Square"
	case shapeTagCircle:
		result = "Circle"
	default:
		panic("non-exhaustive union match")
	}
	return result
}
```

## Rejected alternatives

### Interface-based unions as the default model

Rejected because Ard unions are a closed data concept and should not depend on Go interface semantics in the same way Ard traits do.

### `Tag + any` payloads

Rejected because they reintroduce host-side dynamism and weaken the compiler-owned representation.

### Pointer-per-variant payloads as the default shape

Rejected as the preferred default because they require pointer-population invariants for valid values, while B3 can use zero-valued inactive payloads and keep `Tag` as the sole source of truth.

## Follow-up questions

- Are there any union categories where a different layout would still be preferable to the default B3 shape?
- How should unions with many variants affect generated code size and layout choices?
- How should unions interact with FFI-exposed types in more complex real-world cases?
- Do any union coercions require explicit backend IR instructions beyond ordinary `match` lowering?
