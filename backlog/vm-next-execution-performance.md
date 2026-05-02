# vm_next Execution Performance Backlog

This document tracks runtime execution performance work for `vm_next` after the
AIR + `vm_next` parity and executable-build milestones.

The focus here is the execution path after a program has already been built or
loaded. Build-time work such as parsing, checking, AIR lowering, serialization,
and executable embedding is intentionally out of scope except where it affects
runtime artifact shape.

## Goal

Make `vm_next` competitive with, and eventually faster than, the current bytecode
VM while preserving the new AIR-centered architecture and signature-driven FFI
model.

The current implementation has proven semantics and parity, but execution still
pays costs from directly interpreting structured AIR, boxed wrapper values,
per-call allocation, and reflective FFI adapters. This backlog should be updated
as each optimization is implemented and benchmarked.

## Current baseline observations

The first profiling pass shows that the largest execution issue is not one single
stdlib host capability. Most pure-runtime benchmarks spend nearly all time inside
VM execution rather than FFI:

| Benchmark | vm_next profile signal |
|---|---|
| `sales_pipeline` | ~80k direct calls, heavy locals/fields/arithmetic, negligible FFI |
| `shape_catalog` | ~100k direct calls, unions/maps/maybe helpers, negligible FFI |
| `word_frequency_batch` | list iteration/push/at/size and loop overhead dominate |
| `decode_pipeline` | very high closure, `try_result`, and extern volume |
| `fs_batch` | host FS time is significant, but VM/string/result overhead still dominates wall time |
| `sql_batch` | FFI is meaningful, but closure/result/decode overhead also matters |

The main architectural observation is:

> `vm_next` currently executes structured AIR directly. A real `AIR -> vm_next`
> bytecode/lowered instruction layer should be the first major execution
> optimization foundation.

## Guiding principles

- Keep AIR target-neutral. `vm_next` bytecode is an implementation artifact, not
  the Ard language ABI.
- Preserve the existing AIR validator as the semantic/invariant gate; runtime can
  then use trusted fast paths where appropriate.
- Optimize general VM execution before introducing workload-specific fusions.
- Keep profiling opt-in and comparable across iterations.
- Prefer incremental parity-preserving steps with benchmark data after each one.

## Milestones

Status markers:

- `Done`: checklist complete and validated.
- `In progress`: at least one item complete.
- `Pending`: no committed implementation work yet.

### Milestone 1: Lower AIR to vm_next bytecode

Status: Done

This is the first major step. The goal is not to optimize everything at once.
The goal is to introduce a real execution artifact and feedback loop:

```text
AIR -> compiler/vm_next/bytecode -> vm_next bytecode interpreter
```

The current tree-walk interpreter was kept temporarily as a fallback/comparison
while bytecode coverage reached parity. After parity passed, the tree-walk
implementation was removed and `vm_next` now executes through bytecode by
default. The initial success condition was behavioral parity, not immediate
benchmark wins at any cost. The bytecode path is now the foundation for targeted
runtime optimizations.

Feedback loop for this milestone:

1. Build small vertical slices: scalars/locals/arithmetic first, then control
   flow, calls, data types, FFI, closures, traits/unions, and fibers.
2. For each slice, compare bytecode execution against the current AIR tree-walk
   execution where practical.
3. Run focused `vm_next` tests, then broaden to parity tests and eventually
   `go test ./...`.
4. Use `ARD_VM_NEXT_PROFILE=1` and later opcode counters to identify hot
   instructions after the bytecode path is running.
5. Record benchmark checkpoints in this document after major slices.

- [x] Define an initial compact `vm_next` bytecode program representation in
  nested package `compiler/vm_next/bytecode`.
  - [x] Function chunks with local counts, parameter counts, return type, and
    instruction stream.
  - [x] Constant pool or immediate encoding for scalar/string constants.
  - [x] Pre-resolved function, extern, type, field, trait, and impl IDs for the
    supported instruction set.
  - [x] Explicit instruction operands for local slots, field indexes, jump
    offsets, arities, and type IDs.
