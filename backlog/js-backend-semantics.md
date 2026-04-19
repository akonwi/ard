# JS Backend Semantics

This document captures the current design direction for transpiling Ard to runnable JavaScript.

The goal of this document is not to specify every emission detail yet. It exists to lock in the runtime model, representation choices, and semantic constraints that the JS backend should follow before implementation begins.

## Summary

Ard should target JavaScript with a runtime model that is:

- idiomatic JavaScript
- readable in generated output
- aligned with Ard's current checker and bytecode semantics
- inspired by Gleam's JS backend where it fits
- willing to diverge from Gleam where Ard semantics explicitly differ

The main Ard-specific difference from Gleam is that Ard has explicit mutability plus copy semantics. That means the JS backend should not assume an immutability-first runtime model.

## Guiding Principle

Follow Gleam's lead for JavaScript code generation and runtime structure unless there is an explicit conflict with Ard semantics.

Known Ard-specific conflicts include:

- explicit mutability
- checker-driven copy semantics
- enums as discrete tagged ints rather than class-backed variants
- unions as erased type-system concepts

## Output Assumptions

The default JS backend should emit modern ESM.

That implies:

- Ard modules compile to JS modules
- imports become ESM imports
- output should run directly in modern Node
- browser support can rely on bundlers or direct ESM loading later

## Runtime Strategy

The JS backend should use a small shared runtime/prelude.

This runtime is responsible for:

- `Result`
- `Maybe`
- deep-copy entrypoints
- equality helpers
- panic/assert helpers
- any other helper behavior where Ard semantics differ from raw JS

The backend should prefer normal JS constructs where possible, and only fall back to runtime helpers where semantics require it.

## Current Decisions

## 1. Primitive Types

| Ard | JS |
|-----|----|
| `Int` | `number` |
| `Float` | `number` |
| `Bool` | `boolean` |
| `Str` | `string` |
| `Void` | `undefined` |
| `Dynamic` | raw JS value |

Notes:

- Ard `Void` may legitimately map to JS `undefined`.
- Ard does not generally model accidental `undefined`; explicit absence should still be represented through `Maybe.none()`.

## 2. Collections

### Lists

Ard lists should compile to native JS arrays.

Reasoning:

- more idiomatic JS
- simpler generated code
- easier interop
- checker-enforced mutability already gives semantic control

### Maps

Ard maps should compile to native JS `Map`.

Reasoning:

- avoids plain-object key/prototype issues
- closer to actual map semantics
- keeps behavior explicit

## 3. Structs

Ard structs should compile to plain idiomatic JS classes with normal constructor field assignment.

Example:

```js
class Person extends ArdValue {
  constructor(name, age) {
    super();
    this.name = name;
    this.age = age;
  }
}
```

No private slots or positional indexing should be used for ordinary structs.

## 4. Methods

Ard `impl` methods should compile to actual JS class methods.

Example:

```js
class Person extends ArdValue {
  constructor(name, age) {
    super();
    this.name = name;
    this.age = age;
  }

  birthday() {
    this.age += 1;
  }
}
```

This is the most idiomatic JS shape and matches Ard method syntax naturally.

## 5. Enums

Ard enums are discrete tagged ints, not class-backed variants.

In JavaScript they should compile to **frozen branded singleton objects** exposed through an object namespace, while still carrying their numeric discriminant.

Example:

```js
const __ard_enum = Symbol.for("ard.enum");

function makeEnum(enumName, variantName, value) {
  return Object.freeze({
    [__ard_enum]: true,
    enum: enumName,
    variant: variantName,
    value,
  });
}

export const Color = Object.freeze({
  Red: makeEnum("Color", "Red", 0),
  Green: makeEnum("Color", "Green", 1),
  Blue: makeEnum("Color", "Blue", 2),
});
```

This preserves an important Ard distinction:

- enums are their own runtime category for union matching
- enums remain comparable to `Int` by their numeric discriminant

That means JS lowering should use helpers when enum/int comparisons occur rather than assuming raw `===` on the emitted enum values.

## 6. Unions

Unions are erased at runtime.

They should not get their own JS runtime representation.

Union matching should lower to runtime tests over the underlying value categories, such as:

- `typeof` for primitives
- `instanceof` for class-backed values
- `instanceof Result` / `instanceof Maybe` for wrapper types
- branded enum checks like `isEnumOf(value, "Color")`
- `Array.isArray(...)` for lists if needed
- `value instanceof Map` for maps if needed

## 7. Pattern Matching

