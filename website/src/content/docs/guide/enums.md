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

## Referencing Enum Values

Use the static operator `::` to reference enum variants:

```ard
let current_status = Status::active
let response_code = HttpStatus::ok
```

## Matching On Enums

Use `match` expressions to do conditional logic based on the enum value:

```ard
enum Priority {
  low,
  medium,
  high,
  critical
}

fn handle_task(priority: Priority) {
  match priority {
    Priority::low => io::print("Task can wait")
    Priority::medium => io::print("Task should be done today")
    Priority::high => io::print("Task needs attention soon")
    Priority::critical => io::print("Drop everything and handle this!")
  }
}
```

## Practical Examples

### State Machines

```ard
enum ConnectionState {
  disconnected,
  connecting,
  connected,
  error,
}

mut state = ConnectionState::disconnected

fn connect() ConnectionState {
  state = match state {
    ConnectionState::disconnected => {
      io::print("Attempting to connect...")
      ConnectionState::connecting
    },
    ConnectionState::connecting => {
      // Simulate connection logic
      match connection_successful() {
        true => ConnectionState::connected
        false => ConnectionState::error
      }
    },
    ConnectionState::connected => {
      io::print("Already connected")
      ConnectionState::connected
    },
    ConnectionState::error => {
      io::print("Connection failed, retrying...")
      ConnectionState::disconnected
    },
  }
}
```

## Enum Limitations

Unlike some languages, Ard enums:
- Cannot have associated data/values
- Are essentially named integers
- Cannot have methods defined on them directly

For actual discriminated unions of various types (A.K.A. sum types) like in Rust, consider using <a href="/guide/types/#type-unions">type unions</a>:

```ard
type Success = { value: Str }
type Error = { message: Str }
type Outcome = Success | Error // supporting the possible shapes cannot be done with a plain enum
```
