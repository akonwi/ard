---
title: Base64 encoding with ard/base64
description: Encode and decode byte buffers using standard base64 and base64url, with optional no-padding mode for JWT and PKCE.
---

The `ard/base64` module provides base64 encoding and decoding for `[Byte]` buffers in two variants: standard (`+`/`/` alphabet) and URL-safe base64url (`-`/`_` alphabet). Both variants accept an optional `no_pad` flag to strip `=` padding when required.

Base64 is a reversible text encoding (not a cryptographic primitive). Convert text explicitly with `.bytes()` before encoding and `Str::from_bytes(...)` after decoding when the bytes are UTF-8 text.

```ard
use ard/base64
use ard/io

fn main() {
  let encoded = base64::encode("hello".bytes())
  io::print(encoded) // "aGVsbG8="

  match base64::decode(encoded) {
    ok(bytes) => io::print(Str::from_bytes(bytes).expect("utf8")),
    err(msg) => io::print("decode failed: {msg}"),
  }
}
```

## API

### `fn encode(input: [Byte], no_pad: Bool?) Str`

Encode bytes using standard base64.

```ard
base64::encode("hello".bytes()) // "aGVsbG8="
base64::encode("f".bytes())     // "Zg=="
base64::encode("f".bytes(), true) // "Zg"
```

### `fn decode(input: Str, no_pad: Bool?) [Byte]!Str`

Decode a standard base64 string into bytes.

```ard
let bytes = try base64::decode("aGVsbG8=")
let text = Str::from_bytes(bytes).expect("utf8") // "hello"

base64::decode("not!valid!").is_err() // true
```

### `fn encode_url(input: [Byte], no_pad: Bool?) Str`

Encode bytes using base64url.

```ard
base64::encode_url("subjects?".bytes()) // "c3ViamVjdHM_"
base64::encode_url("f".bytes(), true)   // "Zg"
```

### `fn decode_url(input: Str, no_pad: Bool?) [Byte]!Str`

Decode base64url text into bytes. Pass `true` for no-padding JWT/PKCE inputs.

## Examples

### PKCE Code Challenge

```ard
use ard/base64
use ard/crypto

fn pkce_challenge(verifier: Str) Str {
  base64::encode_url(crypto::sha256(verifier.bytes()), true)
}
```

### JWT Segment Encoding

```ard
use ard/base64
use ard/json

fn encode_segment(payload: Dynamic) Str!Str {
  let raw = try json::encode(payload)
  Result::ok(base64::encode_url(raw.bytes(), true))
}
```
