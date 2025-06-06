# Importing Code

Each ard file is a module that can be either a complete program or used by another module.
Imports are declared at the top of the file and begin with the `use` keyword.
The import path is a qualified path to the target module, without the file type ending.

```ard
use ard/io
use my_project/utils as helpers

io::print("")
helpers::calculate(42)
```

By default, Ard makes the imported module available by the last segment of the path.
This can be renamed with the `as name` syntax demonstrated above.

The Ard standard library is a collection of modules in the `ard/*` path.

## Module Resolution

Import paths are always absolute from the project root. There are no relative imports.
The project root is determined by the presence of an `ard.toml` file, which specifies
the project name.

For example, with this project structure:
```
my_calculator/
├── ard.toml          # contains: name = "my_calculator"
├── main.ard
├── utils.ard
└── math/
    └── operations.ard
```

You would import modules like this:
```ard
use my_calculator/utils
use my_calculator/math/operations

utils::helper_function()
operations::add(1, 2)
```

If no `ard.toml` file is present, the project name defaults to the root directory name.

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
