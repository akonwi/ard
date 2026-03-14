# TODO
- [ ] FFI functions should be able to use idiomatic Go and compiler handles mappings
- [ ] Generic value equality at runtime: values returned from generic functions like `list::find` don't compare correctly with `==` or `testing::equal(...)` after being unwrapped from Maybe. The checker no longer crashes, but runtime equality comparisons fail silently. Reproduce: call `find([1,2,3], fn(n) { n == 3 })`, unwrap the Maybe, and compare with `== 3`.

