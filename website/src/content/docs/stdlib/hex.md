---
title: Hex encoding with ard/hex
description: Encode and decode byte buffers as hexadecimal strings.
---

The `ard/hex` module provides hexadecimal encoding and decoding for `[Byte]` buffers. It is the standard way to render binary data, such as cryptographic digests, as human-readable text.

```ard
use ard/crypto
use ard/hex
use ard/io

fn main() {
  let digest = crypto::sha256("hello".bytes())
  io::print(hex::encode(digest))
}
```

## API

### `fn encode(bytes: [Byte]) Str`

Encode bytes to lowercase hexadecimal text.

```ard
hex::encode("".bytes())      // ""
hex::encode("abc".bytes())   // "616263"
hex::encode("hello".bytes()) // "68656c6c6f"
```

### `fn decode(input: Str) [Byte]!Str`

Decode hexadecimal text back into bytes. Returns `err(message)` for odd-length or non-hex input.

```ard
let bytes = try hex::decode("68656c6c6f")
let text = Str::from_bytes(bytes).expect("utf8") // "hello"

hex::decode("abc").is_err() // true
hex::decode("zz").is_err()  // true
```

## Examples

```ard
use ard/base64
use ard/crypto
use ard/hex

fn fingerprint(input: Str) Str {
  hex::encode(crypto::sha256(input.bytes()))
}

fn pkce_challenge(verifier: Str) Str {
  base64::encode_url(crypto::sha256(verifier.bytes()), true)
}
```
