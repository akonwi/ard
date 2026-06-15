# 0026: Add Byte and Rune Primitive Types

## Status

Proposed

## Context

Ard currently has one string-like primitive: `Str`. It is used for human text, protocol text, and in several standard-library APIs as a container for raw bytes. For example, `crypto::sha256` returns raw digest bytes as `Str`, and `hex` / `base64` accept raw bytes represented as `Str`.

That simplification is becoming limiting. Terminal and TUI code often needs to inspect or repair text at a lower level than whole strings. In side projects, mojibake can appear in text mostly managed by Ard. Cleaning it in Ard is difficult because `Str.at()` returns a one-character `Str`, `Str` has no byte-level view, and there is no scalar-value type that can be compared, classified, or converted without routing through FFI. In practice, mojibake repair has to be pushed into host-language FFI because Ard cannot express the byte/rune transformations directly.

The current model also blurs semantics:

- `Str` methods mix text and storage concerns. `Str.at()` is documented and implemented as Unicode-code-point indexing, while backend `size()` implementations have historically followed host string length (`len` in Go, `.length` in JS), which is not the same unit for non-ASCII text.
- Raw bytes encoded as `Str` can contain data that is not text, but Ard APIs still expose them as text values.
- JavaScript and Go have different native string storage models, so relying on host string length/indexing leaks target differences into Ard behavior.
- FFI code has no typed way to distinguish text from byte buffers.

Other languages separate these concerns:

- Go has `string` plus `byte` (`uint8`) and `rune` (`int32` code point). Iterating over a string decodes runes, while byte inspection is explicit.
- Rust has UTF-8 `String` / `str`, `u8` byte slices, and `char` Unicode scalar values.
- Python 3 separates Unicode `str` from binary `bytes`.
- Swift exposes multiple string views, including UTF-8 bytes, Unicode scalars, and extended grapheme clusters.
- JavaScript strings are UTF-16 code-unit sequences, and byte access requires explicit encoders/decoders such as `TextEncoder` / `TextDecoder`.

Ard needs a small, explicit model that keeps `Str` as text while allowing portable byte and Unicode-scalar inspection across targets.

## Decision

Introduce two first-class primitive types:

- `Byte`: an unsigned 8-bit value in the range `0..255`.
- `Rune`: a Unicode scalar value. A `Rune` represents one Unicode code point excluding surrogate code points. It is not an extended grapheme cluster and is not a terminal cell.

Keep `Str` as Ard's Unicode text type. `Str` values are sequences of Unicode scalar values at the language level, independent of backend string storage. Byte buffers should be represented as `[Byte]`, not `Str`.

### Core conversions

Add standard APIs for explicit conversion between text, runes, and bytes:

```ard
let bytes: [Byte] = "hé".bytes()      // UTF-8 bytes
let runes: [Rune] = "hé".runes()      // Unicode scalar values
let text = Str::from_bytes(bytes)       // Str!Str
let text2 = Str::from_runes(runes)     // Str!Str
```

`Str.bytes()` returns the UTF-8 encoding of the string. `Str.runes()` returns the Unicode scalar values in order. `Str::from_bytes` validates UTF-8 and returns `err` for invalid byte sequences. `Str::from_runes` validates scalar values and returns `err` if any rune is invalid.

Add primitive conversions for numeric interop:

```ard
Byte::from_int(value)  // Byte?
Rune::from_int(value)  // Rune?
byte.to_int()          // Int
rune.to_int()          // Int
rune.to_str()          // Str, one-rune text
```

Integer literals should not implicitly coerce to `Byte` or `Rune` in the initial implementation. Range-checked literal convenience can be added later if it proves ergonomic.

### Primitive method set

Keep the initial method set intentionally small.

`Byte` has:

```ard
Byte::from_int(value: Int) Byte?
byte.to_int() Int
byte.to_str() Str
byte.to_dyn() Dynamic
```

`Byte::from_int` returns `none` when `value` is outside `0..255`. `byte.to_str()` returns the decimal numeric representation, matching `Byte`'s role as binary data rather than text. To interpret bytes as text, use `Str::from_bytes([byte])` or `Str::from_bytes(bytes)`.

`Rune` has:

```ard
Rune::from_int(value: Int) Rune?
rune.to_int() Int
rune.to_str() Str
rune.to_dyn() Dynamic
```

`Rune::from_int` returns `none` when `value` is not a valid Unicode scalar value. `rune.to_str()` returns a one-rune `Str`; code that needs the numeric code point can use `rune.to_int().to_str()`.

Both types should implement `Str::ToString` through their `to_str()` methods so they work with string interpolation and `io::print`. Additional classification, case-mapping, ASCII, Unicode property, hex, and encoding helpers should live in standard-library modules rather than on the primitives.

