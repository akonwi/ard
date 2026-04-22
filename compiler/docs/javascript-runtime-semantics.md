# JavaScript Runtime Semantics

This document describes the current runtime model and semantic constraints for Ard's JavaScript targets.

It focuses on observable backend behavior rather than historical design rationale. Architecture and lowering details live in the companion docs:

- `compiler/docs/javascript-emission-architecture.md`
- `compiler/docs/javascript-try-lowering.md`

## Targets

Ard currently exposes two JavaScript targets:

- `js-server`
- `js-browser`

Both targets emit modern ESM.

`js-server` is the broader target. It supports server-oriented stdlib modules such as `ard/io`, `ard/fs`, `ard/env`, and `ard/argv`.

`js-browser` is intentionally narrower. It supports browser-safe and JS-native modules such as `ard/js/promise` and `ard/js/fetch`, but not server-only stdlib modules.

## Target selection and checker validation

Checker-side target validation is part of the current JavaScript target model.

This validation happens during checking and import resolution, not during JavaScript code generation. That means target incompatibilities fail early and transitively through user-module imports.

### Effective target selection

For checking that happens as part of `ard run` or `ard build`, the effective target is resolved in this order:

1. CLI `--target`
2. `ard.toml` `target`
3. the compiler default target

`ard check` does not expose a separate `--target` flag.

### What the checker enforces today

The checker currently enforces JavaScript-target compatibility for:

- restricted stdlib imports
- ambiguous union matches that try to discriminate `Int` from `Float`

These rules apply transitively. If a user module imports a JS-incompatible stdlib module, then any root module that imports that user module also fails when checked for that target.

### Diagnostic shape

Restricted-import diagnostics use the form:

```text
Cannot import ard/sql when targeting js-browser; allowed targets: bytecode, go
```

Numeric union diagnostics use the form:

```text
Cannot discriminate Int from Float in union matches when targeting js-server; JavaScript represents both as number
```

## Output model

The JavaScript backend emits real multi-file ESM output:

- each Ard module becomes its own `.mjs` file
- Ard module imports become ESM imports
- shared runtime helpers live in `ard.prelude.mjs`
- extern implementations are loaded from JS companion modules such as:
  - `ffi.stdlib.js-server.mjs`
  - `ffi.stdlib.js-browser.mjs`
  - `ffi.project.<target>.mjs`

The generated output is intended to be readable and close to normal hand-written JavaScript.

## Primitive values

Ard primitive values lower as follows:

| Ard | JavaScript |
|---|---|
| `Int` | `number` |
| `Float` | `number` |
| `Bool` | `boolean` |
| `Str` | `string` |
| `Void` | `undefined` |
| `Dynamic` | raw JS value |

Important consequence:

- JavaScript does not distinguish Ard `Int` from `Float` at runtime
- both are represented as JS `number`
- JS targets therefore reject union matches that attempt to discriminate `Int` from `Float`

## Collections

### Lists

Ard lists lower to native JavaScript arrays.

### Maps

Ard maps lower to native JavaScript `Map` values.

## Structs and methods

Ard structs lower to ordinary JavaScript classes with field assignment in the constructor.

Ard `impl` methods lower to actual JavaScript class methods.

This keeps generated code idiomatic and makes method calls look like normal JS instance method calls.

## Enums

Ard enums lower to branded, frozen singleton objects grouped under an exported enum namespace.

Example shape:

```js
export const Color = Object.freeze({
  Red: makeEnum("Color", "Red", 0),
  Green: makeEnum("Color", "Green", 1),
});
```

The shared prelude provides the runtime support for enum behavior:

- `makeEnum(...)`
- `isArdEnum(...)`
- `isEnumOf(...)`
- `ardEnumValue(...)`

This representation preserves the Ard distinction that:

- enums are not plain integers as runtime values on JS
- enum values remain comparable to `Int` by their numeric discriminant through helper-backed lowering
- enums remain distinguishable inside erased union matches

### Enum methods

Enum methods are not emitted as instance methods on enum objects.

Instead, the backend lowers them to enum-specific helper functions and dispatches enum method calls through those helpers. This matches the branded singleton enum representation.

## Unions and matching

Ard unions remain erased type-system concepts on JavaScript targets.

They do not get their own runtime representation. Matching is lowered using runtime predicates over the emitted value:

- `typeof` for primitives
- wrapper checks for `Maybe` and `Result`
- enum branding checks for enums
- class checks for class-backed values
- JS collection checks where needed

### JS numeric limitation

Because `Int` and `Float` are both JS `number`, JS targets cannot safely distinguish them inside erased unions.

The checker enforces this by rejecting union matches that attempt to discriminate those two cases on JavaScript targets.

