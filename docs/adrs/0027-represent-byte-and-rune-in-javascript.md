# 0027: Represent Byte and Rune in the JavaScript Target

## Status

Proposed

## Context

ADR 0026 proposes first-class `Byte` and `Rune` primitives and intentionally leaves concrete JavaScript target representation out of that core language decision.

The JavaScript target has extra representation tradeoffs that should be considered separately:

- JavaScript numbers are floating-point values, so `Byte` and `Rune` need validation at construction and FFI boundaries.
- JavaScript strings are UTF-16 code-unit sequences, while Ard needs explicit UTF-8 byte views and Unicode-scalar views.
- Ard lists currently lower naturally to JavaScript arrays, but byte buffers often perform better and interoperate better as `Uint8Array`.
- The JavaScript target already emits ESM and runtime helpers, so helper-based validation/conversion can be shared across generated modules.

## Decision

When `Byte` and `Rune` support reaches the JavaScript target, start with a semantically simple representation:

- Lower `Byte` to JavaScript `number` with runtime validation at construction, decode, and FFI boundaries.
- Lower `Rune` to JavaScript `number` with runtime validation at construction, decode, and FFI boundaries.
- Convert `Rune` to text with `String.fromCodePoint`.
- Decode `Str.runes()` with `Array.from(str, ch => ch.codePointAt(0))` or an equivalent helper.
- Lower `Str.at(index)` through the same rune view and return `Maybe<Rune>`.
- Encode `Str.bytes()` with `TextEncoder` and decode `Str::from_utf8` with `TextDecoder` in fatal/strict mode.
- Implement `Str::split(input, delimiter)` in the `ard/string` module; for an empty delimiter, return one-rune strings with `Array.from(input)` or an equivalent helper.
- Lower `Str.size()` to UTF-8 byte length, not JavaScript `.length`.
- Initially lower `[Byte]` as `Array<number>` to fit the existing list model and minimize backend disruption.
- Special-case JSON encoding/decoding of `[Byte]` to follow Go's `encoding/json` convention: base64 JSON strings for byte buffers.

Defer `Uint8Array` lowering to a later optimization after the semantics are stable. A future change may specialize `[Byte]` storage or FFI boundaries to `Uint8Array`, but ordinary Ard list behavior must remain observable and portable.

## Consequences

- The JavaScript target can implement ADR 0026 semantics without first redesigning list lowering.
- `Str.size()` becomes portable with the Go target for non-ASCII text by counting UTF-8 bytes instead of UTF-16 code units.
- Byte and rune validation is explicit rather than relying on JavaScript numeric conventions.
- Initial `[Byte]` performance may be worse than native typed arrays, especially for large buffers.
- JSON behavior for `[Byte]` differs from ordinary list behavior by design: typed byte buffers serialize as base64 strings.
- FFI companions may need adapter code when host JavaScript APIs prefer `Uint8Array`.
- A later optimization ADR can introduce typed-array specialization without changing the language-level `Byte` / `Rune` semantics.

## Related

- `docs/adrs/0026-add-byte-and-rune-primitives.md`
- `docs/adrs/0015-use-esm-javascript-targets-with-explicit-runtime-semantics.md`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
