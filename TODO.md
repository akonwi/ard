## TODO

- [ ] infer types in anonymous functions
- [ ] `Maybe.expect()` for panic shorthands
- [ ] syntax for generics on structs
  - `struct Box { item: $T }`
- [ ] methods on enums?
- [ ] calling functions with named arguments
- [ ] eloquent relative conditions
  - `200 <= status <= 300`
- [ ] ?: make less scripty
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
