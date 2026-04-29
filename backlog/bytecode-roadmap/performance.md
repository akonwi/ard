# Bytecode VM Performance Backlog

This document captures near-term performance work for the bytecode VM and the VM-side FFI boundary.

It is intentionally focused on practical profiling-led work, especially for decode-heavy programs like `benchmarks/programs/decode_pipeline.ard`.

## Current observations

- Decode-heavy workloads spend a large amount of time crossing the extern boundary.
- The current VM extern path still pays for:
  - extern lookup by binding string
  - `runtime.Object` argument/result wrapping at the boundary
  - allocation-heavy Dynamic/list/map materialization in decode helpers
- Decoder combinators also exercise closure creation/calls heavily.
- Handwritten native Go remains much faster than both the VM and generated Go on `decode_pipeline`, so there is still significant runtime overhead to remove.

## Prioritized work

### 1. Make extern dispatch cheaper

This is the highest-priority VM performance item.

Candidate directions:

- replace steady-state binding lookup by string with a cheaper call path
  - pre-resolved numeric extern IDs
  - direct function pointer tables attached to emitted programs
  - or another constant-time path that avoids repeated string-key map work
- remove unnecessary synchronization from the hot call path if registration is already complete before execution
- reduce per-call temporary allocations around argument passing
- investigate whether some or all VM extern calls can avoid `runtime.Object` boxing at the boundary
  - especially for scalar arguments/results and Dynamic-heavy decode helpers

### 2. Avoid materializing Ard collections when decode only needs traversal

Decode currently converts raw JSON-backed Go values into Ard lists/maps eagerly.

Candidate directions:

- avoid building full Ard `[Dynamic]` / `[Dynamic:Dynamic]` values when a decoder only needs to iterate through raw `[]any` / `map[string]any`
- add VM/runtime support for cheaper iteration over raw decoded data
- measure how much time and allocation volume comes specifically from:
  - `DynamicToList`
  - `DynamicToMap`
  - repeated `MakeDynamic` / list / map construction

### 3. Optimize closure-heavy decode paths

Decoder combinators rely on closures and nested small function calls.

Candidate directions:

- reduce allocation overhead in `OpCallClosure`
- reuse scratch argument slices for closure invocation the same way module/extern calls already do
- measure closure creation count, closure call count, and average closure arity on benchmark programs

### 4. Keep decode optimization general-purpose

For now, avoid a fused/special-purpose decoder compiler path unless profiling proves it is necessary.

Preferred approach:

- first make generic VM execution and generic extern calls cheaper
- prefer improvements that help all programs, not only `ard/decode`
- only revisit decode-specific lowering/intrinsics if general VM/FFI work does not close enough of the gap

## Instrumentation backlog

Before making larger changes, collect repeatable data.

Add and use instrumentation for:

- total extern calls
- per-binding extern call counts and cumulative time
- closure creation/call counts
- direct function call vs closure call mix
- `runtime.Object` constructor/allocation counters
- Dynamic/list/map construction counts during VM execution

The instrumentation should:

- be opt-in
- avoid changing normal stdout program output
- be usable from both `ard run` and embedded bytecode binaries built via `ard build`
- print to stderr or another non-program-output channel

## Initial experiments to run

1. Profile `decode_pipeline` with VM instrumentation enabled.
2. Record:
   - total extern calls
   - hottest extern bindings by call count and time
   - object allocation counts by constructor kind
   - closure creation/call counts
3. Compare those numbers against benchmark wall-clock timings.
4. Use the results to choose the first implementation target:
   - cheaper extern dispatch
   - reduced `runtime.Object` boxing
   - cheaper Dynamic collection traversal
   - cheaper closure invocation

## Instrumentation now available

Current VM profiling is implemented and opt-in.

Enable it with:

```bash
ARD_VM_PROFILE=1
```

Optional tuning:

```bash
ARD_VM_PROFILE_TOP=10
```

Behavior:

- works for `ard run` on the bytecode target
- works for embedded bytecode binaries produced by `ard build`
- writes profiling output to `stderr`
- keeps normal program `stdout` unchanged

