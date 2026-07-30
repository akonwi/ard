---
title: List
description: Built-in list type syntax and methods.
---

Lists are Ard's growable ordered collections. A list type is written `[T]`, where `T` is the element type.

```ard
let numbers: [Int] = [1, 2, 3]
let names: [Str] = ["Ada", "Grace"]
```

## Methods

### `fn size() Int`

Return the number of elements.

```ard
let count = [1, 2, 3].size() // 3
```

### `fn at(index: Int) T?`

Return the element at `index`, or `Maybe::new<T>()` if the index is out of bounds.

```ard
let values = [10, 20]
let first = values.at(0).or(0) // 10
let missing = values.at(9)     // Int?
```

### `fn push(value: T) Int`

Append `value` through a list reference and return the new length.

```ard
let values = mut [1]
let size = values.push(42)
```

### `fn prepend(value: T) Int`

Insert `value` at the beginning through a list reference and return the new length.

```ard
let values = mut [2, 3]
values.prepend(1)
```

### `fn set(index: Int, value: T) Bool`

Replace the element at `index` through a list reference. Returns `true` if the index existed, or `false` if it was out of bounds.

```ard
let values = mut [1, 2, 3]
let updated = values.set(1, 20)
```

### `fn swap(l: Int, r: Int)`

Swap two elements through a list reference.

```ard
let values = mut [1, 2, 3]
values.swap(0, 2)
```

### `fn sort(cmp: fn(T, T) Bool)`

Sort through a list reference using a comparison callback. The callback returns `true` when the first argument should come before the second.

```ard
let values = mut [3, 1, 2]
values.sort(fn(a: Int, b: Int) Bool { a < b })
```

## Module helpers

The [`ard/list`](/stdlib/list/) module provides helper functions such as `list::map`, `list::keep`, and `list::find`. Import that module when you want those helpers; list methods are available without an import.
