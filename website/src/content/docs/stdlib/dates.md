---
title: Date and Time with ard/dates
description: Work with dates and time information using Ard's dates module.
---

The `ard/dates` module provides functions for working with date and time information.

The dates module provides:
- **Current date retrieval** to get today's date as a string

```ard
use ard/dates
use ard/io

fn main() {
  let today = dates::get_today()
  io::print("Today is: {today}")
}
```

## API

### `fn get_today() Str`

Get today's date as a string in standard date format.

```ard
use ard/dates

let date_str = dates::get_today()
```

## Examples

### Display Current Date

```ard
use ard/dates
use ard/io

fn main() {
  let today = dates::get_today()
  io::print(today)
}
```

### Log with Current Date

```ard
use ard/dates
use ard/io
use ard/fs

fn main() {
  let timestamp = dates::get_today()
  let message = "{timestamp}: Application started"
  fs::append("app.log", "{message}\n").expect("Failed to write log")
}
```
