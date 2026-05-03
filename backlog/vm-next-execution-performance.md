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
  - [x] Direct function/closure locals fast paths for 0/1/2/3 args.
  - [x] Extern reflection input fast paths beyond the current small fixed array.
    - [x] Precompute scalar/dynamic host input conversion plans after signature
      validation so generic reflective extern calls avoid repeated type-shape
      dispatch before constructing `reflect.Value` inputs.
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
  - [x] Add a contiguous locals arena for nested frame-loop calls to avoid
    repeated pool get/put traffic for active direct, closure, and trait frames.
- [x] Merge autoresearch-proven local bytecode specializations into this
  milestone.
  - [x] Cache sorted map entries for deterministic map iteration and invalidate
    the cache on mutation, avoiding repeated sort/copy work for `key_at` /
    `value_at` loops.
  - [x] Add local map iteration opcodes for map size, index comparison,
    key-at, and value-at patterns.
  - [x] Add local struct field access opcode for `LoadLocal` + `GetField`
    patterns.
- [x] Pull forward limited direct FFI fast paths where they unblock runtime
  execution benchmarks.
  - [x] Add signature-based adapters for stdlib dynamic/decode externs.
  - [x] Add signature-based adapters for common string/bool/error filesystem
    and SQL-style externs.
  - [x] Precompute Result/value type metadata captured by fast FFI adapters.
  - [ ] Generalize into generated/direct adapter coverage in Milestone 4.
- [x] Remove redundant runtime validation from trusted hot paths after bytecode
  validation succeeds.
  - Closed as intentionally not pursued further for Milestone 2. Multiple
    profiling-led experiments showed that removing small defensive checks did
    not improve the full runtime suite and often made it slower, likely because
    the extra code/layout changes outweighed the tiny branch savings.
  - Evaluated trusted removal of `LoadLocal`/`StoreLocal` local bounds checks
    after the locals arena work; `go test ./...` passed, but the 10-run runtime
    suite regressed directionally (`vm_next` total about `805.0 ms` vs the
    `790.7 ms` locals-arena checkpoint), so the change was not kept.
  - Evaluated trusting verified direct-call target/arity operands in the frame
    loop; tests passed, but the 10-run runtime suite regressed more broadly, so
    the change was not kept.
  - Earlier autoresearch also rejected trusting extern arity and removing local
    bounds checks. Keep defensive validation unless a future profile identifies
    a specific check with measurable standalone cost.
- [ ] Benchmark pure-runtime programs after each step.

### Milestone 3: Value representation improvements

Status: In progress — Result representation complete; remaining Maybe,
`Ref any`, and zero-value work is profiling-gated.

The current `Value` representation boxes many common Ard values behind `Ref any`.
This is simple but allocation-heavy and type-assertion-heavy.

This milestone is now the primary follow-up for the remaining `decode_pipeline`
gap after Milestone 2. Profiling shows decode still executes hundreds of
thousands of `TryResult`, `Return`, closure, and frame operations. Fast FFI
adapters removed much of the reflective boundary cost, so the next decode wins
are likely in `Result`/`Maybe` representation, cheaper zero values, and reducing
wrapper allocation/type-assertion churn on success-heavy decode paths.

