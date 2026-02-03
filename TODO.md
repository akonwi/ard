# TODO
- [ ] `Maybe.or_lazy()` to run a function to generate the fallback
- [ ] FFI functions should be able to use idiomatic Go and compiler handles mappings
- [ ] compile time constant variables
  - should readonly variables with a literal value be considered constants?
    pro: simpler than adding a new `const` keyword and just works
  - would allow limiting variables in `match Int` patterns to constants for better analysis that there are no conflicts or overlaps in patterns
- [ ] **i'm not sure what this means anymore** selective variable capture for closures
  - data optimization
  -👇🏿 the returned fn should only have `as` in its scope, not the entire scope
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
