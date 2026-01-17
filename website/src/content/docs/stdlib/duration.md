---
title: Time Duration Conversion with ard/duration
description: Convert human-readable time durations to nanoseconds using Ard's duration module.
---

The `ard/duration` module provides utility functions to convert common time units into nanoseconds, which is the standard duration unit used throughout Ard.

The duration module provides:
- **Nanosecond conversions** from various time units
- **Helper functions** for readable duration specification in async and time-related operations

```ard
use ard/duration
use ard/async

fn main() {
  // Sleep for 1 second
  async::sleep(duration::from_seconds(1))
}
```

## API

### `fn from_seconds(s: Int) Int`

Convert seconds to nanoseconds.

```ard
use ard/duration

duration::from_seconds(5)  // 5,000,000,000 nanoseconds
```

### `fn from_millis(ms: Int) Int`

Convert milliseconds to nanoseconds.

```ard
use ard/duration

duration::from_millis(500)  // 500,000,000 nanoseconds
```

### `fn from_minutes(m: Int) Int`

Convert minutes to nanoseconds.

```ard
use ard/duration

duration::from_minutes(2)  // 120,000,000,000 nanoseconds
```

### `fn from_hours(h: Int) Int`

Convert hours to nanoseconds.

```ard
use ard/duration

duration::from_hours(1)  // 3,600,000,000,000 nanoseconds
```