Current reports include:

- total direct/module/closure/extern call counts
- closure creation stats
- per-binding extern call counts and cumulative time
- `runtime.Object` constructor and fresh-allocation counters

## Initial data points: `decode_pipeline`

Runtime benchmark snapshot on this branch:

- VM: ~240 ms
- Ard Go backend: ~407 ms
- native Go: ~22 ms
- JS: ~78 ms

A profiled VM run of `benchmarks/programs/decode_pipeline.ard` produced:

### Call counts

- direct calls: `132,012`
- module calls: `456,038`
- closure calls: `456,038`
- extern calls: `468,040`

### Closure profile

- closures created: `120,010`
- average captures per closure: `1.00`
- max captures: `2`
- average closure call arity: `1.00`

### Extern totals

- timed inside `vm.ffi.Call(...)`: `58.755 ms`
- distinct extern bindings: `7`

Top bindings by total measured time:

1. `JsonToDynamic` — `12,001` calls — `26.503 ms`
2. `DecodeInt` — `336,028` calls — `12.366 ms`
3. `DynamicToList` — `24,002` calls — `9.356 ms`
4. `DynamicToMap` — `12,001` calls — `5.101 ms`
5. `DecodeString` — `48,004` calls — `3.035 ms`
6. `ExtractField` — `36,003` calls — `2.346 ms`

### `runtime.Object` profile

- constructor calls: `4,896,415`
- fresh allocations: `1,223,850`
- `make_dynamic`: `432,036`
- `make_list`: `48,004`
- `make_map`: `24,002`
- `copy_calls`: `72,006`
- `make_int`: `2,496,211` with `2,292,448` small-int cache hits
- `make_str`: `168,016`
- `make_bool`: `1,092,092`

## What the first data says

The initial profile supports a few conclusions:

1. Extern call volume is very high, so cheaper extern dispatch is still a strong first target.
2. Extern body time alone does not explain the full VM/native gap, so dispatch-only work will not be sufficient by itself.
3. `runtime.Object` churn is substantial, which supports investigating ways to reduce boxing/unboxing and Dynamic wrapper creation at the FFI boundary.
4. Eager Dynamic list/map materialization is visible in both time and allocation counts.
5. Closure-heavy decode composition is also a meaningful part of the hot path.

## Recommended implementation order

Based on the current profile, the recommended order is:

1. make extern dispatch cheaper
2. reduce `runtime.Object` boxing/materialization on decode-heavy paths
3. optimize closure invocation
4. avoid eager Dynamic list/map conversion where possible

## Concrete next step: redesign extern dispatch

The first implementation step should target the generic VM extern call path without changing Ard semantics.

### Goals

- remove repeated binding-string lookup from steady-state execution
- remove synchronization from steady-state extern calls
- preserve current FFI registration, generator output, and panic/`Result` behavior
- keep profiling support intact so before/after comparisons stay easy

### Current hot path

Today `OpCallExtern` does all of the following during execution:

1. load a string constant for the binding name
2. build/pop the argument slice
3. call `RuntimeFFIRegistry.Call(binding, args, returnType)`
4. inside the registry, do a map lookup under `RWMutex`
5. invoke the function

That means every extern call pays for string-based dispatch even though the target binding is already known at emit time.

### Proposed phase 1: pre-resolved extern table

Add an extern table to bytecode programs and resolve it once when constructing the VM.

#### Bytecode/program changes

Introduce a program-level extern table, for example:

```go
type ExternEntry struct {
    ID      int
    Binding string
}

type Program struct {
    Constants []Constant
    Types     []TypeEntry
    Externs   []ExternEntry
    Functions []Function
}
```

Emitter changes:

- add `Emitter.addExtern(binding string) int`
- deduplicate bindings at program-build time
- change `OpCallExtern.A` to store an extern ID instead of a string constant index
- keep binding strings in the program only for debugging/profiling/reporting

#### VM changes

At `vm.New(program)` time:

- resolve `program.Externs[i].Binding` once against the shared FFI registry
- store a VM-local resolved table such as:

```go
type resolvedExtern struct {
    Binding string
    Func    FFIFunc
}
```

Then `OpCallExtern` becomes:

1. read extern ID from the instruction
2. fetch the resolved function pointer from a slice
3. call it directly
4. apply existing panic/`Result` handling semantics

This removes repeated string-constant loading and repeated map lookup from the hot path.

#### Registry changes

Keep the existing registration API, but treat the registry as immutable after startup.

Candidate simplification:

- register all builtins once during process init / lazy singleton creation
- after registration, use plain read-only structures on the call path
- remove `RWMutex` from steady-state extern dispatch if nothing mutates after startup

Even if the public `Register(...)` API remains, execution should not pay synchronization costs after VM construction.

### Proposed phase 1.5: keep compatibility while simplifying the call surface

The current `RuntimeFFIRegistry.Call(...)` helper mostly exists to:

- look up a binding name
- wrap panic recovery with `Result` awareness

After extern resolution moves to VM construction time, the VM can own the panic-recovery wrapper directly around the already-resolved function pointer.

That would let the steady-state call path avoid the registry abstraction almost entirely.

### Proposed phase 2: reduce `runtime.Object` boundary overhead

Once dispatch itself is cheaper, the next likely win is reducing object boxing/unboxing for extern-heavy paths.

Likely direction:

- keep existing raw `FFIFunc` support for complex bindings
- add an optional faster extern ABI for simple scalar/Dynamic-heavy bindings
- generate wrappers that can operate on cheaper raw values where safe
- focus first on decode-oriented helpers like:
  - `DecodeInt`
  - `DecodeString`
  - `DynamicToList`
  - `DynamicToMap`
  - `ExtractField`

The goal is not to special-case `ard/decode` in the compiler, but to make the generic extern boundary cheaper for bindings that do not need full `runtime.Object` semantics.

### Validation plan for phase 1

After implementing pre-resolved extern dispatch:

1. re-run `decode_pipeline` benchmark
2. re-run VM profiling on `decode_pipeline`
3. compare:
   - total wall time
   - total extern timed section
   - hottest binding totals
   - object allocation counts
4. confirm no output/behavior changes across VM tests and backend parity tests

Success criteria for phase 1:

- measurable runtime improvement on `decode_pipeline`
- lower total time attributed to extern calls
- no semantic change in existing test coverage

### Status update: pre-resolved extern table implemented

This phase has now been implemented.

What changed:

- bytecode programs now carry a deduplicated extern table
- `OpCallExtern` now references an extern ID instead of a string constant index
- the VM resolves extern bindings once at VM construction time
- steady-state extern execution now calls the pre-resolved function pointer directly
- pseudo-externs like `NewList`, `AsyncStart`, and `AsyncEval` remain lowered to VM ops instead of going through the FFI registry

### Observed impact on `decode_pipeline`

Profile comparison on a profiled VM run:

- extern total before: `58.755 ms`
- extern total after: `50.188 ms`
- improvement in measured extern section: about `8.6 ms` (~15%)

Selected per-binding improvements:

- `DecodeInt`: `12.366 ms` -> `9.334 ms`
- `DynamicToList`: `9.356 ms` -> `7.906 ms`
- `DynamicToMap`: `5.101 ms` -> `4.591 ms`
- `ExtractField`: `2.346 ms` -> `1.632 ms`

Other observations:

- total call counts and object-allocation counts stayed effectively the same
- runtime benchmark improvement was small and within normal benchmark noise
- this matches the profile: the extern dispatch section got cheaper, but `runtime.Object` churn and closure-heavy decode composition still dominate a large part of total execution time

### Updated takeaway

The pre-resolved extern table is still a good change because it removes avoidable dispatch work and simplifies future optimization.

But the profile now makes it clearer that the next higher-leverage work is:

1. reducing `runtime.Object` boxing/materialization across the extern boundary
2. reducing eager Dynamic list/map construction
3. reducing closure-call overhead in decode-heavy code

## Non-goals for now

- Do not introduce a fused decoder compiler path yet.
- Do not add decode-only special cases until the generic VM/FFI data says they are needed.
