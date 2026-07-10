---
title: Array
description: Built-in array type syntax and methods.
---

Arrays are Ard's fixed-length ordered collections. An array type is written `[T; N]`, where `T` is the element type and `N` is the length.

```ard
let rgb: [Byte; 3] = [255, 128, 0]
let empty: [Int; 0] = []
```

The length is part of the type, so `[Byte; 3]` and `[Byte; 4]` are different types.

## Methods

### `fn size() Int`

Return the array length.

```ard
let rgb: [Byte; 3] = [255, 128, 0]
let count = rgb.size() // 3
```

### `fn at(index: Int) T?`

Return the element at `index`, or `Maybe::new<T>()` if the index is out of bounds.

```ard
let rgb: [Byte; 3] = [255, 128, 0]
let zero: Byte = 0
let red = rgb.at(0).or(zero)
let missing = rgb.at(9) // Byte?
```

## Construction

Arrays use normal list literal syntax with an expected array type. The literal length must match exactly.

```ard
let point: [Int; 2] = [10, 20]
```

This is an error because the length does not match:

```ard
let point: [Int; 2] = [10, 20, 30]
```

## Lists vs arrays

Use `[T]` for growable lists and `[T; N]` for fixed-size arrays. There is no implicit list-to-array or array-to-list conversion.
