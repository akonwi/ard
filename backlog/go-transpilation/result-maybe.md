# ADR: Go Target `Result` and `Maybe` Representation

## Status

Accepted

## Context

Ard's `Result` and `Maybe` constructs sit at the boundary between:

- language semantics
- control-flow lowering (`try`, matching, method calls)
- stdlib design
- FFI interop with Go

They do not have direct 1:1 equivalents in Go.

The Go target needs to preserve Ard semantics while still moving toward the long-term goal of:

- emitting ordinary readable Go
- avoiding `ardgo` runtime helpers for ordinary language features
- keeping the emitter straightforward and mostly mechanical

The main lesson from the `match` discussion applies here as well: richer Ard semantics should be normalized in backend IR as early as practical, so the emitter mainly renders explicit control flow, direct method calls, ordinary structs, and straightforward assignments.

## Decision

The Go target will use:

- **generated `Maybe[T]` and `Result[T, E]` generic types** as the preferred internal representation strategy for ordinary Ard-generated code
- **early backend-IR lowering** for richer control-flow semantics around those types
- **FFI-specific mappings** at the Go boundary where host APIs naturally use Go conventions such as `(T, error)`

This means:

1. **Strategy A is the default internal model**
   - Ard-generated Go code should represent `Maybe` and `Result` as stable first-class generic value types.

2. **Strategy B is not the primary model**
   - Contextual native Go lowering like `(T, bool)` or `(T, error)` is only a possible optimization mechanism for simple, non-compositional cases, if it is worth doing at all.

3. **Strategy C is the preferred FFI boundary model**
   - The host boundary may translate between Go-native conventions and Ard's internal `Maybe` / `Result` representation.
   - More complicated FFI functions that need nested `Result` / `Maybe` values should use the stable generated generic structs directly.

4. **`try` and related control flow are lowering problems**
   - `try` on `Result` / `Maybe` should desugar early in backend IR into explicit branching.
   - `match` on `Result` / `Maybe` should follow the broader early-desugaring `match` model.

## Rationale

### Why Strategy A is preferred internally

The key reason is composability.

Ard `Result` and `Maybe` can be:

- nested
- stored in structs
- stored in lists/maps
- returned from functions
- matched on as ordinary values

Go multi-return conventions like `(T, bool)` and `(T, error)` are not first-class value representations. They can be returned from functions, but they cannot be nested or treated uniformly as ordinary values.

That makes them a poor general representation strategy for Ard semantics.

By contrast, generated generic structs:

- compose naturally
- preserve Ard semantics clearly
- work cleanly with `try`, `match`, and generic code
- keep the internal representation predictable

### Why Strategy B is demoted

Contextual native lowering can look attractive for simple leaf cases, for example:

- `Maybe[T]` as `(T, bool)`
- `Result[T, E]` as `(T, error)`

But as a primary representation strategy it is weak because it:

- is not composable
- introduces inconsistent representations across contexts
- complicates matching, methods, and generic code
- works against the goal of keeping emission straightforward unless the rules become very constrained

For those reasons, Strategy B is treated only as a possible optimization mechanism, not as the default model.

### Why Strategy C remains important

At the FFI boundary, Go-native conventions are useful and expected.

Examples:

- Go `(T, error)` results can be wrapped into Ard `Result[T, Str]` or other Ard result shapes
- simple host-side optional patterns can be wrapped into Ard `Maybe[T]`
- more complicated nested result/optional returns can use the stable generic structs directly

This keeps:

- Ard's internal model consistent
- host interop explicit
- representation complexity localized to the boundary

## Consequences

### Positive

- Ard semantics remain composable in generated Go
- the emitter can stay straightforward because rich control flow lowers in backend IR
- `try` and `match` integrate cleanly with a stable representation model
- nested `Result` / `Maybe` values remain expressible without special cases
- FFI can still feel Go-native where appropriate

### Negative

- generated support types are still required
- the Go API shape for internal Ard-generated code is less idiomatic than plain `(T, error)`
- the FFI layer needs deliberate translation rules
- future optimization work must be careful not to fracture the stable internal representation model

## Implementation guidance

### Preferred internal representation

Use stable generic structs for ordinary Ard-generated code.

Example Ard (`Maybe` + `try`):

```ard
fn half(n: Int) Int? {
  if n == 0 {
    None
  } else {
    Some(n / 2)
  }
}

fn next(n: Int) Int? {
  let h = try half(n)
  Some(h + 1)
}
```

Possible normalized backend IR shape:

```text
call half(n) -> __try_subject
if __try_subject.IsNone() {
  return None[Int]()
}
bind h = __try_subject.Value
return Some(h + 1)
```

Possible emitted Go shape:

```go
func Half(n int) Maybe[int] {
	if n == 0 {
		return None[int]()
	}
	return Some(n / 2)
}

func Next(n int) Maybe[int] {
	trySubject := Half(n)
	if trySubject.IsNone() {
		return None[int]()
	}
	h := trySubject.Unwrap()
	return Some(h + 1)
}
```

Example Ard (`Result` + `try`):

```ard
fn parse_port(text: Str) Int!Str {
  let value = try Int::from_str(text) -> _ { Result::err("invalid") }
  Result::ok(value)
}
```

Possible emitted Go shape:

```go
func ParsePort(text string) Result[int, string] {
	trySubject := IntFromStr(text)
	if trySubject.IsNone() {
		return Err[int, string]("invalid")
	}
	value := trySubject.Unwrap()
	return Ok[int, string](value)
}
```

### Optimization-only native lowering

If ever used, contextual native lowering should be treated as an optimization mechanism only.

Example leaf-level shape:

```go
func Head(values []int) (int, bool) {
	if len(values) == 0 {
		return 0, false
	}
	return values[0], true
}
```

This is acceptable only where the representation does not need to compose as an ordinary Ard value.

### Preferred FFI boundary mapping

Use wrappers/adapters at the boundary.

Example Go-native host API wrapped into Ard `Result`:

```go
func ReadConfig(path string) Result[Config, string] {
	config, err := hostpkg.ReadConfig(path)
	if err != nil {
		return Err[Config, string](err.Error())
	}
	return Ok[Config, string](config)
}
```

Example direct Go-target FFI implementation using stable generic structs:

```go
func OpenFile(path string) Result[string, string] {
	data, err := os.ReadFile(path)
	if err != nil {
		return Err[string, string](err.Error())
	}
	return Ok[string, string](string(data))
}
```

For more complicated FFI cases involving nested option/result shapes, use the stable generated generic structs directly instead of trying to encode them through Go multi-return conventions.

## Rejected alternatives

### Strategy B as the default internal model

Rejected because Go multi-return conventions are not first-class nestable values and therefore do not compose as a general representation strategy for Ard semantics.

### Pushing `try`/success-failure semantics into emitter-local tricks

Rejected because it works against the backend direction established by `match`: rich semantics should lower early in backend IR so emission stays straightforward.

## Follow-up questions

- Should `Result` and `Maybe` be emitted into generated user/module code, generated stdlib code, or shared generated support code?
- If shared generated support code exists, when is it still acceptable versus becoming a forbidden helper runtime?
- Which operations should remain visible as methods on generated types, and which should desugar earlier in backend IR?
- What exact normalized IR shape should `try` on `Result` / `Maybe` lower into?
- How should `match` on `Result` / `Maybe` interact with the broader match-lowering strategy?
- What should the Go-side FFI mapping story be for simple versus nested `Result` / `Maybe` returns?