Ard's current matching needs are narrower than Gleam's.

The JS backend does not currently need to support:

- struct-field pattern matching
- list destructuring patterns
- equality guards

So the relevant lowering problem is mostly runtime discrimination for unions and wrapper types.

Current direction:

- user-defined class-backed values: `instanceof ConcreteType`
- `Result` / `Maybe`: `instanceof BaseWrapper` plus predicate methods
- enums: branded object checks plus numeric discriminant access

## 8. `Result`

`Result` should be represented as a single prelude wrapper class.

It should not use `Ok` / `Err` subclasses.

Instead it should have:

- payload fields: `ok`, `error`
- predicate methods: `isOk()`, `isErr()`
- static constructors: `Result.ok(...)`, `Result.err(...)`

State should be determined by **own-field presence**, not field value.

Example:

```js
class Result {
  constructor(ok, error) {
    if (arguments.length === 1) {
      this.ok = ok;
    } else {
      this.error = error;
    }
  }

  isOk() {
    return Object.hasOwn(this, "ok");
  }

  isErr() {
    return Object.hasOwn(this, "error");
  }

  static ok(value) {
    return new Result(value);
  }

  static err(error) {
    return new Result(undefined, error);
  }
}
```

Using field presence means `Result.ok(undefined)` and `Result.err(undefined)` remain distinguishable.

## 9. `Maybe`

`Maybe` should also be represented as a single prelude wrapper class.

It should have:

- payload field: `value`
- predicate methods: `isSome()`, `isNone()`
- static constructors: `Maybe.some(...)`, `Maybe.none()`

State should also be determined by own-field presence.

Example:

```js
class Maybe {
  constructor(value) {
    if (arguments.length === 1) {
      this.value = value;
    }
  }

  isSome() {
    return Object.hasOwn(this, "value");
  }

  isNone() {
    return !Object.hasOwn(this, "value");
  }

  static some(value) {
    return new Maybe(value);
  }

  static none() {
    return new Maybe();
  }
}
```

Using field presence means `Maybe.some(undefined)` and `Maybe.none()` remain distinguishable.

## 10. Copy Semantics

The JS backend should follow the checker's current copy-insertion policy, not a broader ad hoc JS policy.

That means copy operations should be emitted where current checker semantics produce `CopyExpression`, notably:

- mutable variable initialization for:
  - `Struct`
  - `List`
  - `Map`
- explicit mutable-argument copying for `mut` parameters

The runtime VM already deep-copies more categories internally, but the JS backend should treat the checker as the source of truth for when copies occur.

### Copy Depth

Copies at those points should be **deep structural copies**.

### Shared Copy Entrypoint

The JS runtime should expose a shared `copyValue(...)` entrypoint.

That function should:

- return primitives unchanged
- deep-copy arrays
- deep-copy `Map`
- call a generated per-class copy method when available

Example direction:

```js
function copyValue(value) {
  if (value == null) return value;

  switch (typeof value) {
    case "string":
    case "number":
    case "boolean":
    case "bigint":
      return value;
  }

  if (Array.isArray(value)) {
    return value.map(copyValue);
  }

  if (value instanceof Map) {
    const out = new Map();
    for (const [k, v] of value) {
      out.set(copyValue(k), copyValue(v));
    }
    return out;
  }

  if (typeof value.__ard_copy === "function") {
    return value.__ard_copy();
  }

  return value;
}
```

### Per-Class Copy Method

Class-backed Ard values should implement a generated copy method such as `__ard_copy()`.

Example:

```js
class Person extends ArdValue {
  constructor(name, tags, scores) {
    super();
    this.name = name;
    this.tags = tags;
    this.scores = scores;
  }

  __ard_copy() {
    return new Person(
      copyValue(this.name),
      copyValue(this.tags),
      copyValue(this.scores),
    );
  }
}
```

This gives the backend:

- one emitter-facing copy API
- explicit generated class-copy logic
- native handling for collections/primitives

### Special Note on `Maybe`, `Result`, and `Dynamic`

`Maybe`, `Result`, and `Dynamic` are not new checker-trigger copy categories by themselves.

However, they may still be copied if deep-copy semantics recurse into them or if later language semantics explicitly require copying them.

## 11. Mutability

Ard mutation should map to in-place JS mutation once a value is owned at the correct copy boundary.

This mirrors the existing bytecode/runtime model:

- copy at specific semantic boundaries
- mutate in place afterward

That means the JS backend does **not** need to model Gleam-style immutable record update as the default execution strategy.

## 12. Equality

