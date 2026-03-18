# TODO
- [ ] FFI functions should be able to use idiomatic Go and compiler handles mappings
- [ ] Generic type variables emit KindDynamic at runtime instead of the resolved concrete kind. When a generic function like `list::find` returns a value, the emitter/VM wraps it as KindDynamic even though the checker has resolved the type variable to a concrete type (e.g. Int). This required a workaround in `evalCompare` (vm.go) to compare underlying Go values when either operand is KindDynamic. The proper fix is to thread the resolved type through emission so the runtime object gets the correct kind (e.g. KindInt) rather than KindDynamic.
- [ ] allow FFI in user land
- [ ] Dependency system
  * needs more thought
  * likely just a vendoring system
  * add dependencies section to ard.toml
  * `ard add [git-path]` to install from git repo
    * optionally add `@[version]` for a particular tag
- [ ] build Agent sdk
- [x] Opaque types — `extern type` for opaque FFI handles (SQL connections, HTTP requests, WaitGroups)
- [x] Idiomatic FFI `any` type support — Go `any`/`interface{}` maps to Ard Dynamic/extern types