- [x] Add `AIR -> vm_next bytecode` lowering.
  - [x] Lower scalar expressions into stack operations for the initial slice.
  - [x] Lower blocks and `if` control flow into jumps for the initial slice.
  - [x] Lower direct function calls for the initial slice.
  - [x] Lower extern and trait calls for the initial print/Encodable slice.
  - [x] Lower closure calls.
  - [x] Lower initial list/map/string operations used by simple loops and
    collection tests.
  - [x] Lower initial `Maybe`, `Result`, `try`, enum match, int match, and
    union match operations.
  - [x] Lower remaining match shapes covered by current `vm_next` parity tests.
- [x] Add an initial bytecode verifier for backend-facing invariants.
  - [x] Valid instruction operands and jump targets.
  - [x] Valid local/function/extern/type references.
  - [x] Arity checks for calls and extern calls.
  - [x] Field indexes are validated for field opcodes.
  - [x] Match tags are lowered into validated scalar comparisons for union
    matching.
- [x] Add a bytecode interpreter loop.
  - [x] Execute entry and script roots.
  - [x] Preserve panic/diagnostic behavior expected by parity tests.
  - [x] Remove the old tree-walk interpreter after bytecode parity passed.
- [x] Integrate bytecode execution into `ard run --target vm_next` and embedded
  `ard build --target vm_next` executables.
- [x] Run current `vm_next` parity tests against the bytecode path.
- [x] Run benchmark suite and record before/after numbers in this document.

### Milestone 2: Remove obvious interpreter allocation overhead

Status: In progress

Once the bytecode path exists, reduce allocation and dispatch overhead in the new
execution loop.

Preparation notes after Milestone 1:

- The old per-expression/per-statement AIR tree-walk cost is gone.
- Bytecode profiles now report direct/closure/trait/extern calls, frames,
  opcode counts, and allocation-site counters for locals, stacks, and temporary
  arg slices.
- `decode_pipeline` and `sql_batch` still spend substantial time in reflective
  FFI conversion, but pure-runtime workloads (`sales_pipeline`,
  `word_frequency_batch`, `shape_catalog`) now point at bytecode interpreter
  overhead and allocation as the next general-purpose target.
- Hot allocation candidates in the bytecode interpreter:
  - per-call `locals := make([]Value, fn.Locals)`
  - per-call `stack := make([]Value, 0, len(fn.Code))`
  - per-call/per-method `popArgs(...)` and `popMethodArgs(...)` slices
  - trait calls building `argsWithReceiver`
  - closure callbacks such as `[]Value{maybeValue.Value}` and sort comparator
    `[]Value{left, right}`
  - zero-capture closure construction still allocating empty capture slices
  - FFI `[]reflect.Value` input slices, which will be handled more fully in
    Milestone 4 but can get small-arity fast paths here

Recommended Milestone 2 feedback loop:

1. Add bytecode execution counters and allocation-oriented microbenchmarks before
   changing data structures.
2. Remove easy `[]Value` allocations for common arities.
3. Reuse frame/local/stack storage or introduce an explicit bytecode call stack.
4. Rerun `go test ./...` and runtime benchmarks after each step.
5. Record benchmark and profile deltas in this document.

- [x] Avoid per-expression/per-statement AIR object copying in the hot path.
  - Completed by Milestone 1 when default execution moved to bytecode.
- [x] Add bytecode-specific instrumentation needed to guide this milestone.
  - [x] Direct function, closure, trait, and extern call counts from bytecode.
  - [x] Opcode counters gated by `ARD_VM_NEXT_PROFILE=1`.
  - [x] Allocation counters for frames, locals slices, stacks, and arg slices.
- [ ] Avoid generic `[]Value` allocation for common call arities.
  - [x] Direct call path reads args from the caller stack instead of allocating
    a temporary args slice.
  - [x] Closure bytecode call path reads args from the caller stack instead of
    allocating a temporary args slice.
  - [x] Extern call path passes a caller-stack arg window instead of allocating
    a temporary `[]Value`.
  - [x] Method-op fast paths avoid `popMethodArgs` slices for list, map,
    string, Maybe, and Result operations.
  - [x] Closure call fast path for unary Maybe/Result mapper callbacks.
  - [ ] Direct function/closure locals fast paths for 0/1/2/3 args.
  - [ ] Extern reflection input fast paths beyond the current small fixed array.