Equality in the JS backend should follow the current checker rules rather than introducing broad structural equality.

The checker currently allows equality for:

- `Int`
- `Float`
- `Str`
- `Bool`
- enums
- enum/int comparisons
- `Maybe`/`Maybe`

The checker currently rejects equality for general compound values such as:

- structs
- lists
- maps
- extern/opaque-like values
- `Result`

So the JS backend should keep equality lightweight.

Current direction:

- primitives: use `===`
- enums: use a small helper that compares their numeric discriminants
- enum/int comparisons: use that same helper rather than raw `===`
- `Maybe`: use a small wrapper-aware helper or equivalent inline logic
- do not add general struct/list/map structural equality for v1

## 13. Opaque Types and Future JS FFI

This document does not design JS FFI itself, but the runtime model should leave room for opaque foreign values.

Important constraint:

- JS interop in Ard should make heavy use of opaque types
- `copyValue(...)` should eventually have a clear policy for opaque foreign handles
- opaque values should not be accidentally deep-copied like ordinary Ard class-backed data unless that is explicitly supported

## Gleam Research Notes

Relevant observations from the Gleam compiler/runtime:

- Gleam's JS backend is strongly class-based for custom types
- Gleam uses `instanceof` heavily in generated JS
- Gleam uses a shared prelude/runtime for core types like `Result` and `List`
- Gleam uses helper-backed equality
- Gleam's JS FFI uses explicit external module/function bindings

Ard should borrow those general lessons while preserving Ard-specific semantics around mutability, copying, enums, and unions.

## 14. Panic and Error Boundaries

Ard currently has two distinct failure channels:

1. recoverable value-level failures
   - `Result`
   - `Maybe`
   - handled by `try`
2. unrecoverable runtime failures
   - `panic`
   - runtime crashes
   - not handled by `try`

The JS backend should preserve that split.

### Panic

`panic(...)` in JS should lower to throwing a structured JavaScript `Error` instance.

The runtime should provide a helper like:

```js
function makeArdError(kind, module, fn, line, message, extra = {}) {
  const error = new globalThis.Error(message);
  error.ard_error = kind;
  error.module = module;
  error.function = fn;
  error.line = line;
  for (const key in extra) error[key] = extra[key];
  return error;
}
```

And codegen for panic should look like:

```js
throw makeArdError("panic", moduleName, functionName, line, message)
```

This follows Gleam's general JS runtime error shape while keeping Ard semantics.

### `try`

Ard `try` in JS should remain value-based only.

That means:

- `try` unwraps `Result` and `Maybe`
- `try` performs early returns exactly as Ard does today
- `try` should **not** become general JS exception handling
- Ard `try` should **not** catch `panic` or arbitrary thrown JS exceptions

### FFI Boundary Rule

The current bytecode/runtime already has an important rule for foreign panics:

- if the foreign function's Ard return type is `Result`, panics are converted into `Err(...)`
- otherwise the panic is re-thrown

The JS backend should preserve that idea when JS FFI is introduced:

- JS exceptions at extern boundaries may be converted into `Result.err(...)` when the signature says the call is recoverable
- otherwise they should be re-thrown as Ard runtime errors (or re-thrown with Ard context)

This should be handled at the extern boundary, not by ordinary Ard `try`.

## 15. Async Scope for v1

The initial JS backend should **not** support Ard's current `ard/async` model.

Reasoning:

- Ard async currently behaves like concurrent fibers with blocking `join()` / `get()` semantics.
- That maps well to goroutines, but not cleanly to JavaScript's event-loop and `Promise` model.
- Translating `get()` into implicit `await` would infect the call graph and change language semantics too much.
- Worker-based emulation is too heavy and too runtime-specific for a first JS backend.

So the current decision is:

- `ard/async` is unsupported for the first runnable JS backend
- the first JS backend should remain synchronous
- future JS async support should come from a separate design, likely centered around JavaScript promises rather than the existing fiber abstraction

## Remaining Open Questions

These still need explicit design decisions before implementation:

1. Can generics be fully erased, or are there cases that require specialization?
2. Do we want a dedicated lowering IR before final JS emission?
3. What recursion limitations are acceptable for the first version?
4. What should a future JS-native promise construct look like?

## Suggested Next Step

The next design pass should focus on the remaining semantic hotspots, especially:

1. whether a lowering IR is needed before final emission
2. generics erasure vs specialization
3. future JS promise design as a separate follow-up

A separate document should cover JS FFI and ecosystem interop once the runtime/value model is considered stable.
