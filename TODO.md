# TODO
- [ ] allow FFI in user land
- [ ] Dependency system
  * needs more thought
  * likely just a vendoring system
  * add dependencies section to ard.toml
  * `ard add [git-path]` to install from git repo
    * optionally add `@[version]` for a particular tag
- [ ] build Agent sdk
- [ ] I had to keep anonymous closure inference coverage in Go for List::keep; putting untyped closure params in stdlib Ard tests currently trips formatter/idempotence testsI had to keep anonymous closure inference coverage in Go for List::keep; putting untyped closure params in stdlib Ard tests currently trips formatter/idempotence tests
