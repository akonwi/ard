---
title: Map Operations with ard/map
description: Work with maps using utility functions from the ard/map module.
---

The `ard/map` module provides utility functions for working with maps.

:::note
The `ard/map` module is a prelude module. It is automatically imported and aliased as `Map` in all programs, allowing methods to be accessed with the `Map::` namespace (e.g., `Map::new()`).
:::

The map module provides:
- **Map creation** with `Map::new()` for easily initializing empty maps

```ard
use ard/map

fn main() {
  let config: [Str:Str] = Map::new()
}
```

## API

### `fn new() [Str:$V]`

Create a new empty map with string keys. The value type must be explicitly specified.

```ard
use ard/map

// Create an empty string-to-string map
let map1: [Str:Str] = Map::new()

// Create an empty string-to-int map
let map2: [Str:Int] = Map::new()
```

## Examples

### Initialize an Empty Map

```ard
use ard/map
use ard/io

fn main() {
  let config: [Str:Str] = Map::new()
  config.set("host", "localhost")
  config.set("port", "8080")
  
  io::print(config.at("host"))  // localhost
}
```

### Build a Configuration Map

```ard
use ard/map

fn main() {
  let settings: [Str:Str] = Map::new()
  settings.set("theme", "dark")
  settings.set("language", "en")
  settings.set("timezone", "UTC")
}
```

### Create a Lookup Table

```ard
use art/map

fn main() {
  let status_codes: [Str:Int] = Map::new()
  status_codes.set("ok", 200)
  status_codes.set("created", 201)
  status_codes.set("bad_request", 400)
  status_codes.set("unauthorized", 401)
  status_codes.set("not_found", 404)
}
```
