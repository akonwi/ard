---
title: Map
description: Built-in map type syntax and methods.
---

Maps are Ard's key-value collections. A map type is written `[K:V]`, where `K` is the key type and `V` is the value type.

```ard
let scores: [Str:Int] = ["Ada": 10, "Grace": 12]
let names: [Int:Str] = [1: "one", 2: "two"]
```

## Methods

### `fn size() Int`

Return the number of entries.

```ard
let scores = ["Ada": 10]
let count = scores.size() // 1
```

### `fn get(key: K) V?`

Return the value for `key`, or `Maybe::new<V>()` if the key is absent.

```ard
let scores = ["Ada": 10]
let ada = scores.get("Ada").or(0)
let missing = scores.get("Grace") // Int?
```

### `fn has(key: K) Bool`

Return whether `key` exists in the map.

```ard
let scores = ["Ada": 10]
let ok = scores.has("Ada")
```

### `fn keys() [K]`

Return the map's keys as a list. Go map iteration order is not deterministic, so do not rely on the key order.

```ard
let scores = ["Ada": 10, "Grace": 12]
let names = scores.keys()
```

### `fn set(key: K, value: V)`

Set `key` to `value` in a mutable map.

```ard
mut scores: [Str:Int] = [:]
scores.set("Ada", 10)
```

### `fn delete(key: K)`

Remove `key` from a mutable map. Deleting an absent key is allowed.

```ard
mut scores = ["Ada": 10]
scores.delete("Ada")
```

## Module helpers

The [`ard/map`](/stdlib/map/) module provides helper functions such as `map::new`. Import that module when you want those helpers; map methods are available without an import.
