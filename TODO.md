# TODO
- [ ] FFI functions should be able to use idiomatic Go and compiler handles mappings
- [ ] Generic type comparison crash in co-located stdlib tests: calling `testing::equal(...)` or comparing values from generic stdlib functions (e.g. `list::concat`, `list::find`) inside a `test fn` co-located in a stdlib module causes a checker stack overflow / type-unification loop. Discovered while dogfooding `std_lib/list.ard` tests. Workaround: use `testing::assert(...)` with boolean expressions instead of `testing::equal(...)` for values from generic functions.

