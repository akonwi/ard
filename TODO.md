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
