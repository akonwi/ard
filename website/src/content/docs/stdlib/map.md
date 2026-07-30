---
title: ard/map
description: Helper for creating empty string-keyed maps.
---

The `ard/map` module currently provides one helper for creating empty maps with `Str` keys.

It is available from the prelude as `Map`. You can also import `ard/map` when you want the lowercase module namespace explicitly:

```ard
use ard/map
```

## API

### `new() [Str: $V]`

Create a new empty map with `Str` keys and values of type `$V`. The value type is usually inferred from the assignment context, or named with an explicit type argument — `let counts = mut Map::new<Int>()` starts an empty mutable map without a binding annotation or import through the prelude alias.

```ard
use ard/map

let labels: [Str: Str] = map::new()
let counts: [Str: Int] = map::new()
```

This is equivalent to an empty map literal with an explicit type:

```ard
let labels: [Str: Str] = [:]
```