- [x] Measure allocation counts for `vm_next` execution with Go allocation
  profiling or additional counters.
  - Initial Milestone 3 profiling reused `ARD_VM_NEXT_PROFILE=1` counters after
    the Milestone 2 locals arena. `decode_pipeline` still reports very high
    wrapper/control-flow pressure: ~588k frames, ~120k closures created, ~492k
    `TryResult` opcodes, and ~468k fast extern Result calls in a representative
    profile sample.
  - Added opt-in value allocation counters under `ARD_VM_NEXT_PROFILE=1` to
    count constructed wrapper/container values by kind. Representative profile
    samples show where wrapper work concentrates:
    - `decode_pipeline`: ~1.13M value allocations counted, including ~924k
      `Result`, ~120k closure, ~60k list, ~12k map, and ~12k union values.
    - `sql_batch`: ~120k value allocations counted, including ~85k `Result`,
      ~20k union, ~5k struct, ~5k list, and ~5k map values.
    - `shape_catalog`: ~100k value allocations counted, split mostly between
      ~50k struct and ~50k union values.
    - `word_frequency_batch`: only ~15 counted value allocations in the initial
      broad wrapper counters; its remaining cost was interpreter loop/local/
      scalar overhead rather than Result wrapper churn.
  - After the scalar Result representation landed, added more targeted
    profiling counters for the remaining Milestone 3 questions. These detailed
    counters are built with `-tags vmnext_profile_detail` and reported when
    `ARD_VM_NEXT_PROFILE=1` is set, keeping the default runtime path focused on
    the existing lighter profile hooks:
    - `maybe profile`: `Some`/`None` construction and access counts.
    - `ref accesses`: helper-level typed `Ref any` access counts split by
      struct/list/map/Maybe/Result/union/trait/extern/dynamic/closure/fiber.
    - `zero values`: `zeroValue` calls split by AIR type category.
    Representative `ARD_VM_NEXT_PROFILE=1` samples now show that the remaining
    questions are workload-specific rather than obviously global:
    - `decode_pipeline`: `Maybe` is cold (`maybe=0`), while list/map/closure
      `Ref` access dominates; zero values are effectively absent.
    - `shape_catalog`: `Maybe` construction/access is hot (~100k, nearly all
      `Some`), with union/map/list/struct `Ref` access also high.
    - `sql_batch`: moderate `Maybe` (~20k all `Some`), Result (~85k), union,
      closure, and extern access remain visible; zero values are mostly `Void`.
    - `word_frequency_batch`: `Maybe` is now visible through map lookup
      profiling (~120k, almost all `Some`), but this should be evaluated with
      loop/collection overhead in Milestone 5 before changing global Maybe
      representation.
  - Evaluated a conservative immutable zero-value cache for scalar and nested
    immutable `Maybe`/`Result`/`Union` zero values. Tests passed, but the
    10-run runtime suite did not improve directionally, so the change was not
    kept. Zero-value caching should only be revisited with allocation evidence
    showing repeated zero construction as a standalone bottleneck.
- [ ] Inline or specialize common wrappers.
  - [ ] `Maybe` fast representation.
    - Profiling-gated. A broad Result-style `Maybe` representation was already
      rejected, and new counters show Maybe is hot in shape/sql/word-frequency
      paths but cold in decode. Prefer lower-risk access/lowering experiments
      over a global representation change unless profiles show a specific hot
      Maybe shape.
  - [x] Result fast representation.
    - Autoresearch retained a coherent Result redesign: inline success tag in
      `Value.Bool`, generic payload as typed `*Value`, dedicated internal
      scalar success kinds for `Int`, `Str`, `Bool`, and `Float`, and direct
      unboxing in hot `TryResult` plus local Result-match paths.
    - Best retained Milestone 3 Result autoresearch snapshot improved the
      session baseline from `total_ms=867.3 ms` to `775.8 ms` (`-10.5%`), with
      the largest win in `decode_pipeline`.
    - Rejected follow-ups include `Maybe` generalization, void/reference/list/
      dynamic Result kinds, switch dispatch, branch reshuffles, and pruning the
      float specialization.
  - [x] Evaluate helper-level `TryResult`/`TryMaybe` unboxing specialization.
    Inlining the try unbox path directly into the bytecode switch passed tests
    but regressed the 10-run runtime suite directionally, matching earlier
    autoresearch results. Do not pursue helper inlining as the representation
    strategy; focus on changing allocation/boxing shape if revisiting wrappers.
  - [x] Evaluate storing `Maybe`/`Result` payloads directly in `Value.Ref` with
    the tag in `Value.Bool` instead of allocating `MaybeValue`/`ResultValue`
    wrappers. Tests passed, but the 10-run runtime suite regressed strongly,
    especially decode. Interface-boxing a full `Value` payload and compatibility
    paths outweighed removing wrapper structs, so this representation is not a
    keeper.
  - [x] Evaluate small/void result cases without heap allocation. A narrow
    `Result[Void, E]` success representation using `Value.Bool` for the tag and
    `Value.Int` for the void payload type passed tests, but the 10-run suite did
    not improve and introduced compatibility overhead in result access. Do not
    keep ad hoc partial encodings unless paired with a broader tagged
    representation.
