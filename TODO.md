## TODO

- [ ] refactor: checker.StructDef should have a separate array for methods
  - everything in `fields` makes it difficult to distinguish between properties and methods
- [ ] in ard/sqlite, implement prepared statements
  - needs API design
- [ ] allow omitting nullable arguments in function calls
- [ ] when a diagnostic error is encountered, don't skip current statement. 2 options:
  - return a complete node with void where necessary
  - halt (c.halted = true) if it's a critical error and prevent cascades
- [ ] `Database.query_row<$V>(table: Str, expr: Str)`
- [ ] support FFI in stdlib OR add an `external` escape for definitions
- [ ] ffi
  - sort of like gleam
  - #external attribute on functions with external fn name
  - look for ffi defs in `./ffi` folder
  - all FFI functions must return `Result<$V, Str>`
- [ ] introduce `ard/encode` package for marshalling into various formats
  - accept something and stringify as JSON `fn json(any: $A) Str!Error`
    - support options like casing for object keys; snake_case|kebab-case|pascalCase
  - probably replaces the `ard/json` package
- [ ] `try` on Maybe types
- [ ] support handling fiber panics
- [ ] infer types in anonymous functions
- [ ] syntax for generics on structs
  - `struct Box { item: $T }`
- [ ] eloquent relative conditions
  - `200 <= status <= 300`
- [ ] allow non-linear declarations at the top level
  -  i.e. a type declared at the top of the file, can reference one declared below
- [ ] matching on strings?
- [ ] `?` sugar for safely unwrapping maybes
- [ ] loops as expressions (comprehensions?)
  - `let doubled: [Int] = for i in 1..10 { i * 2 }`
- [ ] inline block as expression
  ```
  let foo: Int = {
    // do stuff here
    let stuff = get_stuff()
    stuff + 5
  }
  ```
- [ ] private variables
- [ ] equivalent of Gleam's `use`
  - sugar to denest callbacks
