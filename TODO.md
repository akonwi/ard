# TODO
- [ ] Fix Go target `else if` lowering bug
  * Repro seen in `~/Developer/agent/vaxis-ard/counter`: an `if/else if` chain for key handling lowered so later branches collapsed into a broad fallback; `r` reset was treated as decrement.
  * Add a compiler regression with an `else if` chain where an early branch, middle branch, and final fallback produce distinct values/effects.
- [ ] try to improve FFI perf by autocasting args to externs
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
- [ ] automate current Go-target sample/server/manual checks into durable regression tests
  * cover the existing interactive stdin flows and server route assertions for `--target go`
  * normalize volatile response fields like `Date` where needed
