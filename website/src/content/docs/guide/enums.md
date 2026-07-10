---
title: Enums
description: Learn about defining and using enums for representing discrete sets of values.
---

## Defining Enums

Enums are used to represent labels for a discrete set of options. In Ard, enums are simply labeled integers and cannot have associated data:

```ard
enum Status {
  active,
  inactive,
  pending
}

enum HttpStatus {
  ok,
  not_found,
  server_error,
  bad_request
}
```

Enums can have methods through `impl` blocks, just like structs:

```ard
enum Status { active, inactive }

impl Status {
  fn is_active() Bool {
    self == Status::active
  }
}
```

## Referencing Enum Values

Use the static operator `::` to reference enum variants:

```ard
enum Status {
  active,
  inactive,
}

let current_status = Status::active
```

## Matching On Enums

Use `match` expressions to do conditional logic based on the enum value:

```ard
use go:fmt

enum Priority {
  low,
  medium,
  high,
  critical,
}

fn handle_task(priority: Priority) {
  match priority {
    Priority::low => fmt::Println("Task can wait"),
    Priority::medium => fmt::Println("Task should be done today"),
    Priority::high => fmt::Println("Task needs attention soon"),
    Priority::critical => fmt::Println("Drop everything and handle this!"),
  }
}
```

## Practical Examples

### State Machines

```ard
use go:fmt

enum ConnectionState {
  disconnected,
  connecting,
  connected,
  error,
}

fn connection_successful() Bool {
  true
}

fn connect(state: ConnectionState) ConnectionState {
  match state {
    ConnectionState::disconnected => {
      fmt::Println("Attempting to connect...")
      ConnectionState::connecting
    },
    ConnectionState::connecting => {
      // Simulate connection logic
      match connection_successful() {
        true => ConnectionState::connected,
        false => ConnectionState::error,
      }
    },
    ConnectionState::connected => {
      fmt::Println("Already connected")
      ConnectionState::connected
    },
    ConnectionState::error => {
      fmt::Println("Connection failed, retrying...")
      ConnectionState::disconnected
    },
  }
}
```

## Enum Limitations

Unlike some languages, Ard enums:
- Cannot have associated data/values
- Are represented as named integer-like variants

For actual discriminated unions of various types (A.K.A. sum types) like in Rust, consider using <a href="/guide/types/#type-unions">type unions</a>:

```ard
struct Success { value: Str }
struct Failure { message: Str }

// supporting the possible shapes cannot be done with a plain enum
type Outcome = Success | Failure
```