- [ ] Introduce reusable frame/local storage.
  - [x] Reuse locals and operand stack slices through a VM-local `sync.Pool`.
  - [x] Avoid allocating a fresh locals slice on every call when a pooled slice
    is available.
  - [x] Avoid allocating a fresh stack slice on every call when a pooled slice is
    available.
  - [x] Preserve safe behavior for recursion and fibers with separate borrowed
    slices per active frame and pool return on frame exit.
  - [x] Replace recursive direct/closure/trait bytecode calls with an explicit
    frame loop and shared operand stack per VM invocation.
  - [x] Reduce operand stack allocations from per-frame to per invocation for
    in-VM calls; fibers and host callback closures still get separate safe
    invocations.
  - [ ] Consider a contiguous locals arena if profiles still show locals copy or
    pool overhead after frame-loop dispatch.
- [ ] Remove redundant runtime validation from trusted hot paths after bytecode
  validation succeeds.
- [ ] Benchmark pure-runtime programs after each step.

### Milestone 3: Value representation improvements

Status: Pending

The current `Value` representation boxes many common Ard values behind `Ref any`.
This is simple but allocation-heavy and type-assertion-heavy.

- [ ] Measure allocation counts for `vm_next` execution with Go allocation
  profiling or additional counters.
- [ ] Inline or specialize common wrappers.
  - [ ] `Maybe` fast representation.
  - [ ] `Result` fast representation.
  - [ ] Small/void result cases without heap allocation.
- [ ] Reduce `Ref any` usage on hot paths.
  - [ ] Prefer typed heap references or tagged heap records.
  - [ ] Avoid repeated Go type assertions for validated value kinds.
- [ ] Add cheaper zero-value handling.
  - [ ] Cache common zero values by `TypeID` where safe.
  - [ ] Avoid repeatedly scanning type tables for `Void`.
- [ ] Benchmark decode/result-heavy and map/maybe-heavy workloads.

### Milestone 4: Direct generated FFI adapters

Status: Pending

The current `vm_next` FFI path validates signatures at VM construction, but each
extern call still uses reflection for argument conversion, invocation, and return
conversion. This is especially visible in decode and SQL workloads.

- [ ] Generate or register direct typed adapter functions keyed by `ExternID`.
- [ ] Avoid `reflect.Value` creation and `reflect.Call` in steady-state extern
  calls.
- [ ] Generate scalar adapters for `Int`, `Float`, `Bool`, `Str`, `Void`.
- [ ] Generate direct adapters for `Maybe[T]` and error-backed `Result[T,E]`.
- [ ] Generate direct adapters for stdlib decode/json/sql hot externs.
- [ ] Keep panic recovery and `Result` error wrapping semantics intact.
- [ ] Preserve callback handle behavior for VM-to-host callback APIs.
- [ ] Benchmark `decode_pipeline`, `sql_batch`, `fs_batch`, and HTTP parity
  coverage.

### Milestone 5: Collection and iteration fast paths

Status: Pending

Current list/map operations are expressed as generic method-like AIR operations,
and map storage is a linear entry list.

- [ ] Add explicit bytecode instructions or lowered operations for list
  iteration.
  - [ ] Avoid repeated `list.size()`/`list.at()` patterns for `for item in list`.
  - [ ] Support index-producing list loops without extra method overhead.
- [ ] Add explicit map iteration support.
  - [ ] Avoid sorting/copying entries more often than necessary.
  - [ ] Preserve deterministic iteration order.
- [ ] Evaluate real hash-map storage for primitive Ard key types.
  - [ ] `Str`, `Int`, `Bool`, and enum keys.
  - [ ] Deterministic sorted-key cache with dirty flag for iteration.
  - [ ] Fallback representation for unsupported key shapes if needed.
- [ ] Add list growth/capacity improvements for common push-heavy patterns.
- [ ] Benchmark `sales_pipeline`, `shape_catalog`, and
  `word_frequency_batch`.

### Milestone 6: Closure, trait, and async call fast paths

Status: Pending

