---
title: Encoding with ard/encode
description: Encode primitive values to JSON strings with the ard/encode module.
---

The `ard/encode` module provides a focused JSON encoder for primitive values. It exposes a small `Encodable` trait implemented only by the core primitives.

:::note
`Encodable` is implemented by `Str`, `Int`, `Float`, and `Bool`. For structs, lists, maps, or `Dynamic`, use `ard/json` instead.
:::

```ard
use ard/encode
use ard/io

fn main() {
  io::print(encode::json("hello").expect(""))  // "hello"
  io::print(encode::json(42).expect(""))       // 42
  io::print(encode::json(3.14).expect(""))     // 3.14
  io::print(encode::json(true).expect(""))     // true
}
```

## API

### `trait Encodable`

The trait required by the encoder. It is implemented only by the primitive types.

```ard
trait Encodable {
  fn to_dyn() Dynamic
}
```

### `fn json(value: Encodable) Str!Str`

Encode a primitive value as a JSON string. Returns `Ok` with the JSON string or `Err` with an error message.

```ard
use ard/encode

let json = encode::json(200).expect("Failed to encode")
```
