# TODO
- [ ] FFI functions should be able to use idiomatic Go and compiler handles mappings
- [ ] Generic type variables emit KindDynamic at runtime instead of the resolved concrete kind. When a generic function like `list::find` returns a value, the emitter/VM wraps it as KindDynamic even though the checker has resolved the type variable to a concrete type (e.g. Int). This required a workaround in `evalCompare` (vm.go) to compare underlying Go values when either operand is KindDynamic. The proper fix is to thread the resolved type through emission so the runtime object gets the correct kind (e.g. KindInt) rather than KindDynamic.

