---
title: Cryptography with ard/crypto
description: Hash values, hash and verify passwords with bcrypt, and generate UUID v4 values.
---

The `ard/crypto` module provides hashing utilities, bcrypt password helpers, and UUID generation.

The crypto module provides:
- **Digest hashes** with `md5`, `sha256`, and `sha512`
- **Password hashing** with `hash` (bcrypt, configurable cost)
- **Password verification** with `verify`
- **ID generation** with UUID v4 via `uuid`

```ard
use ard/crypto
use ard/io

fn main() {
  let digest = crypto::sha256("hello")
  io::print(digest)
}
```

## API

### `fn md5(input: Str) Str`

Return the MD5 digest of `input` as a lowercase hex string.

### `fn sha256(input: Str) Str`

Return the SHA-256 digest of `input` as a lowercase hex string.

### `fn sha512(input: Str) Str`

Return the SHA-512 digest of `input` as a lowercase hex string.

### `fn hash(password: Str, cost: Int?) Str!Str`

Hash `password` using bcrypt.

- When `cost` is omitted (or `none`), the runtime default bcrypt cost is used.
- When `cost` is provided, it must be within bcrypt's valid range.
- Returns `ok(hashed_password)` on success or `err(message)` on failure.

### `fn verify(password: Str, hashed: Str) Bool!Str`

Verify a plaintext `password` against a bcrypt `hashed` string.

- Returns `ok(true)` if the password matches.
- Returns `ok(false)` when it does not match.
- Returns `err(message)` for malformed hashes or runtime errors.

### `fn uuid() Str`

Generate an RFC 4122 UUID v4 string (for example, `550e8400-e29b-41d4-a716-446655440000`).

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

### Hash with Explicit Cost

```ard
use ard/crypto

fn main() {
  let hashed = crypto::hash("my-secret-password", 12).expect("Could not hash password")
}
```

### Generate a UUID for New Records

```ard
use ard/crypto
use ard/io

struct User {
  id: Str,
  email: Str
}

fn main() {
  let user = User {
    id: crypto::uuid(),
    email: "person@example.com"
  }

  io::print("Created user with id: {user.id}")
}
```

### Compare Different Digest Algorithms

```ard
use ard/crypto
use ard/io

fn main() {
  let value = "hello"

  io::print("md5: {crypto::md5(value)}")
  io::print("sha256: {crypto::sha256(value)}")
  io::print("sha512: {crypto::sha512(value)}")
}
```
