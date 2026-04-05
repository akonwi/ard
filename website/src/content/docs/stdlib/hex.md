---
title: Hex encoding with ard/hex
description: Encode and decode raw bytes as hexadecimal strings.
---

The `ard/hex` module provides hexadecimal encoding and decoding for raw byte buffers. It's the standard way to render binary data (such as cryptographic digests) as a human-readable text string.

In Ard, byte buffers are represented as `Str`. Functions that return binary data — like `crypto::sha256` and `crypto::sha512` — produce this representation, and `hex::encode` turns it into a hex string.

```ard
use ard/crypto
use ard/hex
use ard/io

fn main() {
  let digest = crypto::sha256("hello")
  io::print(hex::encode(digest))
  // 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
}
```

## API

### `fn encode(bytes: Str) Str`

Encode `bytes` as a lowercase hexadecimal string. Each input byte becomes two hex characters.

```ard
hex::encode("")       // ""
hex::encode("abc")    // "616263"
hex::encode("hello")  // "68656c6c6f"
```

### `fn decode(input: Str) Str!Str`

Decode a hexadecimal string back into raw bytes. Returns `ok(bytes)` on success, or `err(message)` if `input` is not valid hex (odd length, non-hex characters, etc.).

```ard
let bytes = try hex::decode("616263") -> _ { "" }
// bytes == "abc"

hex::decode("xyz").is_err()    // true (non-hex characters)
hex::decode("abc").is_err()    // true (odd length)
```

## Examples

### Render a SHA-256 digest as hex

```ard
use ard/crypto
use ard/hex

fn fingerprint(input: Str) Str {
  hex::encode(crypto::sha256(input))
}
```

### PKCE S256 code challenge

For the OAuth 2.1 PKCE flow, hash the verifier with SHA-256 then base64url-encode the raw bytes (no padding):

```ard
use ard/base64
use ard/crypto

fn pkce_challenge(verifier: Str) Str {
  base64::encode_url(crypto::sha256(verifier), true)
}
```

If you need both the base64url challenge and the hex digest for debugging, `hex::encode` and `base64::encode_url` both accept the same raw bytes from `crypto::sha256`:

```ard
let digest = crypto::sha256(verifier)
let challenge = base64::encode_url(digest, true)
let debug_hex = hex::encode(digest)
```

### Roundtrip bytes through a text channel

```ard
use ard/hex

fn main() {
  let original = "some bytes"
  let encoded = hex::encode(original)
  let decoded = try hex::decode(encoded) -> _ {
    return
  }
  // decoded == original
}
```
