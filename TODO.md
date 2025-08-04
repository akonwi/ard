## TODO

- [ ] copy semantics
  ideas:
  A: create implicit copy when using `mut` keyword on read-only reference
  ```
  let map = [1: "one", 2: "two"] // immutable

  // as an argument
  foo(mut map) // foo receives a copy of `map` and the original stays in tact
  mut map2 = map // new binding to `map2` copies `map`
  ```
  works if the main use case is to maintain a consistent source and starting point for changing.
  where does it not work?
- [ ] when a diagnostic error is encountered, don't skip current statement. 2 options:
  - return a complete node with void where necessary
  - halt (c.halted = true) if it's a critical error and prevent cascades
- [ ] `Database.query_row<$V>(table: Str, expr: Str)`
- [ ] support FFI in stdlib OR add an `external` escape for definitions
- [ ] ffi
  - sort of like gleam
  - #external attribute on functions with external fn name
  - look for ffi defs in `./ffi` folder
  - only allow primitives across boundaries
  - all FFI functions must return `Result<$V, Str>`
- [ ] infer types in anonymous functions
- [ ] `Maybe.expect()` for panic shorthands
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
