---
title: ard/list
description: Generic helpers for creating, transforming, searching, and partitioning lists.
---

The `ard/list` module provides a small set of generic helpers for working with Ard lists.

Import it explicitly when needed:

```ard
use ard/list
```

## API

### `new() [$T]`

Create a new empty list. The expected type is usually inferred from context.

```ard
use ard/list

let nums: [Int] = list::new()
```

### `concat(a: [$T], b: [$T]) [$T]`

Return a new list containing the elements of `a` followed by the elements of `b`.

```ard
use ard/list

let combined = list::concat([1, 2, 3], [4, 5])
```

### `drop(from: [$T], till: Int) [$T]`

Return a new list containing elements whose index is greater than or equal to `till`.

```ard
use ard/list

let tail = list::drop([1, 2, 3, 4], 2) // [3, 4]
```

### `keep(list: [$T], where: fn($T) Bool) [$T]`

Return a new list containing only values for which `where` returns `true`.

```ard
use ard/list

let evens = list::keep([1, 2, 3, 4], fn(n: Int) Bool { n % 2 == 0 })
```

### `map(list: [$A], transform: fn($A) $B) [$B]`

Transform each element and return the transformed values in order.

```ard
use ard/list

let doubled = list::map([1, 2, 3], fn(n: Int) Int { n * 2 })
```

### `find(list: [$T], where: fn($T) Bool) $T?`

Return the first matching value, or `none` if no value matches.

```ard
use ard/list

let found = list::find([1, 2, 3], fn(n: Int) Bool { n == 2 })
```

### `partition(list: [$T], where: fn($T) Bool) Partition<$T>`

Split a list into matching and non-matching values.

```ard
use ard/list

let parts = list::partition([1, 2, 3, 4], fn(n: Int) Bool { n > 2 })
// parts.selected == [3, 4]
// parts.others == [1, 2]
```

### `partition_int(list: [Int], where: fn(Int) Bool) IntPartition`

Specialized `Int` version of `partition`.

## Types

### `Partition<$T>`

```ard
struct Partition {
  selected: [$T],
  others: [$T],
}
```

### `IntPartition`

```ard
struct IntPartition {
  selected: [Int],
  others: [Int],
}
```
