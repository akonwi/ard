# TODO
- [ ] allow FFI in user land
- [ ] make duplicate FFI binding registration return an error
- [ ] Dependency system
  * needs more thought
  * likely just a vendoring system
  * add dependencies section to ard.toml
  * `ard add [git-path]` to install from git repo
    * optionally add `@[version]` for a particular tag
- [ ] build Agent sdk
- [ ] allow `main` to return non-`Void` values
- [ ] revisit target-aware embedded stdlib loading for runtime FFI helpers
  * `checker.FindEmbeddedModule(...)` is currently target-blind
  * most current non-checker usages are runtime FFI helpers (`compiler/ffi/decoders.go`, `compiler/ffi/http.go`)
  * this is probably not something to solve in the checker-validation pass right now
  * revisit once JS runtime/backend work makes those runtime paths concrete
- [ ] strengthen AIR result/maybe typing in nested contextual expressions
  * current Go target had to compensate during lowering when nested blocks/match arms/try expressions carried weaker AIR types like `Int!Void` even though the enclosing context required `Int!Str`
  * investigate whether AIR should normalize these nested contextual `Result`/`Maybe` expression types earlier so backends need less context-sensitive repair
  * treat this as an AIR-wide design follow-up, not just a Go-target workaround, because it may affect vm_next, diagnostics, and other backends
- [ ] automate current Go-target sample/server/manual checks into durable regression tests
  * cover the existing interactive stdin flows and server route assertions for `--target go`
  * normalize volatile response fields like `Date` where needed
