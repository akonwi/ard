# TODO
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
- [ ] support handling fiber panics
- [ ] allow omitting nullable arguments in function calls
- [ ] decode::path() could support both field name and array index
    - `fn decode::path(segments: [Str|Int], as: decode::Decoder<$Out>) $Out![decode::Error] `
- [ ] refactor ard/http request handlers so they don't need to construct the responses
  - introduce this as ard/http2 so it doesn't break existing code
  - handler signature should become `fn (req: Request, mut res: Response) Void!Str`
  - the handler can mutate the response accordingly
  - the library code will take care of flushing the response
  - if the handler returns an error, the library will use that as the response body
- [ ] for loops as expressions
  - evaluates to a list that can be assigned to a variable
    - can't be used in place of other expressions
  - broken loops: still a list up to the break point
- [ ] (idea): change `json::encode()` to only accept Dynamic  
  - it would make it easier on interpreter, to not have to use reflection to do the encoding
- [ ] replace angle brackets for generics. with parens or brackets?
- [ ] change for loop syntax to something like odin
  - `for [1,2,3] i { ... }`
- [ ] `Maybe.or_lazy()` to run a function to generate the fallback
