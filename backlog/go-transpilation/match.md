# ADR: Go Target `match` Lowering

## Status

Accepted

## Context

Ard's `match` construct does not map 1:1 onto a single Go language construct.

The Go target needs to preserve Ard semantics while still moving toward the long-term goal of:

- emitting ordinary readable Go
- avoiding `ardgo` runtime helpers for ordinary language features
- keeping the emitter simple and mostly mechanical

The current emitter can produce closure-heavy output for expression-shaped control flow. That is workable, but it is not the preferred long-term shape for the Go backend.

## Decision

Ard `match` will be handled primarily as an **early backend-IR desugaring problem**.

The backend will:

- desugar expression-shaped `match` into explicit control flow before Go emission
- normalize matches into branching plus temp assignment where needed
- preserve single-evaluation semantics explicitly in backend IR
- keep exhaustiveness failures explicit in generated code when required

The emitter should then choose ordinary Go branch forms based on the normalized shape:

- `if` / `else if` for bool and condition-heavy cases
- `switch` for enum/discrete-value cases
- type switches for union/type cases

This means the preferred architecture is:

- semantic lowering in backend IR
- mostly mechanical Go emission afterward

## Consequences

### Positive

- the emitter stays simpler
- generated Go becomes more readable
- `match` semantics are handled in one backend phase instead of being re-invented during emission
- the same general approach can be reused for other expression-shaped constructs
- future optimization passes become easier because control flow is already explicit in backend IR

### Negative

- backend IR lowering becomes more sophisticated
- some backend IR structure may need to evolve to represent normalized branching cleanly
- migration may require temporary coexistence with older closure-heavy lowering paths while coverage is expanded

## Implementation guidance

### Preferred emitted shapes

#### Bool / condition-heavy matches

Prefer `if` / `else if` chains.

Ard:

```ard
let sign = match n > 0 {
  true => "positive",
  false => "zero-or-negative",
}
```

Go shape:

```go
result := ""
if n > 0 {
	result = "positive"
} else {
	result = "zero-or-negative"
}
sign := result
```

#### Enum / discrete-value matches

Prefer `switch` statements.

Ard:

```ard
let weight = match region {
  Region::North => 3,
  Region::South => 2,
  Region::East => 4,
  Region::West => 5,
}
```

Go shape:

```go
subject := region
result := 0
switch subject.Tag {
case 0:
	result = 3
case 1:
	result = 2
case 2:
	result = 4
case 3:
	result = 5
default:
	panic("non-exhaustive enum match")
}
weight := result
```

#### Union / type matches

Prefer type switches.

Ard:

```ard
let label = match shape {
  Circle(c) => "circle",
  Rect(r) => "rect",
}
```

Go shape:

```go
subject := shape
result := ""
switch value := subject.(type) {
case Circle:
	c := value
	_ = c
	result = "circle"
case Rect:
	r := value
	_ = r
	result = "rect"
default:
	panic("non-exhaustive union match")
}
label := result
```

## Rollout order

All `match` categories should be covered by the same overall lowering model, but implementation can proceed incrementally.

Suggested order:

1. bool matches
2. enum matches
3. int/range matches
4. maybe/result matches
5. union/type matches
6. conditional matches

## Rejected alternatives

### Late emitter-only handling

Rejected because it keeps too much semantic complexity in the emitter and encourages closure-heavy output.

### Closure-heavy lowering as the default long-term shape

Rejected because it produces less readable Go and works against the goal of a simpler, more mechanical emitter.

### Single emitted form for all match categories

Rejected because different match categories map more naturally to different Go branch forms.

## Follow-up questions

- What exact normalized backend IR shape should all match forms lower into?
- How much existing backend IR can be reused versus extended?
- Which parity tests should gate each rollout stage?
- When should any temporary closure fallback paths be deleted completely?
