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

### Functions, Structs, Enums, and Traits
By default, functions, structs, enums, and traits are public and accessible from other modules.
To make them private (only accessible within the same file), they must be preceded with the `private` keyword.

```ard
// this can be called from another file
fn do_things(with: Int) {
  do_here(with)
}

// this cannot be referenced outside of the current file
private fn do_here(num: Int) {}
```

### Variables
Variables have different privacy rules based on mutability:

- **Immutable variables** (`let`) are **public** and can be accessed from other modules
- **Mutable variables** (`mut`) are **private** and cannot be accessed from other modules

```ard
// Public constants - accessible from other modules
let PI = 3.14159
let MAX_USERS = 1000

// Private state - not accessible from other modules  
mut user_count = 0
mut is_initialized = false
```

Usage from another module:
```ard
use my_project/constants

fn calculate_area(radius: Float) Float {
  constants::PI * radius * radius  // ✅ Can access immutable variables
  // constants::user_count         // ❌ Cannot access mutable variables
}
```

## Struct fields
All struct fields are public by default.
Methods are also public by default but can be made private with the `private` keyword.
