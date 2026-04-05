---
title: Cryptography with ard/crypto
description: Hash values, hash and verify passwords with bcrypt or scrypt, and generate UUID v4 values.
---

The `ard/crypto` module provides hashing utilities, password helpers (bcrypt and scrypt), and UUID generation.

The crypto module provides:
- **Digest hashes** with `md5`, `sha256`, and `sha512`
- **Password hashing** with `hash` (bcrypt, configurable cost)
- **Password verification** with `verify`
- **Scrypt hashing and verification** with `scrypt_hash` and `scrypt_verify`
- **ID generation** with UUID v4 via `uuid`

```ard
use ard/crypto
use ard/hex
use ard/io

fn main() {
  // sha256/sha512 return raw bytes — use hex or base64 to render them.
  let digest = crypto::sha256("hello")
  io::print(hex::encode(digest))
}
```

## API

### `fn md5(input: Str) Str`

Return the MD5 digest of `input` as a lowercase hex string.

### `fn sha256(input: Str) Str`

Return the SHA-256 digest of `input` as **raw bytes** (32 bytes, represented as a `Str`). Use [`hex::encode`](/stdlib/hex) or [`base64::encode`](/stdlib/base64) to render the result as text:

```ard
use ard/crypto
use ard/hex

hex::encode(crypto::sha256("hello"))
// "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
```

### `fn sha512(input: Str) Str`

Return the SHA-512 digest of `input` as **raw bytes** (64 bytes, represented as a `Str`). As with `sha256`, wrap the result in `hex::encode` or `base64::encode` to produce a text representation.

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

### `fn scrypt_hash(password: Str, salt_hex: Str?, n: Int?, r: Int?, p: Int?, dk_len: Int?) Str!Str`

Hash `password` with scrypt and return a hash string in the format `<salt_hex>:<derived_key_hex>`.

- `password` is normalized with Unicode NFKC before derivation.
- If `salt_hex` is omitted (or `none`), a random 16-byte salt is generated.
- Defaults are used for omitted parameters:
  - `n = 16384`
  - `r = 16`
  - `p = 1`
  - `dk_len = 64`
- Returns `ok(hash)` on success or `err(message)` on failure.

### `fn scrypt_verify(password: Str, hash: Str, n: Int?, r: Int?, p: Int?, dk_len: Int?) Bool!Str`

Verify `password` against a scrypt hash string in the format `<salt_hex>:<derived_key_hex>`.

- `password` is normalized with Unicode NFKC before derivation.
- Uses constant-time comparison for derived key matching.
- Uses the same default parameters as `scrypt_hash` when omitted.
- Returns `ok(true)` on match and `ok(false)` on non-match.
- Returns `err(message)` for malformed hash format or runtime failures.

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
use ard/hex
use ard/io

fn main() {
  let value = "hello"

  io::print("md5:    {crypto::md5(value)}")
  io::print("sha256: {hex::encode(crypto::sha256(value))}")
  io::print("sha512: {hex::encode(crypto::sha512(value))}")
}
```

### PKCE Code Challenge

For OAuth 2.1 / PKCE, hash the verifier with SHA-256 and base64url-encode the raw bytes (no padding). Since `sha256` now returns raw bytes directly, the challenge is a one-liner:

```ard
use ard/base64
use ard/crypto

fn pkce_challenge(verifier: Str) Str {
  base64::encode_url(crypto::sha256(verifier), true)
}
```

### Scrypt Password Hashing

```ard
use ard/crypto

fn main() {
  let hashed = crypto::scrypt_hash("my-secret-password").expect("Could not hash password")
  let valid = crypto::scrypt_verify("my-secret-password", hashed).expect("Could not verify password")

  if not valid {
    panic("Expected password verification to succeed")
  }
}
```
