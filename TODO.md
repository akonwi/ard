# TODO

## Initial 0.1.0 build
- [ ] need to be able to alias fn signatures with `type`
  - a single type union should act as an alias
  - matching is only required if it supports variadic types
- [ ] in ard/sqlite, implement prepared statements
  - needs API design
- [ ] `match` as a replacement for if statements
  - like cascading if conditions rather than chaining if-else-if
  - supports catch-all case
  ```
  match {
    condition_1 => {},
    condition_2 => {},
    condition_3 => {},
    _ => {}
  }
  ```
- [ ] support json encode options like casing for object keys; snake_case|kebab-case|pascalCase
  - idea: `fn encode_with(data: $T, casing: CaseEnum) Str!Str`
- [ ] `Maybe.expect()` for panicing
- [ ] `?` sugar for safely unwrapping maybes
- [ ] `fn decode::at(segments: [$Seg], as: decode::Decoder<$Out>) $Out![decode::Error] `
  - $Seg could be either strings or ints
    - as ints, act as an array index
- [ ] support handling fiber panics
- [ ] syntax for generics on structs
  - `struct Box { item: $T }`
- [ ] eloquent relative conditions
  - `200 <= status <= 300`
- [ ] allow non-linear declarations at the top level
  -  i.e. a type declared at the top of the file, can reference one declared below
- [ ] matching on strings?
  - probably should be enums
  - could be accomplished with a plain `match` and  various conditions
- [ ] allow omitting nullable arguments in function calls
- [ ] inline block as expression
  ```
  let foo: Int = {
    // do stuff here
    let stuff = get_stuff()
    stuff + 5
  }
  ```
- [ ] private module variables
- [ ] start versioning (begin with 0.1.0)

## Future improvements
- [ ] infer types in anonymous functions
- [ ] equivalent of Gleam's `use`
  - sugar to denest callbacks
- [ ] selective variable capture for closures
  - data optimization
  -👇🏿 the returned fn should only have `as` in its scope, not the entire scope
- [ ] loops as expressions (comprehensions?)
  - `let doubled: [Int] = for i in 1..10 { i * 2 }`
  ```
  fn first(as: fn(Dynamic) $T![decode::Error]) fn(Dynamic) $T![decode::Error] {
    fn(data: Dynamic) $T![decode::Error] {
      let list = try decode::run(data, decode::list(as))
      match list.size() {
        0 => Result::err([decode::Error{path: [""], expected: "non-empty list", found: "empty list"}]),
        _ => Result::ok(list.at(0))
      }
    }
  }
  ```
- [ ] FFI functions should be able to use idiomatic Go and compiler handles mappings
- [ ] Support omitting `Void` in fn type declarations and externs
- [ ] define enum values
  ```
  enum HttpStatus {
    200,
    201,
    404,
    429
  }

  fn HttpStatus::from_int(int: Int) HttpStatus? {}
  ```
