---
title: Base64 encoding with ard/base64
description: Encode and decode strings using standard base64, base64url, and base64url without padding.
---

The `ard/base64` module provides base64 encoding and decoding in the three common variants: standard, URL-safe (base64url), and URL-safe without padding.

Base64 is a reversible text encoding (not a cryptographic primitive). It is commonly used to embed binary data in text-only contexts like JSON payloads, URLs, JWTs, and HTTP headers.

The base64 module provides:
- **Standard base64** with `encode` and `decode`
- **URL-safe base64** with `encode_url` and `decode_url`
- **URL-safe base64 without padding** with `encode_url_no_pad` and `decode_url_no_pad`

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

| Variant               | Alphabet                   | Padding | Use for                                  |
|-----------------------|----------------------------|---------|------------------------------------------|
| `encode` / `decode`   | `A–Z a–z 0–9 + /`          | `=`     | Email, generic binary-to-text            |
| `encode_url` / `decode_url` | `A–Z a–z 0–9 - _`    | `=`     | URL-safe contexts that accept padding    |
| `encode_url_no_pad` / `decode_url_no_pad` | `A–Z a–z 0–9 - _` | none    | JWTs, PKCE, URL query parameters         |

The no-pad URL-safe variant is required by:
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

### `fn encode_url(input: Str) Str`

Encode `input` using base64url with `=` padding. Uses the URL-safe alphabet `A–Z`, `a–z`, `0–9`, `-`, `_` — so the result is safe to use in URLs and filenames without further escaping.

```ard
base64::encode_url("subjects?")  // "c3ViamVjdHM_"
base64::encode_url("f")          // "Zg=="
```

### `fn decode_url(input: Str) Str!Str`

Decode a base64url-encoded string (with padding).

```ard
let decoded = try base64::decode_url("aGVsbG8gd29ybGQ=") -> _ { "" }
// decoded == "hello world"
```

### `fn encode_url_no_pad(input: Str) Str`

Encode `input` using base64url **without** trailing `=` padding. Use this for JWT segments and PKCE code challenges.

```ard
base64::encode_url_no_pad("f")    // "Zg"
base64::encode_url_no_pad("fo")   // "Zm8"
base64::encode_url_no_pad("foo")  // "Zm9v"
```

### `fn decode_url_no_pad(input: Str) Str!Str`

Decode a base64url string that has no padding.

```ard
let decoded = try base64::decode_url_no_pad("aGVsbG8gd29ybGQ") -> _ { "" }
// decoded == "hello world"
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
  base64::encode_url_no_pad(hashed)
}
```

### JWT Segment Encoding

```ard
use ard/base64
use ard/json

fn encode_segment(payload: Dynamic) Str!Str {
  let raw = try json::encode(payload)
  Result::ok(base64::encode_url_no_pad(raw))
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
