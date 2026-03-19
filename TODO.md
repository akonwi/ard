# TODO
- [ ] allow FFI in user land
- [ ] Dependency system
  * needs more thought
  * likely just a vendoring system
  * add dependencies section to ard.toml
  * `ard add [git-path]` to install from git repo
    * optionally add `@[version]` for a particular tag
- [ ] build Agent sdk
- [ ] Revisit moving `List::keep` anonymous closure inference coverage into Ard stdlib tests.
  * Today that coverage stays in `compiler/bytecode/vm/list_test.go` instead of `compiler/std_lib/list.ard`.
  * The desired Ard test shape is an untyped closure like `List::keep(users, fn(u) { u.age >= 30 })`, which exercises checker inference on closure params and field access.
  * When that style appears in `std_lib/*.ard`, formatter/idempotence tests currently fail; the formatter crashes while rendering anonymous function params whose types are omitted/inferred.
  * Revisit after fixing the formatter path so stdlib Ard tests can cover this compiler behavior without needing an explicit type annotation workaround.
- [ ] Fix formatter handling for `private extern type` declarations.
  * `ard format std_lib` currently rewrites declarations like `private extern type WaitGroup`, `private extern type RawRequest`, and `private extern type Db/Tx` to non-private `extern type ...`.
  * This affects at least `compiler/std_lib/async.ard`, `compiler/std_lib/http.ard`, and `compiler/std_lib/sql.ard`.
  * The formatter should preserve visibility modifiers on extern type declarations and have regression coverage for both `extern type Foo` and `private extern type Foo`.
