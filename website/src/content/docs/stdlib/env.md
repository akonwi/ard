---
title: Environment Variables with ard/env
description: Access environment variables from your Ard program.
---

The `ard/env` module provides functions for reading environment variables from the operating system.

The env module provides:
- **Environment variable access** with the `get` function
- **Maybe type returns** for safe handling of undefined variables

```ard
use ard/env
use ard/io

fn main() {
  match env::get("HOME") {
    path => io::print("Home directory: {path}"),
    _ => io::print("HOME not set")
  }
}
```

## API

### `fn get(name: Str) Str?`

Get the value of an environment variable by name. Returns a `Maybe` type - `some` if the variable is set, `none` if it's not.

```ard
use ard/env

let home = env::get("HOME")
```

## Examples

### Access a Single Environment Variable

```ard
use ard/env
use ard/io

fn main() {
  match env::get("USER") {
    user => io::print("User: {user}"),
    _ => io::print("USER not set")
  }
}
```

### Use Default Value if Not Set

```ard
use ard/env

fn main() {
  let debug_mode = env::get("DEBUG").or("false")
  io::print("Debug mode: {debug_mode}")
}
```

### Check Multiple Environment Variables

```ard
use ard/env
use ard/io

fn main() {
  let host = env::get("DB_HOST").or("localhost")
  let port = env::get("DB_PORT").or("5432")
  let user = env::get("DB_USER").or("postgres")
  
  io::print("Database: {user}@{host}:{port}")
}
```

### Build Configuration from Environment

```ard
use ard/env
use ard/io

struct Config {
  api_url: Str,
  api_key: Str,
  timeout: Str
}

fn main() {
  let config = Config {
    api_url: env::get("API_URL").or("https://api.example.com"),
    api_key: env::get("API_KEY").or(""),
    timeout: env::get("REQUEST_TIMEOUT").or("30")
  }
  
  io::print("API URL: {config.api_url}")
}
```

### Validate Required Environment Variables

```ard
use ard/env
use ard/io

fn main() {
  match env::get("DATABASE_URL") {
    url => {
      io::print("Using database: {url}")
    },
    _ => {
      io::print("ERROR: DATABASE_URL environment variable not set")
    }
  }
}
```
