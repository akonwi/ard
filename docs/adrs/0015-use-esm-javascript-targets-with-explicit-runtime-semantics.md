# 0015: Use ESM JavaScript Targets with Explicit Runtime Semantics

## Status

Superseded by [ADR 0029](0029-remove-javascript-targets.md)

## Context

Ard supports JavaScript output for server and browser environments. JavaScript has different runtime semantics than Ard and Go: it represents both integers and floats as `number`, uses native promises for async workflows, has environment-specific host APIs, and relies on ESM for modern module loading.

The JavaScript backend therefore needs explicit target selection, target-aware stdlib validation, and a lowering architecture that keeps semantic decisions separate from output formatting.

## Decision

Support two JavaScript targets:

- `js-server`
- `js-browser`

Both targets emit modern multi-file ESM. Each Ard module becomes a `.mjs` file, Ard imports become ESM imports, shared helpers live in `ard.prelude.mjs`, and extern implementations load from target-aware JavaScript companion modules.

The effective target for `run` and `build` is resolved in this order:

1. CLI `--target`
2. `ard.toml` `target`
3. compiler default target

Checker-side validation is part of the JavaScript target model. Target incompatibilities should fail during checking/import resolution, including restricted stdlib imports and semantic cases JavaScript cannot represent safely, such as union matches that try to discriminate `Int` from `Float`.

JavaScript runtime representations should be explicit:

- `Int` and `Float` both lower to JS `number`
- `Bool` lowers to `boolean`
- `Str` lowers to `string`
- `Void` lowers to `undefined`
- `Dynamic` lowers to a raw JS value
- lists lower to arrays
- maps lower to `Map`
- structs lower to classes
- methods lower to JS methods where appropriate
- enums lower to branded frozen singleton objects
- unions remain erased and are matched with runtime predicates
- `Maybe` and `Result` use prelude wrapper classes that preserve `some(undefined)`/`none` and `ok(undefined)`/`err(undefined)` distinctions

Ard equality rules remain checker-defined. The JS backend should only add helper-backed equality where needed for JS-specific representation, such as enum equality, enum/int comparisons, and `Maybe` equality. It should not add broad structural equality for structs, lists, maps, or `Result`.

Ard `try` on JavaScript targets remains value-based. It unwraps `Result` and `Maybe`, performs early returns from the current Ard function or lambda, and does not catch `panic` or arbitrary thrown JavaScript exceptions.

The JS backend should lower `try` into ordinary JavaScript statement flow instead of sentinel throws and function-boundary catch handlers. Expressions containing `try` should normalize into prelude statements, explicit `Result`/`Maybe` guard returns, and a final simple value expression. Catch blocks remain early-return branches, not JavaScript exception handlers.

The long-term JavaScript emission architecture should separate semantic lowering from presentation:

```text
checked Ard tree
→ JavaScript IR
→ document tree
→ Prettier-like printer
→ .mjs output
```

The JavaScript IR should be small and represent only the emitted JavaScript subset. The document tree is presentation-only and should not introduce temporaries, normalize `try`, decide evaluation order, or rewrite matches. Generated JavaScript formatting should follow Prettier's style where practical for the emitted subset, without shelling out to Prettier or depending on a full external JavaScript formatter.

JavaScript externs should use target-aware companion `.mjs` modules. Recoverable extern results may adapt into Ard `Result` or `Maybe`; invalid extern return shapes are Ard runtime errors; thrown JS exceptions are not handled by ordinary Ard `try`.

JavaScript targets do not directly support Ard's existing `ard/async` fiber model. JS-native async workflows are expressed through JS-target stdlib modules such as `ard/js/promise` and `ard/js/fetch`.

## Consequences

- JavaScript target behavior is explicit instead of being inferred from Go/native semantics.
- Target-specific incompatibilities are caught early by the checker.
- `js-server` can support server-oriented stdlib modules while `js-browser` remains browser-safe.
- Generated JS stays readable and close to ordinary hand-written ESM.
- The backend avoids using JavaScript exceptions for Ard `try` propagation.
- The JS backend needs compatibility rules for JavaScript's numeric model and erased unions.
- Moving emission toward JS IR plus document rendering can happen incrementally while preserving the accepted runtime semantics.
- Ard fiber-style async requires a separate compatibility decision for JavaScript rather than silent approximation.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0005-use-result-maybe-and-try-for-error-handling.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
- `docs/adrs/0012-represent-optional-values-with-maybe.md`
