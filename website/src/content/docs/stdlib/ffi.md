---
title: FFI Helpers with ard/ffi
description: Helpers for direct host interop, including Go nil checks.
---

The `ard/ffi` module contains helpers for direct host interop. These functions are intentionally low-level; prefer ordinary Ard APIs when they already model the behavior you need.

## API

### `fn is_nil(value: $T) Bool`

Return `true` when the value's Go representation is nil.

```ard
use ard/ffi
use go:net/http as gohttp

fn request_path(req: mut gohttp::Request) Str {
  match ffi::is_nil(req.URL) {
    true => "",
    false => req.URL.Path,
  }
}
```

`is_nil` is a normal generic function implemented by Go FFI. It returns `false` for values whose Go representation cannot be nil, such as integers, strings, and structs.

:::caution
`is_nil` only tests the value passed to it. The argument is evaluated first, so `ffi::is_nil(req.URL)` can still panic if `req` itself is nil.
:::

## Why this is not Maybe

Go `nil` is a host value-state. Depending on the API it may mean default behavior, uninitialized storage, or a sentinel value, not always domain-level absence. Ard's `Maybe<T>` remains the way to model intentional absence in Ard APIs.

Use `is_nil` when crossing a direct Go boundary and you need to inspect that host state explicitly.
