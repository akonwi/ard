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

Status: Pending

This is the first major step. The goal is not to optimize everything at once.
The goal is to introduce a real execution artifact and feedback loop:

```text
AIR -> compiler/vm_next/bytecode -> vm_next bytecode interpreter
```

The current tree-walk interpreter should remain available temporarily as a
fallback/comparison while bytecode coverage reaches parity. The initial success
condition is behavioral parity, not immediate benchmark wins at any cost. Once
parity is established, the bytecode path becomes the foundation for targeted
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

- [ ] Define a compact `vm_next` bytecode program representation in nested
  package `compiler/vm_next/bytecode`.
  - [ ] Function chunks with local counts, parameter counts, return type, and
    instruction stream.
  - [ ] Constant pool or immediate encoding for scalar/string constants.
  - [ ] Pre-resolved function, extern, type, field, trait, and impl IDs.
  - [ ] Explicit instruction operands for local slots, field indexes, jump
    offsets, arities, and type IDs.
- [ ] Add `AIR -> vm_next bytecode` lowering.
  - [ ] Lower expressions into stack or register operations.
  - [ ] Lower blocks and control flow into jumps.
  - [ ] Lower function calls, closure calls, extern calls, and trait calls.
  - [ ] Lower list/map/string operations used by current benchmark programs.
  - [ ] Lower `Maybe`, `Result`, `try`, match, union, and enum operations.
- [ ] Add a bytecode verifier or validator for backend-facing invariants.
  - [ ] Valid instruction operands and jump targets.
  - [ ] Valid local/function/extern/type references.
  - [ ] Arity checks for calls and extern calls.
  - [ ] Field indexes and match tags are valid.
- [ ] Add a bytecode interpreter loop.
  - [ ] Execute entry and script roots.
  - [ ] Preserve panic/diagnostic behavior expected by parity tests.
  - [ ] Keep the current tree-walk interpreter available behind an internal
    fallback or test path until bytecode parity is complete.
- [ ] Integrate bytecode execution into `ard run --target vm_next` and embedded
  `ard build --target vm_next` executables.
- [ ] Run current `vm_next` parity tests against the bytecode path.
- [ ] Run benchmark suite and record before/after numbers in this document.

### Milestone 2: Remove obvious interpreter allocation overhead

Status: Pending

Once the bytecode path exists, reduce allocation and dispatch overhead in the new
execution loop.

- [ ] Avoid per-expression/per-statement AIR object copying in the hot path.
- [ ] Avoid generic `[]Value` allocation for common call arities.
  - [ ] Direct call fast paths for 0/1/2/3 args.
  - [ ] Closure call fast paths for 0/1/2 args.
  - [ ] Extern call fast paths for common arities.
- [ ] Introduce reusable frame/local storage.
  - [ ] Reuse frame objects or use a contiguous call stack.
  - [ ] Avoid allocating a new locals slice on every call.
  - [ ] Preserve safe behavior for recursion and fibers.
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

### Initial notes

- PR #101 established functional parity and executable build support for
  `vm_next`.
- The PR benchmark table showed `vm_next` executables roughly `1.5x` to `5.0x`
  slower than current bytecode VM executables depending on workload.
- A first profiling pass after adding `ARD_VM_NEXT_PROFILE=1` indicated that
  pure-runtime overhead dominates several benchmarks, while FFI reflection is a
  major but narrower issue for decode/sql workloads.
