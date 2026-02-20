# TODO
- [ ] get rid of `@` for self referencing
  - could default to `self`
  - could allow renaming with:
  ```
  impl Foo as f {
    fn bar() {
      f.thing
    }
  }
  ```
- [ ] FFI functions should be able to use idiomatic Go and compiler handles mappings