- [ ] Reduce `Ref any` usage on hot paths.
  - [ ] Prefer typed heap references or tagged heap records.
  - [x] Add profiling counters for typed `Ref any` access by helper kind so
    future storage changes can be driven by actual assertion/access volume.
  - [x] Add inlineable fast-ref helpers for hot list/map/struct/union/closure
    access paths while preserving detailed defensive error messages on the cold
    invalid path. This targets the `decode_pipeline` profile where list, map,
    and closure `Ref` access dominate remaining value-representation pressure.
  - [x] Avoid repeated stack traffic and value assertions for local match
    subjects by lowering Result and Union match probes/extractions to local
    bytecode opcodes (`ResultIsOkLocal`, `ResultExpectLocal`,
    `ResultErrValueLocal`, `UnionTagLocal`, and `UnionValueLocal`). This keeps
    defensive validation but removes repeated `LoadLocal` + wrapper op pairs in
    decode/shape/sql hot paths. A follow-up attempt to inline the Result-local
    helper directly into the interpreter switch passed tests but worsened the
    full 10-run suite, so the smaller helper-based form was retained.
  - [ ] Avoid repeated Go type assertions for validated value kinds.
- [ ] Add cheaper zero-value handling.
  - [x] Add profiling counters for `zeroValue` calls by AIR type category.
  - [ ] Cache common zero values by `TypeID` where safe.
  - [ ] Avoid repeatedly scanning type tables for `Void`.
- [x] Benchmark decode/result-heavy and map/maybe-heavy workloads after local
  match subject opcodes.

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

Status: In progress

Current list/map operations are expressed as generic method-like AIR operations,
and map storage is a linear entry list.

List-local iteration fast paths have landed, but collection-heavy workloads such
as `word_frequency_batch` still spend most remaining time in interpreter loop
mechanics: `LoadLocal`, `StoreLocal`, loop jumps, constants, and scalar integer
updates. Keep this milestone as the tracking home for remaining collection/loop
work so the pure-runtime gap does not get lost behind decode/FFI work.

- [x] Add explicit bytecode instructions or lowered operations for list
  iteration.
  - [x] Avoid repeated method-style `list.size()`/`list.at()` patterns for
    `for item in list` by lowering local-list size/index access to specialized
    bytecode opcodes.
  - [x] Support index-producing list loops without extra method overhead.
- [ ] Track and reduce remaining loop-control overhead in collection-heavy
  programs.
  - [ ] Re-profile `word_frequency_batch` after each runtime milestone and
    watch `LoadLocal`, `StoreLocal`, `ConstInt`, `IntAdd`, `Jump`, and
    `JumpIfFalse` counts.
  - [ ] Consider specialized loop/scalar opcodes only under a pure-runtime or
    collection-weighted objective; autoresearch rejected fused local increment
    under the aggregate suite despite some collection/decode wins.
  - [ ] Look for lower-risk lowering changes that reduce local load/store pairs
    without adding broad dispatch complexity. A `ReturnLocal` peephole for
    trailing `LoadLocal; Return` passed tests and reduced some decode opcode
    counts, but the full 10-run suite regressed directionally, so it was not
    kept.
- [ ] Add explicit map iteration support.
  - [ ] Avoid sorting/copying entries more often than necessary.
  - [ ] Preserve deterministic iteration order.
