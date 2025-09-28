# TODO

## Initial 0.1.0 build
- [ ] in ard/sqlite, implement prepared statements
  - needs API design
- [ ] support json encode options like casing for object keys; snake_case|kebab-case|pascalCase
  - idea: `fn encode_with(data: $T, casing: CaseEnum) Str!Str`
- [ ] `fn decode::at(segments: [$Seg], as: decode::Decoder<$Out>) $Out![decode::Error] `
  - $Seg could be union of strings or ints
    - as ints, act as an array index
- [ ] support handling fiber panics
- [ ] syntax for generics on structs
  - `struct Box { item: $T }`
- [ ] eloquent relative conditions
  - `200 <= status <= 300`
- [ ] allow non-linear declarations at the top level
  -  i.e. a type declared at the top of the file, can reference one declared below
- [ ] allow omitting nullable arguments in function calls
- [ ] inline block as expression
  ```
  let foo: Int = {
    // do stuff here
    let stuff = get_stuff()
    stuff + 5
  }
  ```
- [ ] start versioning (begin with 0.1.0)

## Future improvements
- [ ] `?` sugar for safely unwrapping maybes
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
    Ok: 200,
    Created: 201,
    Not_Found: 404,
    // ...
  }

  fn HttpStatus::from_int(int: Int) HttpStatus? {}
  ```
- [ ] compile time constant variables
  - should readonly variables with a literal value be considered constants?
    pro: simpler than adding a new `const` keyword and just works
  - would allow limiting variables in `match Int` patterns to constants for better analysis that there are no conflicts or overlaps in patterns
