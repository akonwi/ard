---
title: Base64 encoding with ard/base64
description: Encode and decode strings using standard base64 and base64url, with optional no-padding mode for JWT and PKCE.
---

The `ard/base64` module provides base64 encoding and decoding in two variants: standard (`+`/`/` alphabet) and URL-safe base64url (`-`/`_` alphabet). The URL-safe functions accept an optional `no_pad` flag to strip `=` padding when required.

Base64 is a reversible text encoding (not a cryptographic primitive). It is commonly used to embed binary data in text-only contexts like JSON payloads, URLs, JWTs, and HTTP headers.

The base64 module provides:
- **Standard base64** with `encode` and `decode`
- **URL-safe base64** with `encode_url` and `decode_url`, with optional no-padding mode

```ard
use ard/base64
use ard/io

fn main() {
  let encoded = base64::encode("hello")
  io::print(encoded) // "aGVsbG8="

  match base64::decode(encoded) {
    ok(original) => io::print(original),
    err(msg) => io::print("decode failed: {msg}"),
  }
}
```

## When to use which variant

| Variant     | Alphabet            | Padding          | Use for                                  |
|-------------|---------------------|------------------|------------------------------------------|
| `encode` / `decode`         | `A–Z a–z 0–9 + /` | `=`              | Email, generic binary-to-text            |
| `encode_url` / `decode_url` | `A–Z a–z 0–9 - _` | `=` (default) or none | URLs, JWTs, PKCE, filenames         |

The no-pad URL-safe form (`encode_url(input, true)`) is required by:
- **JWT** (JSON Web Tokens) — header and payload segments
- **PKCE** (OAuth 2.1) — `base64url(sha256(verifier))` must have no `=` padding

## API

### `fn encode(input: Str) Str`

Encode `input` using standard base64 with `=` padding. Uses the alphabet `A–Z`, `a–z`, `0–9`, `+`, `/`.

```ard
base64::encode("hello")  // "aGVsbG8="
base64::encode("f")      // "Zg=="
base64::encode("fo")     // "Zm8="
base64::encode("foo")    // "Zm9v"
base64::encode("")       // ""
```

### `fn decode(input: Str) Str!Str`

Decode a standard base64 string. Returns `ok(decoded)` or `err(message)` if the input is not valid base64.

```ard
let decoded = try base64::decode("aGVsbG8=") -> _ { "" }
// decoded == "hello"

base64::decode("not!valid!").is_err()  // true
```

### `fn encode_url(input: Str, no_pad: Bool?) Str`

Encode `input` using base64url (URL-safe alphabet: `A–Z`, `a–z`, `0–9`, `-`, `_`).

The optional `no_pad` flag controls trailing `=` padding:
- Omitted or `none`: padded (default)
- `true`: no padding — required by JWT and PKCE

```ard
// With padding (default)
base64::encode_url("subjects?")       // "c3ViamVjdHM_"
base64::encode_url("f")               // "Zg=="

// Without padding (JWT, PKCE)
base64::encode_url("f", true)         // "Zg"
base64::encode_url("fo", true)        // "Zm8"
```

### `fn decode_url(input: Str, no_pad: Bool?) Str!Str`

Decode a base64url-encoded string. The `no_pad` flag must match how the input was encoded:
- Omitted or `none`: expect `=` padding
- `true`: expect input without padding

```ard
// Padded input
let decoded = try base64::decode_url("aGVsbG8gd29ybGQ=") -> _ { "" }

// No-padding input (JWT, PKCE)
let decoded = try base64::decode_url("aGVsbG8gd29ybGQ", true) -> _ { "" }
```

## Examples

### Encode and Decode a String

```ard
use ard/base64
use ard/io

fn main() {
  let original = "The quick brown fox jumps over the lazy dog"
  let encoded = base64::encode(original)
  io::print(encoded)

  let decoded = try base64::decode(encoded) -> err {
    io::print("decode failed: {err}")
    return
  }
  io::print(decoded)
}
```

### PKCE Code Challenge

```ard
use ard/base64
use ard/crypto

fn pkce_challenge(verifier: Str) Str {
  let hashed = crypto::sha256(verifier)
  base64::encode_url(hashed, true)  // no padding for PKCE
}
```

### JWT Segment Encoding

```ard
use ard/base64
use ard/json

fn encode_segment(payload: Dynamic) Str!Str {
  let raw = try json::encode(payload)
  Result::ok(base64::encode_url(raw, true))  // no padding for JWT
}
```

### Handle Invalid Input

Decode functions return a `Result`, so bad input is surfaced as an error rather than a panic:

```ard
use ard/base64

fn safe_decode(input: Str) Str {
  base64::decode(input).or("<invalid>")
}
```
