## TODO

- [ ] when a diagnostic error is encountered, don't skip current statement
- [ ] omitting Maybe fields in structs will default to `None`
- [ ] calling functions with named arguments
  ```ard
  fn foo(bar: Int, baz: Str) { }
  foo(baz: "hello", bar: 5)
  ```
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
- [ ] methods on enums?
- [ ] eloquent relative conditions
  - `200 <= status <= 300`
- [ ] allow non-linear declarations at the top level
  -  i.e. a type declared at the top of the file, can reference one declared below
- [ ] matching on strings?
- [ ] `?` sugar for safely unwrapping maybes
- [ ] loops as expressions (comprehensions?)
  - `let doubled: [Int] = for i in 1..10 { i * 2 }`
- [ ] concurrency (Task?)
- [ ] inline block as expression
  ```
  let foo: Int = {
    // do stuff here
    let stuff = get_stuff()
    stuff + 5
  }
  ```
