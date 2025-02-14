## TODO

- [ ] methods on enums?
- [ ] using unary + member_access in if conditions shouldn't require parens
  - fails: `if !a.b { }`
  - works: `if !(a.b) { }`
- [ ] matching on numbers
- [ ] loops as expressions
  - `let doubled: [Int] = for i in 1..10 { i * 2 }`
- [ ] two part cursor in loops
  - `for person,index in employees { io.print(index) }`
- [ ] named parameters
- [ ] concurrency (Task?)
- [ ] error handling
- [ ] matching on strings?
- [ ] should