### Prelude modules

Add standard-library modules for the primitive namespaces and auto-import them in the prelude:

```ard
use ard/byte as Byte
use ard/rune as Rune
```

These modules provide static constructors such as `Byte::from_int` and `Rune::from_int`, matching the existing `Int::from_str` style. User code should not need explicit imports for ordinary `Byte::...` or `Rune::...` access.

### Dynamic and JSON representation

`Byte` and `Rune` support is incomplete unless `Dynamic` and JSON understand the new primitives.

`dynamic::Primitive` should include `Byte` and `Rune`. `byte.to_dyn()` and `rune.to_dyn()` should represent both values as numeric dynamic values. JSON encoding should emit `Byte` and `Rune` as JSON numbers, and JSON parsing into `Byte` or `Rune` should require an integer in the valid range for that type.

Follow Go's `encoding/json` conventions for byte and rune collections:

- `[Byte]` encodes as a base64 JSON string and decodes from a base64 JSON string.
- `[Rune]` encodes as a JSON array of numbers and decodes from a JSON array of valid Unicode scalar integers.

`Str` remains the text representation. Code that wants a one-rune JSON string should encode `rune.to_str()`, not `rune` directly. Dynamic values should provide a way to preserve byte-buffer identity, such as `dynamic::from_bytes(bytes: [Byte]) Dynamic`, so dynamic JSON encoding can follow the same base64 rule for bytes.

### Comparison and ordering

`Byte` and `Rune` values are comparable within their own type:

```ard
let same_byte = b1 == b2
let earlier_byte = b1 < b2
let same_rune = r1 == r2
let earlier_rune = r1 < r2
```

Equality operators (`==`, `not ==`) and relative comparison operators (`<`, `<=`, `>`, `>=`) should apply to `Byte` with unsigned byte ordering and to `Rune` with Unicode scalar-value ordering.

These comparisons are not text collation, locale comparison, grapheme comparison, or display-width comparison. They only compare numeric byte values or numeric Unicode scalar values.

Mixed comparisons are not implicit in the initial implementation. `Byte` should not compare directly with `Rune` or `Int`; `Rune` should not compare directly with `Int`. Code that needs numeric comparison should use explicit conversion, such as `byte.to_int()` or `rune.to_int()`.

Arithmetic operators should not apply to `Byte` or `Rune` initially. Code that needs arithmetic should convert to `Int`, then convert back through `Byte::from_int` or `Rune::from_int` if needed.

### String indexing and sizes

Define string indexing in terms of `Rune`, not backend bytes or code units:

```ard
"hé".at(1)          // Rune?
"hé".runes().at(1) // same scalar value
```

`Str.at(index)` should return `Rune?`. This is an immediate breaking change from the current one-rune `Str?` return type; code that needs string fragments should call `.to_str()` on the returned rune.

`Str.size()` should continue to return byte length, specified as UTF-8 byte length. It is equivalent to `text.bytes().size()`:

```ard
text.size()         // UTF-8 byte count
text.bytes().size() // UTF-8 byte count
text.runes().size() // Unicode scalar count
```

This preserves existing byte-budget use cases, such as protocol limits and simple ASCII-oriented TUI truncation. It also makes the JavaScript target stop using UTF-16 code-unit length for `Str.size()`; JavaScript should compute UTF-8 byte length instead.

`Str.at(index)` and `Str.runes().at(index)` use rune positions, so `Str.size()` is not their bound for non-ASCII text. `[Byte].size()` and `[Rune].size()` remain ordinary list lengths.

### String iteration

Direct iteration over `Str` should use the same unit as `Str.runes()`: `Rune`.

```ard
for r in "foobar" {
  // r: Rune
}
```

The optional loop index remains `Int` and counts rune positions. Iteration is over Unicode scalar values, not UTF-8 bytes, UTF-16 code units, grapheme clusters, or terminal cells.

Byte iteration is explicit:

```ard
for byte in text.bytes() {
  // byte: Byte
}
```

Code that needs a one-rune string must call `rune.to_str()` explicitly.

This is an immediate breaking change from the current string loop cursor type.

### String splitting

Move string splitting out of the primitive method set and into the prelude `ard/string` module:

```ard
Str::split(input: Str, delimiter: Str) [Str]
Str::split("hé", "") // ["h", "é"]
```

`Str::split(input, "")` should return `[Str]` one-rune strings for compatibility with existing character-splitting code. The old `input.split(delimiter)` primitive method should be removed in the breaking implementation window; call sites should migrate to `Str::split(input, delimiter)`.

This is a compatibility text API, not the primitive scalar API. Code that wants scalar values should use `.runes()`, and code that wants UTF-8 bytes should use `.bytes()`.

