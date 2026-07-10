---
title: ard/unsafe
description: Compiler-backed escape hatches for opaque Any values and host nil state.
---

The `ard/unsafe` module contains compiler-backed helpers for operations Ard intentionally keeps out of the safe core language.

Use this module at interop boundaries, especially when working with opaque `Any` values or Go values that may carry nil state.

:::caution
`ard/unsafe` operations are explicit escape hatches. Prefer typed Ard APIs when possible.
:::

## API

### `cast(value: Any) $T?`

Attempt to recover a typed value from an opaque `Any`. Returns `some(value)` when the contained value has the requested type and `none` otherwise.

```ard
use ard/unsafe

let value: Any = "hello"
let text = unsafe::cast<Str>(value)
```

`Any` is opaque: it has no fields or methods of its own. Use `unsafe::cast<T>` when you intentionally need to inspect or recover a value.

### `is_nil(value: Any) Bool`

Return `true` when the value's backend representation is nil.

```ard
use ard/unsafe
use go:net/http as http

fn has_url(req: mut http::Request) Bool {
  !unsafe::is_nil(req.URL)
}
```

`is_nil` is mainly for direct Go interop. Go nil is a host value state, not Ard absence. Ard APIs should still use `Maybe<T>` when they intentionally model optional values.

:::caution
Arguments are evaluated before `is_nil` is called. For example, `unsafe::is_nil(req.URL)` can still panic first if `req` itself is nil.
:::
