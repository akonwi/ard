---
title: Cryptography with ard/crypto
description: Hash byte buffers, hash and verify passwords with bcrypt or scrypt, and generate UUID v4 values.
---

The `ard/crypto` module provides hashing utilities, password helpers (bcrypt and scrypt), and UUID generation.

Digest APIs are bytes-in and bytes-out. Convert text with `.bytes()` and render digests with `ard/hex` or `ard/base64`.

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

### `fn md5(input: [Byte]) [Byte]`

Return the raw MD5 digest bytes. Use `hex::encode` for the traditional lowercase hex representation.

### `fn sha256(input: [Byte]) [Byte]`

Return the raw SHA-256 digest bytes (32 bytes).

```ard
hex::encode(crypto::sha256("hello".bytes()))
// "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
```

### `fn sha512(input: [Byte]) [Byte]`

Return the raw SHA-512 digest bytes (64 bytes).

### `fn hash(password: Str, cost: Int?) Str!Str`

Hash `password` using bcrypt.

### `fn verify(password: Str, hashed: Str) Bool!Str`

Verify a plaintext `password` against a bcrypt hash.

### `fn scrypt_hash(password: Str, salt_hex: Str?, n: Int?, r: Int?, p: Int?, dk_len: Int?) Str!Str`

Hash `password` with scrypt and return `<salt_hex>:<derived_key_hex>`.

### `fn scrypt_verify(password: Str, hash: Str, n: Int?, r: Int?, p: Int?, dk_len: Int?) Bool!Str`

Verify `password` against a scrypt hash string.

### `fn uuid() Str`

Generate an RFC 4122 UUID v4 string.

## Examples

### Hash and Verify a Password

```ard
use ard/crypto
use ard/io

fn main() {
  let hashed = crypto::hash("my-secret-password").expect("Could not hash password")
  let is_valid = crypto::verify("my-secret-password", hashed).expect("Could not verify password")

  if is_valid {
    io::print("Password is valid")
  } else {
    io::print("Invalid password")
  }
}
```

### Compare Different Digest Algorithms

```ard
use ard/crypto
use ard/hex
use ard/io

fn main() {
  let value = "hello".bytes()

  io::print("md5:    {hex::encode(crypto::md5(value))}")
  io::print("sha256: {hex::encode(crypto::sha256(value))}")
  io::print("sha512: {hex::encode(crypto::sha512(value))}")
}
```

### PKCE Code Challenge

```ard
use ard/base64
use ard/crypto

fn pkce_challenge(verifier: Str) Str {
  base64::encode_url(crypto::sha256(verifier.bytes()), true)
}
```