### Standard library migration

Stop using `Str` as a raw byte buffer in new APIs. Prefer `[Byte]` for binary data:

```ard
hex::encode(bytes: [Byte]) Str
hex::decode(input: Str) [Byte]!Str
base64::encode(input: [Byte], no_pad: Bool?) Str
base64::decode(input: Str, no_pad: Bool?) [Byte]!Str
crypto::md5(input: [Byte]) [Byte]
crypto::sha256(input: [Byte]) [Byte]
crypto::sha512(input: [Byte]) [Byte]
```

Do not add text convenience wrappers for these byte-oriented APIs. Callers should convert text to bytes explicitly with `.bytes()` and decode bytes explicitly with `Str::from_bytes` when they want text.

For example, text-based PKCE code should become:

```ard
base64::encode_url(crypto::sha256(verifier.bytes()), true)
```

The short names become byte-primary immediately. Existing `Str`-based byte APIs should be changed in the same breaking implementation window rather than kept as compatibility wrappers. Digest APIs should be bytes in and bytes out, matching the shape of Go's `crypto/md5`, `crypto/sha256`, and `crypto/sha512` packages; callers can use `hex::encode(...)` or `base64::encode(...)` to render digests as text.

### FFI and backend representation

Backends should preserve the type distinction even when host representations overlap. The Go target has a direct host-language mapping:

- `Byte` lowers to `uint8` / `byte`.
- `Rune` lowers to `rune` / `int32` with validation at construction boundaries.
- `[Byte]` lowers naturally to `[]uint8` / `[]byte`.
- `[Rune]` lowers to `[]rune` / `[]int32`.

Concrete JavaScript target representation choices are deferred to ADR 0027. This decision defines language-level semantics and requires eventual JavaScript behavior to match those semantics, but does not choose between generic arrays, typed arrays, or helper-based lowering here.

FFI metadata generation should expose `Byte`, `Rune`, `[Byte]`, and `[Rune]` distinctly so host companions do not need to guess whether a `Str` is text or binary data.

### Scope limits

This decision does not introduce a `Char` type. "Character" is ambiguous: users may mean byte, Unicode scalar value, grapheme cluster, or terminal cell. `Rune` intentionally names the Unicode scalar layer.

This decision also does not solve grapheme segmentation, Unicode normalization, terminal display width, legacy encodings such as Windows-1252, URL percent encoding/decoding, or optimized JavaScript byte-buffer representation. Those can be layered on top once Ard can represent `[Byte]` and `[Rune]` directly.

URL percent decoding should be provided later as a dedicated helper such as `url::decode_component(input: Str) Str!Str`, instead of encouraging user code to decode percent escapes through low-level `hex::decode` text concatenation.

## Consequences

- Ard code can inspect and transform text at byte and Unicode-scalar granularity without FFI-only escape hatches.
- Mojibake repair and terminal-text cleanup can be expressed in Ard when paired with appropriate encoding helpers.
- Binary standard-library APIs become type-correct and no longer overload `Str` as a byte buffer.
- `Str.size()` becomes a portable UTF-8 byte count across Go and JavaScript targets. Explicit `Str.bytes()`, `Str.runes()`, and `Str.at()` rune semantics are also language-defined rather than host-defined.
- The compiler, checker, AIR, Go target, runtime helpers, formatter/docs, Dynamic/JSON support, and generated FFI metadata need updates for two new primitives. JavaScript representation details are tracked separately in ADR 0027.
- Direct `Str` iteration and `Str.at()` changing from one-rune `Str` values to `Rune` values will break existing code that concatenates or compares those values as strings. Migration should be explicit via `rune.to_str()`.
- Removing the primitive `input.split(delimiter)` method requires migration to the prelude function `Str::split(input, delimiter)`.
- `Str.size()` keeps byte-length semantics, though JavaScript lowering must change from UTF-16 code-unit length to UTF-8 byte length.
- `[Byte]` as a normal list is simple and type-safe; target-specific byte-buffer optimizations can be added later without changing the language semantics.
- `Rune` APIs expose code points, not user-perceived graphemes. Text UI libraries will still need higher-level helpers for grapheme clusters and display widths.

## Related

- `docs/adrs/0027-represent-byte-and-rune-in-javascript.md`
- `compiler/std_lib/crypto.ard`
- `compiler/std_lib/hex.ard`
- `compiler/std_lib/base64.ard`
- `compiler/std_lib/dynamic.ard`
- `compiler/std_lib/json.ard`
- `website/src/content/docs/stdlib/string.md`
- GitHub issue #224: Add URL percent-decoding helper
- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0008-use-target-aware-extern-companions-for-ffi.md`
