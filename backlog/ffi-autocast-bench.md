# FFI autocast benchmark experiment

Branch: `experiment/ffi-autocast-bench`

## Baseline snapshot

Captured before any changes:

```bash
cd compiler && go test -run '^$' -bench . -benchmem ./bytecode
```

Environment:
- goos: darwin
- goarch: arm64
- cpu: Apple M3 Pro

Results:

```text
goos: darwin
goarch: arm64
pkg: github.com/akonwi/ard/bytecode
cpu: Apple M3 Pro
BenchmarkBytecodeRun-12    	     818	   1469854 ns/op	    8680 B/op	     145 allocs/op
PASS
ok  	github.com/akonwi/ard/bytecode	1.362s
```

## Runtime benchmark snapshot

Saved separately in an uncommitted file:
- `compiler/benchmarks/results/ffi-autocast-baseline-runtime.txt`

Primary signals from `cd compiler && ./benchmarks/run.sh`:
- `decode_pipeline`
  - `vm`: 254.0 ms
  - `vm_next`: 321.1 ms
  - `go`: 90.7 ms
  - `native-go`: 23.2 ms
- `fs_batch`
  - `vm`: 114.2 ms
  - `vm_next`: 117.2 ms
  - `go`: 193.6 ms
  - `native-go`: 109.4 ms
- `sql_batch`
  - `vm`: 47.6 ms
  - `vm_next`: 51.9 ms
  - `go`: 22.5 ms
  - `native-go`: 12.0 ms

## Experiment focus

Goal:
- treat this as a general typed-extern ABI experiment for `vm_next` and the Go target, not just a one-off micro-optimization
- improve performance by reducing generic `any`/reflection/cast-heavy extern adaptation work
- use the results to judge whether the approach is worth carrying forward for eventual userland FFI

Primary benchmark targets:
- `decode_pipeline`
  - best `vm_next` signal for dynamic/list/map/result-heavy extern traffic
- `sql_batch`
  - best mixed signal for extern handles, `[]any` argument conversion, returned dynamic rows, and decode follow-up work

Secondary target:
- `fs_batch`
  - keep as a guardrail, but do not use it as the main success signal because syscall cost dominates more of the runtime

## Findings from trace-through

### `decode_pipeline`
Hot externs are:
- `JsonToDynamic`
- `ExtractField`
- `DynamicToList`
- `DynamicToMap`
- `DecodeInt`
- `DecodeString`

This path is mostly conversion-heavy and should be the best place to measure `vm_next` autocast improvements.

### `sql_batch`
Hot externs are:
- `SqlCreateConnection`
- `SqlBeginTx`
- `SqlCommit`
- `SqlClose`
- `SqlExecute`
- `SqlQuery`
- then decode externs on returned row maps

This path exercises:
- extern handles (`Db`, `Tx`)
- `[]any` parameter conversion
- `[]any` / `map[string]any` result conversion
- decode follow-up work

### `fs_batch`
Most externs are already simple typed string/bool/error calls. It is still useful as a reality check, but it is a weaker pure autocast signal.

## Implementation plan

### Phase 1: stabilize measurement
- [ ] keep the current runtime benchmark snapshot file uncommitted for local comparison
- [ ] optionally add narrower benchmark invocations for fast iteration:
  - `./benchmarks/run.sh decode_pipeline sql_batch`
  - maybe lower `--runs` during development, then restore for final measurement
- [ ] if needed, add targeted Go test benchmarks for isolated `vm_next` extern adapter overhead so regressions are easier to attribute

### Phase 2: `vm_next` adapter fast paths
Target the generated adapter + bridge layer first.

Current expensive pattern for non-primitive args:
- `bridge.HostArg(args, i, reflect.TypeFor[T]())`
- `generatedHostCast[T](...)`

Experiment ideas:
- [ ] add specialized bridge methods for common FFI shapes so adapters can skip the `any` roundtrip where possible
- [ ] prioritize shapes seen in `decode_pipeline` and `sql_batch`:
  - `[]any`
  - extern handles / empty-interface-backed extern values (`Db`, `Tx`, similar)
  - maybe `map[string]string`
  - `Maybe[T]` if it shows up in hot paths
- [ ] update adapter generation to emit those direct bridge calls instead of `HostArg(... reflect.TypeFor[T]()) + generatedHostCast[T]`
- [ ] validate that return conversion remains correct for dynamic/list/map/result values

### Phase 3: Go-target extern adaptation follow-up
Focus on the SQL-heavy paths first, since general stdlib FS calls are already fairly direct.

Current suspicious spots:
- `lowerUnionArgToAny`
- `lowerUnionSliceArgToAny`
- repeated result wrapping around direct stdlib FFI calls

Experiment ideas:
- [ ] inspect generated Go for `sql_batch` and identify the heaviest conversion helpers in the hot loop
- [ ] specialize union/list-to-`[]any` lowering where the target extern signature is already known
- [ ] avoid generic helper churn if a typed direct conversion can be emitted instead

### Phase 4: compare and decide
Success criteria:
- [ ] `decode_pipeline` improves measurably on `vm_next`
- [ ] `sql_batch` improves measurably on `vm_next` and/or Go target
- [ ] no correctness regressions in benchmark output verification
- [ ] changes do not obviously worsen unrelated runtime benchmarks

Decision criteria:
- keep going if the fast paths produce clear wins on the primary targets without making the ABI/codegen story much more brittle
- scale back if the complexity rises faster than the observed perf gains

## Outcome summary

Net outcome: mildly positive, not transformative.

What landed well:
- broader `vm_next` bridge fast paths for common extern shapes
- broader `vm_next` direct return conversion fast paths for common `Maybe`, list, and map shapes
- Go-target helper-based union/union-slice conversion for SQL-oriented `any` / `[]any` extern calls

Observed direction vs original baseline:
- `vm_next decode_pipeline`
  - `321.1 ms` → about `306.3 ms` in the final focused rerun
  - likely improved, but still noisy and still dominated by deeper dynamic/decode work
- `vm_next sql_batch`
  - `51.9 ms` → about `49.6 ms`
  - small but credible improvement
- `go sql_batch`
  - `22.5 ms` → about `21.7 ms`
  - small improvement
- `go decode_pipeline`
  - `90.7 ms` → about `86.1 ms`
  - improved, though not all of that should be over-attributed to the Go-target helper pass alone

Interpretation:
- the first narrow fast-path pass did not help much
- a broader “add it everywhere practical” pass did move the main targets in the right direction
- this supports the typed-extern ABI direction for future userland FFI work, even though it does not close the backend/native gap on its own

Follow-up recommendation:
- keep the broad `vm_next` and Go-target changes as a reasonable foundation
- do not overclaim the perf impact yet
- defer deeper work until there is a reason to continue, likely around userland FFI or more focused backend perf work

## Notes

- Current repo benchmarks are sparse in `go test`, but the `compiler/benchmarks/` suite gives a much better macro signal for this experiment.
- The strongest first implementation target was `vm_next`, not the legacy bytecode VM.
- The experiment result is encouraging enough to keep, but not urgent enough to continue immediately.