## `Maybe`

`Maybe` is implemented as a single prelude wrapper class.

State is determined by own-field presence, not by a tag field.

Current API shape in the JS prelude includes:

- `Maybe.some(...)`
- `Maybe.none()`
- `isSome()`
- `isNone()`
- `or(...)`
- `expect(...)`
- `map(...)`
- `andThen(...)`

This preserves the distinction between:

- `Maybe.some(undefined)`
- `Maybe.none()`

## `Result`

`Result` is implemented as a single prelude wrapper class.

Like `Maybe`, state is determined by own-field presence.

Current API shape in the JS prelude includes:

- `Result.ok(...)`
- `Result.err(...)`
- `isOk()`
- `isErr()`
- `or(...)`
- `expect(...)`
- `map(...)`
- `mapErr(...)`
- `andThen(...)`

This preserves the distinction between:

- `Result.ok(undefined)`
- `Result.err(undefined)`

## Equality

JavaScript equality lowering follows Ard's checker rules rather than adding broad structural equality.

The prelude helper `ardEq(...)` is used for the cases that need JS-specific handling:

- enum equality
- enum ↔ int comparisons
- `Maybe` equality

General structural equality for structs, lists, maps, and `Result` is not added on the JS backend.

## `panic` and runtime failures

Ard keeps its split between:

- recoverable value failures via `Result` and `Maybe`
- unrecoverable runtime failures via `panic`

`panic(...)` lowers to a thrown JavaScript `Error` created by the shared helper:

- `makeArdError(...)`

Generated panic paths and non-exhaustive match failures use that runtime error shape.

## Ard `try`

Ard `try` on JavaScript targets remains value-based only.

Semantic guarantees:

- it unwraps `Result` and `Maybe`
- it performs early returns from the current Ard function or lambda
- it does not become general JS exception handling
- it does not catch `panic(...)` or arbitrary thrown JS exceptions

Lowering details are intentionally documented separately in `compiler/docs/javascript-try-lowering.md`.

## Externs and FFI

JavaScript extern resolution is target-aware.

Extern bindings may specify:

- `js`
- `js-server`
- `js-browser`

Shared `js` bindings are used as the common fallback for both JS targets.

Extern implementations are loaded from companion JS modules rather than hardcoded backend special cases. This is how JS stdlib and project-level FFI are connected into generated output.

At the extern boundary:

- recoverable JS extern results can be adapted into Ard `Result` or `Maybe`
- invalid extern return shapes are reported as Ard runtime errors
- thrown JS exceptions are not handled by ordinary Ard `try`

## JS-native async support

The JavaScript backend does not support Ard's existing `ard/async` fiber model directly.

Instead, JS-native asynchronous workflows are currently expressed through:

- `ard/js/promise`
- `ard/js/fetch`

This means JavaScript targets do support async-style programming, but through JS-native promise/fetch APIs rather than the existing Ard fiber abstraction.

## Target-specific stdlib scope

The current checker uses module-level allowlists for stdlib portability rules.

Restricted modules currently behave as follows:

| Module | bytecode | go | js-browser | js-server |
|---|---|---|---|---|
| `ard/fs` | yes | yes | no | yes |
| `ard/sql` | yes | yes | no | no |
| `ard/env` | yes | yes | no | yes |
| `ard/io` | yes | yes | no | yes |
| `ard/argv` | yes | yes | no | yes |
| `ard/js/promise` | no | no | yes | yes |
| `ard/js/fetch` | no | no | yes | yes |

Stdlib modules not listed in the checker allowlist are currently treated as unrestricted unless they hit some other JS-specific semantic limitation.

### Available on `js-server`

Current JS-server-specific or JS-server-allowed stdlib modules include:

- `ard/io`
- `ard/fs`
- `ard/env`
- `ard/argv`
- `ard/js/promise`
- `ard/js/fetch`

JS-server also supports the JS extern-backed stdlib/runtime work used by:

- `ard/decode`
- `ard/json`
- `ard/dynamic`

### Available on `js-browser`

Current browser-safe JS modules include:

- `ard/js/promise`
- `ard/js/fetch`

`js-browser` intentionally does not support server-oriented modules such as `ard/io`, `ard/fs`, `ard/env`, or `ard/argv`.

## Current built-in scope limits

The current JavaScript backend intentionally does not provide built-in support for:

- `ard/sql` as a standard JS target module
- Ard's existing `ard/async` fiber model

These limits are about the current built-in stdlib/backend surface, not about what userland FFI can express.

## Related docs

- `compiler/docs/javascript-emission-architecture.md`
- `compiler/docs/javascript-try-lowering.md`
- `compiler/docs/ffi.md`