- [ ] Evaluate real hash-map storage for primitive Ard key types.
  - [ ] `Str`, `Int`, `Bool`, and enum keys.
  - [ ] Deterministic sorted-key cache with dirty flag for iteration.
  - [ ] Fallback representation for unsupported key shapes if needed.
- [ ] Add list growth/capacity improvements for common push-heavy patterns.
- [ ] Benchmark `sales_pipeline`, `shape_catalog`, and
  `word_frequency_batch` with at least `--runs 10 --warmup 3` for retained
  changes.

### Milestone 6: Closure, trait, and async call fast paths

Status: Pending

Closure and trait dispatch are important for decode helpers, sorting callbacks,
trait-based encoding, and host callback APIs.

This is the other major tracking area for the remaining `decode_pipeline` gap.
Current profiles show very high closure call volume and many captured closure
creations in decode combinators. Runtime representation tweaks alone may not be
enough; compiler/lowering-level closure reuse or hoisting may be needed to reduce
closure churn without adding branch-heavy `ClosureValue` access paths.

Added opt-in closure capture histogram counters under `ARD_VM_NEXT_PROFILE=1`.
Representative decode profiling after local match-subject opcodes shows
~120k closure creations split across ~48k zero-capture, ~24k one-capture, and
~48k two-capture closures. VM-level zero-capture closure caching removed those
counted zero-capture allocations but regressed the 10-run runtime suite. Both a
map-backed runtime cache and a lower-overhead bytecode-indexed zero-closure pool
passed tests but failed to improve pure/decode benchmark means, so only the
histogram was kept. Do not retry zero-capture closure caching unless the runtime
representation can avoid adding dispatch/cache checks to every zero-capture
closure expression.

- [ ] Reduce closure creation allocation.
  - [x] Evaluate capture distribution and zero-capture closure caching. Both
    map-backed and bytecode-indexed cache experiments passed tests but regressed
    the full benchmark suite, so only the profiling histogram was kept.
  - [ ] Avoid capture slice allocation for zero-capture closures only if a
    cheaper representation than runtime caching is available.
  - [x] Evaluate small-arity capture collection fast paths for `OpMakeClosure`.
    Special-casing 0/1/2 captures removed `popArgs` profile counts but did not
    remove captured-slice allocation, and the full 10-run suite did not improve;
    keep the simpler generic helper until representation changes remove the
    underlying allocation.
  - [ ] Reuse or compact capture storage where safe.
  - [ ] Investigate lowering-level closure hoisting/reuse for decoder
    combinators; autoresearch rejected a runtime-only unary capture inline
    representation because the added branching hurt the aggregate suite.
- [ ] Add closure call fast paths for common arities.
  - [ ] Pay special attention to unary closure calls from decode and
    `Maybe`/`Result` mapper paths.
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

### List iteration bytecode fast-path snapshot

Validation:

- `cd compiler && go test ./...`
- `cd compiler && ./benchmarks/run.sh --mode runtime --runs 1 --warmup 0`

Changes in this checkpoint:

- Added `ListSizeLocal`, `ListAtLocal`, and `ListIndexLtLocal` bytecode opcodes.
- Lowered `list.size()`, `list.at(index)`, and `index < list.size()` patterns
  against locals to specialized instructions.
- Preserved dynamic loop-size semantics: the list length is still checked each
  loop condition rather than cached once up front.

Directional runtime benchmark snapshot:

| Benchmark | bytecode VM | vm_next bytecode | vm_next delta |
|---|---:|---:|---:|
| `sales_pipeline` | 63.2 ms | 79.1 ms | 1.3x slower |
| `shape_catalog` | 78.9 ms | 96.2 ms | 1.2x slower |
| `decode_pipeline` | 239.8 ms | 652.1 ms | 2.7x slower |
| `word_frequency_batch` | 52.9 ms | 77.7 ms | 1.5x slower |
| `async_batches` | 13.3 ms | 12.8 ms | parity |
| `fs_batch` | 103.2 ms | 520.7 ms | 5.0x slower |
| `sql_batch` | 46.5 ms | 66.0 ms | 1.4x slower |