Closure and trait dispatch are important for decode helpers, sorting callbacks,
trait-based encoding, and host callback APIs.

- [ ] Reduce closure creation allocation.
  - [ ] Avoid capture slice allocation for zero-capture closures.
  - [ ] Reuse or compact capture storage where safe.
- [ ] Add closure call fast paths for common arities.
- [ ] Make trait calls cheaper after bytecode validation.
  - [ ] Pre-resolve impl method function IDs in bytecode operands.
  - [ ] Avoid repeated trait/impl bounds checks in hot dispatch.
- [ ] Review sort comparator calls and other high-frequency callback paths.
- [ ] Keep fiber spawn/get/wait profiling and safety behavior intact.
- [ ] Benchmark `decode_pipeline`, `sql_batch`, `async_batches`, and sort-heavy
  samples.

### Milestone 7: Profiling and benchmark tracking

Status: Pending

Keep this backlog profiling-led so the optimization order can change as data
changes.

- [ ] Keep `ARD_VM_NEXT_PROFILE=1` reports working on bytecode execution.
- [ ] Add allocation-oriented profiling for `vm_next` values and frames.
- [ ] Add optional opcode/instruction counters once bytecode exists.
- [ ] Record benchmark snapshots after each milestone.
- [ ] Track both wall-clock and profile deltas.
- [ ] Compare regularly against:
  - [ ] current bytecode VM executable
  - [ ] generated Go target
  - [ ] native Go benchmark programs

## Recommended initial implementation sequence

1. Implement Milestone 1 with minimal optimization: bytecode shape, lowering,
   verifier, interpreter loop, and parity.
2. Use opcode/profile data from the bytecode path to pick the first hot-path
   implementation target.
3. Likely first follow-up: remove `[]Value` argument allocation and frame/local
   allocation.
4. Then attack FFI reflection for decode/sql workloads.
5. Then redesign `Maybe`/`Result` representation if allocation profiles confirm
   wrapper churn is still a dominant bottleneck.

## Benchmark snapshot log

Add dated entries here as work lands.

### Milestone 1 completion snapshot

Validation:

- `cd compiler && go test ./...`
- `cd compiler && ./benchmarks/run.sh --mode runtime --runs 1 --warmup 0 ...`

Runtime benchmark snapshot after switching default `vm_next` construction to the
new bytecode path. These are single-run numbers, so use them as directional data,
not statistical proof:

| Benchmark | bytecode VM | vm_next bytecode | vm_next delta |
|---|---:|---:|---:|
| `sales_pipeline` | 62.4 ms | 100.1 ms | 1.6x slower |
| `shape_catalog` | 75.4 ms | 137.3 ms | 1.8x slower |
| `decode_pipeline` | 233.9 ms | 871.4 ms | 3.7x slower |
| `word_frequency_batch` | 48.3 ms | 101.4 ms | 2.1x slower |
| `async_batches` | 14.3 ms | 12.1 ms | 1.2x faster |
| `fs_batch` | 105.3 ms | 500.4 ms | 4.8x slower |
| `sql_batch` | 44.7 ms | 89.7 ms | 2.0x slower |

Compared with the PR #101 baseline, the bytecode layer substantially improves
pure runtime and SQL/decode workloads, while FS remains dominated by remaining
VM/string/result/FFI overhead around host filesystem calls. Milestone 2 should
now focus on allocation and argument/frame overhead in the bytecode interpreter.

### Milestone 2 first checkpoint snapshot

Validation:

- `cd compiler && go test ./vm_next`
- `cd compiler && go test ./...`
- `cd compiler && ./benchmarks/run.sh --mode runtime --runs 1 --warmup 0`

Changes in this checkpoint:

- Added bytecode opcode/call/frame/allocation-site counters to
  `ARD_VM_NEXT_PROFILE=1`.
- Removed temporary `[]Value` argument slices for direct calls, closure calls,
  extern calls, and list/map/string/Maybe/Result method ops by using caller
  stack windows.
- Added a unary closure fast path for Maybe/Result mapper callbacks.
- Used a small fixed reflection input array for <=3-arity extern calls.
- Reused locals and operand stack slices through a VM-local `sync.Pool`.

