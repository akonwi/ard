## TODO

- [ ] importing from other files (see ./docs/imports.md)
  - [x] support `pub` keyword in AST and parser
  - [x] support imports from other files in checker
  - [x] handle imports in interpreter
- [ ] syntax for generics on structs
  - `struct Box { item: $T }`
- [ ] methods on enums?
- [ ] matching on numbers
- [ ] calling functions with named arguments
- [ ] eloquent relative conditions
  - `200 <= status <= 300`
- [ ] ?: make less scripty
  - [ ] `main()` function for execution start
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
