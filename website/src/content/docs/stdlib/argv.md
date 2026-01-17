---
title: Command-line Arguments with ard/argv
description: Parse and access command-line arguments passed to your Ard program.
---

The `ard/argv` module provides a simple way to access command-line arguments passed to your Ard program.

The argv module provides:
- **Program name access** to get the name of the executable
- **Argument access** to retrieve all command-line arguments after the program name
- **Structured access** via the `Argv` struct

```ard
use ard/argv
use ard/io

fn main() {
  let args = argv::load()
  io::print("Program: {args.program}")
  
  for arg in args.arguments {
    io::print("Argument: {arg}")
  }
}
```

## API

### `struct Argv`

Contains parsed command-line arguments.

- **`program: Str`** - The name of the program/executable
- **`arguments: [Str]`** - List of arguments passed to the program

### `fn load() Argv`

Load and parse command-line arguments from the operating system.

```ard
use ard/argv

let args = argv::load()
```

## Examples

### Access Program Name and Arguments

```ard
use ard/argv
use ard/io

fn main() {
  let args = argv::load()
  io::print("Running: {args.program}")
  
  if args.arguments.size() == 0 {
    io::print("No arguments provided")
  } else {
    io::print("Got {args.arguments.size().to_str()} arguments:")
    for arg in args.arguments {
      io::print("  - {arg}")
    }
  }
}
```

### Process Named Arguments

```ard
use ard/argv
use ard/io

fn main() {
  let args = argv::load()
  
  for arg in args.arguments {
    match arg.starts_with("--") {
      true => io::print("Flag: {arg}"),
      false => io::print("Value: {arg}")
    }
  }
}
```