Profile highlights:

- `word_frequency_batch`: repeated list iteration opcodes changed from
  `ListSize`/`ListAt` method-style ops to `ListIndexLtLocal`/`ListAtLocal`; top
  opcode `LoadLocal` dropped by roughly another 120k in the profile sample.
- `decode_pipeline` and `sql_batch` still improve slightly because general local
  list-size/index patterns are also cheaper, but reflective FFI remains the
  dominant bottleneck.

Next target: either continue specializing hot loop/control-flow opcodes, or move
back to FFI and `Maybe`/`Result` value representation for decode/sql/fs.

### Milestone 2 autoresearch contribution snapshot

Validation:

- `cd compiler && go test ./...`
- Official autoresearch runtime harness (`./autoresearch.sh`) with output
  verification.

Kept autoresearch contributions merged into `refactor.vm-next`:

- Signature-based fast FFI adapters for stdlib dynamic/decode and
  string/bool/error extern signatures.
- Precomputed fast FFI Result/value type metadata.
- Cached sorted map entries for deterministic map iteration, invalidated on
  mutation.
- Local map iteration bytecode specialization (`MapSizeLocal`,
  `MapIndexLtLocal`, `MapKeyAtLocal`, `MapValueAtLocal`).
- Local struct field access bytecode specialization (`GetFieldLocal`).

Best official runtime-suite snapshot from autoresearch:

| Benchmark | vm_next bytecode |
|---|---:|
| `sales_pipeline` | 74.3 ms |
| `shape_catalog` | 88.3 ms |
| `decode_pipeline` | 391.6 ms |
| `word_frequency_batch` | 72.0 ms |
| `async_batches` | 11.7 ms |
| `fs_batch` | 100.3 ms |
| `sql_batch` | 52.8 ms |
| **total** | **791.0 ms** |

This reduced the autoresearch suite total from `1511.9 ms` to `791.0 ms`
(`-47.7%`) while preserving benchmark output verification. Several attempted
micro-optimizations were rejected by the full suite; see the autoresearch branch
history for discarded experiments.

### Milestone 2 locals fast-path checkpoint

Validation:

- `cd compiler && go test ./...`
- `cd compiler && ./benchmarks/run.sh --mode runtime --runs 1 --warmup 0`

Changes in this checkpoint:

- Added direct locals initialization fast paths for direct and closure bytecode
  frames with 0, 1, 2, or 3 arguments.
- Kept the generic `copy` fallback for higher arities and added an explicit
  locals-vs-arity guard before assignment.

Directional single-run runtime benchmark snapshot:

| Benchmark | vm_next bytecode |
|---|---:|
| `sales_pipeline` | 75.9 ms |
| `shape_catalog` | 92.5 ms |
| `decode_pipeline` | 409.1 ms |
| `word_frequency_batch` | 77.1 ms |
| `async_batches` | 13.0 ms |
| `fs_batch` | 104.6 ms |
| `sql_batch` | 54.6 ms |
| **total** | **826.8 ms** |

This is within expected single-run noise relative to the immediately prior
post-merge snapshot (`832.5 ms` total). Continue to treat very small interpreter
micro-optimizations cautiously and verify against full-suite output checks.

### Milestone 2 FFI input-plan checkpoint

Validation:

- `cd compiler && go test ./...`
- `cd compiler && ./benchmarks/run.sh --mode runtime --runs 10 --warmup 3`

Changes in this checkpoint:

- Added prevalidated host input conversion plans to generic extern adapters.
- Fast-planned common scalar parameters (`Int`/enum, `Float`, `Bool`, `Str`) and
  `Dynamic -> any` before falling back to the full generic `valueToHost` path.
- Kept the existing small fixed `reflect.Value` input array for low-arity extern
  calls.

