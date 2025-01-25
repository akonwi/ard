# Ard Programming Language

## Language Overview

Checkout the [language spec](./language-spec.md)

## TODO

- [ ] traditional for-loop
  ```ard
  for mut i = 1; i < 10; i =+2; {
    io.print(i)
  }
  ```
- [ ] remove `:` from type declarations
- [ ] trailing commas (lists, structs, maps, matches)
- [ ] fn return type is required otherwise, it's a void function
- [ ] replace `!` with `not`
- [ ] proper mutability checks
- [ ] methods on enums?
- [ ] using unary + member_access in conditions shouldn't require parens
  - fails: `if !a.b { }`
  - works: `if !(a.b) { }`
- [ ] matching on numbers
- [ ] matching on strings?
- [ ] loops as expressions
  - `let doubled: [Num] = for i in 1..10 { i * 2 }`
- [ ] concurrency (Task?)
- [ ] error handling