Directional runtime benchmark snapshot:

| Benchmark | bytecode VM | vm_next bytecode | vm_next delta |
|---|---:|---:|---:|
| `sales_pipeline` | 62.5 ms | 83.3 ms | 1.3x slower |
| `shape_catalog` | 80.0 ms | 102.4 ms | 1.3x slower |
| `decode_pipeline` | 231.6 ms | 743.8 ms | 3.2x slower |
| `word_frequency_batch` | 47.6 ms | 76.3 ms | 1.6x slower |
| `async_batches` | 13.4 ms | 12.1 ms | 1.1x faster |
| `fs_batch` | 103.3 ms | 517.6 ms | 5.0x slower |
| `sql_batch` | 43.3 ms | 71.8 ms | 1.7x slower |

Profile highlights after temporary arg-slice removal:

- `sales_pipeline`: temporary arg slices fell to 16; frame slice requests are
  now served through pooling after warm-up.
- `word_frequency_batch`: temporary arg slices fell to 20; remaining dominant
  opcode pattern is list iteration via repeated `ListSize`/`ListAt`.
- `decode_pipeline`: temporary arg slices fell to 72,007, with reflective FFI and
  high closure/frame traffic still dominant.
- `sql_batch`: temporary arg slices fell to 5; reflective FFI and Result-heavy
  control flow remain visible.

Next Milestone 2 target: decide whether to continue with contiguous frame/call
stack work now, or move to FFI/value representation because pure temporary
argument allocation is no longer dominant.

### Milestone 2 frame-loop checkpoint snapshot

Validation:

- `cd compiler && go test ./...`
- `cd compiler && ./benchmarks/run.sh --mode runtime --runs 1 --warmup 0`

Changes in this checkpoint:

- Replaced recursive bytecode direct/closure/trait calls with an explicit frame
  loop.
- Shared one operand stack per bytecode VM invocation, using frame `stackBase`
  boundaries to isolate callee temporaries and return values.
- Kept fibers and host callback closure calls as independent bytecode VM
  invocations for safety.
- Trait calls now push a bytecode frame directly instead of allocating a receiver
  args slice and recursively entering the VM.

Directional runtime benchmark snapshot:

| Benchmark | bytecode VM | vm_next bytecode | vm_next delta |
|---|---:|---:|---:|
| `sales_pipeline` | 62.3 ms | 78.5 ms | 1.3x slower |
| `shape_catalog` | 78.4 ms | 95.2 ms | 1.2x slower |
| `decode_pipeline` | 233.9 ms | 670.0 ms | 2.9x slower |
| `word_frequency_batch` | 48.1 ms | 84.7 ms | 1.8x slower |
| `async_batches` | 13.2 ms | 13.0 ms | parity |
| `fs_batch` | 102.8 ms | 515.7 ms | 5.0x slower |
| `sql_batch` | 45.0 ms | 68.1 ms | 1.5x slower |

Profile highlights after the explicit frame loop:

- `sales_pipeline`: stack allocation sites dropped from ~80k frame requests to
  16 invocation-level stack requests.
- `decode_pipeline`: stack allocation sites dropped from ~588k to 1 for the main
  invocation; reflective FFI and value conversion remain dominant.
- `sql_batch`: stack allocation sites dropped from ~50k to 1 for the main
  invocation.
- `word_frequency_batch`: worsened directionally despite few stack allocations;
  its remaining bottleneck is likely dispatch/list-iteration overhead rather
  than call-stack allocation.

Next Milestone 2 target: locals are still copied per call/frame. Consider a
locals arena or direct scalar/local fast paths only if profiles show further
call-heavy wins; otherwise move to list iteration, FFI, and Maybe/Result value
representation.

### Initial notes

- PR #101 established functional parity and executable build support for
  `vm_next`.
- The PR benchmark table showed `vm_next` executables roughly `1.5x` to `5.0x`
  slower than current bytecode VM executables depending on workload.
- A first profiling pass after adding `ARD_VM_NEXT_PROFILE=1` indicated that
  pure-runtime overhead dominates several benchmarks, while FFI reflection is a
  major but narrower issue for decode/sql workloads.
