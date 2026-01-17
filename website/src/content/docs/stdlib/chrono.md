---
title: Current Time with ard/chrono
description: Get Unix timestamps using Ard's chrono module.
---

The `ard/chrono` module provides access to the current Unix timestamp (seconds since 1970-01-01 UTC).

The chrono module provides:
- **Current time as integer timestamp** for database operations and calculations

```ard
use ard/chrono

fn main() {
  let now = chrono::now()
  io::print("Current Unix timestamp: {now}")
}
```

## API

### `fn now() Int`

Get the current Unix timestamp in seconds.

Returns an integer representing seconds since 1970-01-01 UTC.

```ard
use ard/chrono

let now = chrono::now()
io::print("Current timestamp: {now}")
```

## Examples

### Cache TTL Calculation

```ard
use ard/chrono
use ard/sql

fn cache_set(db: sql::Database, key: Str, value: Dynamic, ttl_seconds: Int) {
  let now = chrono::now()
  let expires_at = now + ttl_seconds
  db.query("INSERT INTO cache (key, value, expires_at) VALUES (?, ?, ?)")
    .run([key, json::encode(value), expires_at])
}
```

### Cache Expiration Check

```ard
use ard/chrono

fn is_expired(expires_at: Int) Bool {
  let now = chrono::now()
  expires_at <= now
}
```

### Timestamp Comparison

```ard
use ard/chrono

let now = chrono::now()
let one_hour_ago = now - 3600
let one_hour_later = now + 3600

io::print("Past: {one_hour_ago}")
io::print("Now: {now}")
io::print("Future: {one_hour_later}")
```

## Use Cases

- **Cache TTL Management** - Calculate expiration times and check if cached data is stale
- **Database Timestamps** - Store current time in `INTEGER` columns
- **Rate Limiting** - Track request windows by timestamp
- **Event Logging** - Record when events occurred
- **Time Comparisons** - Check if deadlines have passed

## Return Type

`chrono::now()` returns an `Int` - the Unix timestamp in seconds.

```ard
use ard/chrono

let now = chrono::now()
// now is an integer, use it directly
let future = now + 3600
```

## Precision

- Returns **seconds** (integer) since Unix epoch (1970-01-01 UTC)
- No fractional seconds or nanoseconds
- Sufficient for cache TTLs, rate limiting, and most timestamp needs

## Timezone

- Always returns UTC (Unix timestamps are timezone-agnostic)
- For human-readable local time with timezone, use `ard/dates`

## Integration with Other Modules

### With `ard/duration` for future timestamps

```ard
use ard/chrono
use ard/duration

let now = chrono::now()
// Add 1 hour (duration returns nanoseconds, divide to get seconds)
let in_one_hour = now + duration::from_hours(1) / 1000000000
```

### With `ard/dates` for human-readable output

```ard
use ard/chrono
use ard/dates

let human_date = dates::get_today()  // "2025-01-17"
let now = chrono::now()
io::print("{human_date} [{now}]")
```

## Error Handling

`chrono::now()` returns an `Int` directly—no error handling needed. System clock failures are extremely rare and not worth defensive programming.
