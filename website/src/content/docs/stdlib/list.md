---
title: List Operations with ard/list
description: Work with lists using functional operations like map, filter, and partition.
---

The `ard/list` module provides functional operations for working with lists.

:::note
The `ard/list` module is a prelude module. It is automatically imported and aliased as `List` in all programs, allowing methods to be accessed with the `List::` namespace (e.g., `List::new()`, `List::map()`).
:::

The list module provides:
- **List creation** with `List::new()`
- **Functional transformations** like map, filter, and find
- **List utilities** for concatenation, dropping elements, and partitioning

```ard
use ard/list

fn main() {
  let numbers = [1, 2, 3, 4, 5]
  let evens = List::keep(numbers, fn(n) { n % 2 == 0 })
}
```

## API

### `fn new() [$T]`

Create a new empty list. Type parameters must be explicitly provided.

```ard
use ard/list

let nums: [Int] = List::new<Int>()
```

### `fn concat(a: [$T], b: [$T]) [$T]`

Concatenate two lists into a new list, with elements from `a` followed by elements from `b`.

```ard
use ard/list

let a = [1, 2, 3]
let b = [4, 5, 6]
let combined = List::concat(a, b)  // [1, 2, 3, 4, 5, 6]
```

### `fn drop(from: [$T], till: Int) [$T]`

Create a new list with elements starting from the given index. Elements before the index are dropped.

```ard
use ard/list

let list = [1, 2, 3, 4, 5]
let dropped = List::drop(list, 2)  // [3, 4, 5]
```

### `fn keep(list: [$T], where: fn($T) Bool) [$T]`

Filter a list, keeping only elements that match the given predicate function.

```ard
use ard/list

let numbers = [1, 2, 3, 4, 5]
let evens = List::keep(numbers, fn(n) { n % 2 == 0 })  // [2, 4]
```

### `fn map(list: [$A], transform: fn($A) $B) [$B]`

Transform each element in a list using the given function, returning a new list of the transformed elements.

```ard
use ard/list

let numbers = [1, 2, 3]
let doubled = List::map(numbers, fn(n) { n * 2 })  // [2, 4, 6]
```

### `fn find(list: [$T], where: fn($T) Bool) $T?`

Find the first element in a list that matches the given predicate. Returns a `Maybe` type.

```ard
use ard/list

let numbers = [1, 2, 3, 4, 5]
let first_even = List::find(numbers, fn(n) { n % 2 == 0 })  // some(2)
```

### `fn partition(list: [$T], where: fn($T) Bool) Partition<$T>`

Split a list into two based on a predicate. Returns a `Partition` struct with `selected` and `others` fields.

```ard
use ard/list

let numbers = [1, 2, 3, 4, 5]
let parts = List::partition(numbers, fn(n) { n > 2 })
// parts.selected = [3, 4, 5]
// parts.others = [1, 2]
```

### `struct Partition<$T>`

Result of partitioning a list.

- **`selected: [$T]`** - Elements matching the predicate
- **`others: [$T]`** - Elements not matching the predicate
