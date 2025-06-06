# Importing Code

Each ard file is a module that can be either a complete program or used by another module.
Imports are declared at the top of the file and begin with the `use` keyword.
The import path is a qualified path to the target module, without the file type ending.

```ard
use ard/io
use pkg/thing as stuff

io::print("")
stuff::get(1)
```

By default, Ard makes the imported module available by the last segment of the path.
This can be renamed with the `as name` syntax demonstrated above.

The Ard standard library is a collection of modules in the `ard/*` path.

## Controlling what can be exported
Just like in Rust or Gleam, every declaration in a module is by default, only accessible within the same file.
In order to reuse a declaration in another module, the declaration must be preceded with the `pub` keyword to signal that it is "public".

```ard
/* this cannot be referenced outside of the current file */
fn do_here(num: Int) {}

/* this can be called from another file */
pub fn do_things(with: Int) {
  do_here(with)
}
```

## Struct fields
A public struct's fields are always public.
Its methods are private though and can be made public.
