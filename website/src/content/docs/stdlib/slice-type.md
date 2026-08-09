---
title: Slice
description: Fixed-length shared views over contiguous list elements.
---

`Slice<T>` is a fixed-length view over a contiguous range of list elements. Creating a slice does not copy its elements.

```ard
let values = [10, 20, 30, 40]
let view: Slice<Int> = values
  .slice(start: 1, end: 3)
  .expect("valid bounds")
```

The range is zero-based and half-open, so this view contains `20` and `30`. Omitted bounds default to the beginning and end:

```ard
values.slice()                       // all elements
values.slice(start: 1)               // elements 1 through the end
values.slice(end: 3)                 // elements before index 3
values.slice(start: 1, end: 3)       // elements 1 and 2
```

Invalid bounds return `none`. Empty ranges, including one at the end, return an empty slice wrapped in `some`.

## Shared storage

A slice initially shares its selected element storage with its source. Mutating a visible element through a slice reference is observable through the source:

```ard
let values = mut [10, 20, 30]
let view = values.slice(start: 1).expect("valid bounds")
let writable = mut view

writable.set(0, 99)
// values is [10, 99, 30]
```

A slice cannot grow, shrink, append, prepend, or sort. If the source list grows and moves to new backing storage, existing slices remain valid but stay attached to the old storage.

A small slice may keep a much larger backing allocation alive. Use `to_list()` when the selected elements must have independent storage and the larger source allocation should be eligible for release.

## Methods

### `fn size() Int`

Return the number of visible elements.

### `fn is_empty() Bool`

Return whether the slice contains no elements.

### `fn at(index: Int) T?`

Return the visible element at `index`, or `none` when the slice has no element at that index, such as when the slice is empty or the index is outside `0..size()`.

### `fn slice(start: Int?, end: Int?) Slice<T>?`

Create another fixed-length shared view. Both bounds are optional.

### `fn to_list() [T]`

Allocate an ordinary growable list and shallow-copy the visible elements. The new list has independent top-level element storage.

### `fn set(index: Int, value: T) Bool`

Replace a visible element through a slice reference. Return `false` when `index` is out of bounds.

### `fn swap(l: Int, r: Int)`

Swap two visible elements through a slice reference.

## Go interop

An explicit `mut Slice<T>` can be passed to a compatible Go `[]T` parameter. Go receives a normal slice descriptor whose capacity is restricted to its visible length. Go may mutate or retain the visible elements; this is an explicit FFI trust boundary.

Go `*[]T` parameters do not accept `mut Slice<T>` because replacing the descriptor would violate the fixed-length contract.