10-run mean runtime benchmark snapshot:

| Benchmark | vm_next bytecode |
|---|---:|
| `sales_pipeline` | 75.8 ms |
| `shape_catalog` | 91.3 ms |
| `decode_pipeline` | 405.9 ms |
| `word_frequency_batch` | 75.4 ms |
| `async_batches` | 12.4 ms |
| `fs_batch` | 103.8 ms |
| `sql_batch` | 56.7 ms |
| **total** | **821.3 ms** |

This remains broadly in line with the locals fast-path checkpoint and keeps the
remaining reflective extern path simpler to profile before Milestone 4 expands
coverage with generated/direct adapters.

### Milestone 2 locals arena checkpoint

Validation:

- `cd compiler && go test ./...`
- `cd compiler && ./benchmarks/run.sh --mode runtime --runs 10 --warmup 3`

Changes in this checkpoint:

- Added an invocation-local contiguous locals arena for nested bytecode direct,
  closure, and trait frames.
- Kept the first frame, fibers, and host callback invocations isolated through
  the existing safe pooled locals path.
- Released arena locals in LIFO order as frames return, clearing slots before
  truncating the arena.

10-run mean runtime benchmark snapshot:

| Benchmark | vm_next bytecode |
|---|---:|
| `sales_pipeline` | 74.3 ms |
| `shape_catalog` | 91.2 ms |
| `decode_pipeline` | 376.5 ms |
| `word_frequency_batch` | 76.4 ms |
| `async_batches` | 13.0 ms |
| `fs_batch` | 106.3 ms |
| `sql_batch` | 53.0 ms |
| **total** | **790.7 ms** |

The biggest improvement in this run was `decode_pipeline`, consistent with its
very high direct/closure frame volume. This suggests frame-local pool traffic was
still meaningful after earlier argument and stack allocation work.

### Milestone 3 local match-subject checkpoint

Validation:

- `cd compiler && go test ./...`
- `cd compiler && ./benchmarks/run.sh --mode runtime --runs 10 --warmup 3`

Changes in this checkpoint:

- Added local bytecode opcodes for Result match tests/extractions:
  `ResultIsOkLocal`, `ResultExpectLocal`, and `ResultErrValueLocal`.
- Added local bytecode opcodes for Union match tests/extractions:
  `UnionTagLocal` and `UnionValueLocal`.
- Lowered Result and Union matches to use these local subject operations instead
  of repeated `LoadLocal` + generic wrapper operations.
- Preserved runtime validation and wrapper semantics; this is a lowering and
  dispatch optimization, not a representation change.

10-run mean runtime benchmark snapshot:

| Benchmark | vm_next bytecode |
|---|---:|
| `sales_pipeline` | 77.6 ms |
| `shape_catalog` | 91.1 ms |
| `decode_pipeline` | 381.5 ms |
| `word_frequency_batch` | 78.1 ms |
| `async_batches` | 12.9 ms |
| `fs_batch` | 104.2 ms |
| `sql_batch` | 55.0 ms |
| **total** | **800.4 ms** |

The local match opcodes reduced profiled `decode_pipeline` `LoadLocal` count from
~5.10M to ~4.31M and improved the 10-run `decode_pipeline`/`shape_catalog`
means relative to the value-allocation profiling checkpoint. The aggregate total
is still close to the Milestone 2 locals-arena checkpoint, so larger remaining
wins are likely in closure churn, Result allocation shape, and loop/scalar
lowering.

### Initial notes

- PR #101 established functional parity and executable build support for
  `vm_next`.
- The PR benchmark table showed `vm_next` executables roughly `1.5x` to `5.0x`
  slower than current bytecode VM executables depending on workload.
- A first profiling pass after adding `ARD_VM_NEXT_PROFILE=1` indicated that
  pure-runtime overhead dominates several benchmarks, while FFI reflection is a
  major but narrower issue for decode/sql workloads.
