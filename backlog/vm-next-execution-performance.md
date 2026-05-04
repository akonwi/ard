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

Status: Done

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
- [x] Avoid generic `[]Value` allocation for common call arities.
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
- [x] Introduce reusable frame/local storage.
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
  - Historical note: signature-based direct adapters were evaluated during M2,
    M4, and M7, but were later removed because VM-local stdlib adapter matrices
    do not scale. Future FFI optimization should be generated from stdlib
    metadata or implemented in stdlib host functions rather than hand-written in
    `vm_next`.
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
- [x] Benchmark pure-runtime programs after each step.

### Milestone 3: Value representation improvements

Status: Done

The current `Value` representation boxes many common Ard values behind `Ref any`.
This is simple but allocation-heavy and type-assertion-heavy.

This milestone was the primary follow-up for the remaining `decode_pipeline`
gap after Milestone 2. Profiling showed decode still executed hundreds of
thousands of `TryResult`, `Return`, closure, and frame operations. Fast FFI
adapters removed much of the reflective boundary cost, and Milestone 3 narrowed
that remaining value pressure to Result representation plus hot wrapper/ref
access paths.

Milestone 3 is complete as of the `vm_next` Result representation, detailed
value-profile counters, inlineable hot `Ref` access helpers, and cached runtime
Void `TypeID` lookup. The remaining decode gap is no longer primarily a generic
value-representation problem: detailed profiles point to list/map access,
closure calls/creation, and loop/interpreter mechanics. Those follow-ups are
tracked in Milestones 5 and 6 rather than extending this milestone with broader
speculative representation changes.

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
- [x] Inline or specialize common wrappers.
  - [x] `Maybe` fast representation.
    - Closed without a global representation change. A broad Result-style
      `Maybe` representation was already rejected, and detailed counters show
      Maybe is hot in shape/sql/word-frequency paths but cold in decode. The
      hot `Maybe` sites are mostly `Some` results from collection/map access, so
      remaining opportunities belong with Milestone 5 collection/lowering work
      rather than a global `ValueMaybe` redesign.
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
- [x] Reduce `Ref any` usage on hot paths.
  - [x] Prefer typed heap references or tagged heap records.
    - Retained the successful typed `*Value` Result payload plus dedicated
      scalar Result tags. Broader storage redesigns for Maybe/Union/reference
      payloads regressed, so the retained completion strategy is typed/narrow
      records only where profiles and full-suite benchmarks justified them.
  - [x] Add profiling counters for typed `Ref any` access by helper kind so
    future storage changes can be driven by actual assertion/access volume.
  - [x] Add inlineable fast-ref helpers for hot list/map/struct/union/closure
    access paths while preserving detailed defensive error messages on the cold
    invalid path. This targets the `decode_pipeline` profile where list, map,
    and closure `Ref` access dominate remaining value-representation pressure.
    - Checkpoint commit: `fa53c17 perf(vm_next): inline hot value ref access`.
    - 10-run runtime suite after this change: aggregate `806.7 ms`; strongest
      directional win was `decode_pipeline` at `367.5 ms` vs the immediately
      previous `383.2 ms` checkpoint. `word_frequency_batch` also improved to
      `78.7 ms`, and `sql_batch` to `55.3 ms`; `fs_batch` remained noisy.
    - Same-suite current bytecode VM comparison: aggregate `634.8 ms`, so
      `vm_next` remains about `1.27x` slower overall. The largest remaining
      absolute gap is still `decode_pipeline` (`vm_next 367.5 ms` vs current VM
      `254.3 ms`), followed by collection/loop-heavy `word_frequency_batch`
      (`78.7 ms` vs `52.0 ms`).
    - Next decode-oriented work should continue targeting list/map/closure
      access and closure churn. Detailed counters show `Maybe` and zero-value
      handling are not decode bottlenecks.
  - [x] Avoid repeated stack traffic and value assertions for local match
    subjects by lowering Result and Union match probes/extractions to local
    bytecode opcodes (`ResultIsOkLocal`, `ResultExpectLocal`,
    `ResultErrValueLocal`, `UnionTagLocal`, and `UnionValueLocal`). This keeps
    defensive validation but removes repeated `LoadLocal` + wrapper op pairs in
    decode/shape/sql hot paths. A follow-up attempt to inline the Result-local
    helper directly into the interpreter switch passed tests but worsened the
    full 10-run suite, so the smaller helper-based form was retained.
  - [x] Avoid repeated Go type assertions for validated value kinds.
    - Completed for the hot validated paths identified by profiles using local
      Result/Union opcodes and inlineable fast-ref helpers for list/map/struct/
      union/closure access. Further assertion elimination should be tied to a
      specific Milestone 5/6 lowering or closure change.
- [x] Add cheaper zero-value handling.
  - [x] Add profiling counters for `zeroValue` calls by AIR type category.
  - [x] Cache common zero values by `TypeID` where safe.
    - A broad immutable zero-value cache was evaluated and rejected because it
      did not improve the 10-run runtime suite. Detailed counters now show zero
      values are cold for decode and mostly `Void` in sql/fs-style paths, so no
      broader zero cache is retained for Milestone 3.
  - [x] Avoid repeatedly scanning type tables for `Void`.
    - Cached the runtime Void `TypeID` in `VM` and use it for empty entry/script
      returns and fallback Maybe zero handling instead of repeatedly scanning
      the type table.
- [x] Benchmark decode/result-heavy and map/maybe-heavy workloads after local
  match subject opcodes.
- [x] Run final Milestone 3 validation.
  - `cd compiler && go test ./...`
  - `cd compiler && go test -tags vmnext_profile_detail ./vm_next`
  - `cd compiler && ./benchmarks/run.sh --mode runtime --runs 10 --warmup 3`
  - Final Milestone 3 checkpoint after cached Void `TypeID`: vm_next aggregate
    `805.2 ms` (`sales_pipeline 77.2`, `shape_catalog 97.6`,
    `decode_pipeline 369.9`, `word_frequency_batch 78.7`, `async_batches 16.3`,
    `fs_batch 110.4`, `sql_batch 55.1`). The same run's current bytecode VM
    aggregate was `641.3 ms` with noisy `shape_catalog`/`async_batches`, so
    vm_next remains about `1.26x` slower overall. Use the earlier `fa53c17`
    checkpoint (`decode_pipeline 367.5 ms`) as the cleaner decode signal for
    the hot Ref-access optimization; the cached Void `TypeID` change is a small
    cleanup/closure item rather than a major benchmark driver.

### Milestone 4: Direct generated FFI adapters

Status: Rejected / removed

This milestone originally explored avoiding `reflect.Value` creation and
`reflect.Call` for hot stdlib externs by adding direct typed adapters in
`vm_next`. The performance wins were real for SQL/decode profiles, but the
approach was rejected architecturally:

- VM internals started accumulating knowledge of stdlib-specific bindings and
  host representation details.
- Name-based shortcuts such as `DecodeInt` / `DecodeString` duplicated stdlib
  semantics inside `vm_next`.
- The adapter type-switch grew with every stdlib signature and would not scale as
  the standard library evolves.
- Custom/user externs still needed reflection, so the system became two adapter
  models rather than one coherent FFI design.

The direct-adapter implementation and `hostExternAdapter.direct` hook were
removed. `compiler/vm_next/ffi.go` now uses the validated reflective adapter path
for all host externs again. This keeps the FFI layer generic and makes stdlib
semantics live in `compiler/std_lib/ffi`, not in the VM.

Future FFI performance work should avoid hand-maintained VM special cases. If
reflection overhead becomes important again, prefer one of these designs:

- Generate direct adapters from the same source of truth that defines stdlib FFI
  bindings, so adapter coverage changes automatically with the stdlib.
- Move semantic optimizations into `compiler/std_lib/ffi` host functions
  themselves, where the stdlib behavior already lives.
- Add a public opt-in adapter API for custom host registries rather than hidden
  binding-name or stdlib-shape checks inside `vm_next`.

Validation after removing the direct adapter layer:

- `cd compiler && go test ./...`
- `cd compiler && go test -tags vmnext_profile_detail ./vm_next`

### Milestone 5: Collection and iteration fast paths

Status: Done

Milestone 5 is complete as of the vm_next collection/loop performance branch.
The aggregate target was reached: best official runtime-suite measurement for
`vm_next` was `599.5 ms` versus `623.6 ms` for the current bytecode VM in the
same run (`0.961x`, `24.1 ms` faster aggregate). This improved the Milestone 5
baseline of `795.4 ms` by `195.9 ms` (`-24.6%`).

This milestone focused on general collection and loop execution patterns rather
than benchmark-specific behavior. Retained changes include local collection
opcodes, map/Maybe fast paths, scalar loop-control jump fusions, deterministic
map iteration with sorted indices, and reduced map entry copying.

Remaining caveat: `decode_pipeline` is still slower than the current bytecode VM
(`305.7 ms` vs `252.6 ms` in the best aggregate run). That gap is now tracked
separately in the decode-focused milestone below rather than blocking Milestone
5, because vm_next is now faster on aggregate and substantially faster on
collection-heavy pure-runtime benchmarks such as `word_frequency_batch`.

- [x] Add explicit bytecode instructions or lowered operations for list
  iteration.
  - [x] Avoid repeated method-style `list.size()`/`list.at()` patterns for
    `for item in list` by lowering local-list size/index access to specialized
    bytecode opcodes.
  - [x] Support index-producing list loops without extra method overhead.
  - [x] Add local list push and modulo-list-access fast paths for common
    push-heavy and cycling patterns.
- [x] Track and reduce loop-control overhead in collection-heavy programs.
  - [x] Re-profile `word_frequency_batch` during retained runtime changes and
    monitor `LoadLocal`, `StoreLocal`, `ConstInt`, `IntAdd`, `Jump`, and
    `JumpIfFalse` counts.
  - [x] Add retained scalar loop-control reductions including local integer
    add-with-constant, fused loop-body/backedge increment, local loop-limit
    checks, and modulo-equality condition jumps.
  - [x] Record rejected variants to avoid retrying unstable shapes: broad fused
    increment/store-local variants, `ReturnLocal`, store/load keep peepholes,
    direct-store collection iteration opcodes, guarded list-at iteration, and
    broad local alias rewrites all passed tests but regressed the aggregate
    benchmark suite.
- [x] Add explicit map iteration support.
  - [x] Avoid sorting/copying entries more often than necessary by using sorted
    map-entry indices for deterministic iteration.
  - [x] Preserve deterministic iteration order.
  - [x] Return sorted map iteration entries by pointer in the immediate helper
    path to avoid extra `MapEntryValue` copies.
- [x] Improve primitive-key map lookup without changing deterministic map
  storage semantics.
  - [x] Specialize primitive-key scans inside `mapEntryIndex` while retaining
    linear entry order and deterministic sorted iteration.
  - [x] Avoid full map-entry copies while scanning lookup keys.
  - [x] Record rejected primitive-key index/hash-map directions; lazy indexes
    and call-site string-key specialization regressed the aggregate suite.
- [x] Evaluate list growth/capacity improvements for common push-heavy patterns.
  - [x] Blanket empty-list preallocation and first-push capacity allocation both
    regressed and are not retained.
- [x] Benchmark retained changes with the official runtime suite
  (`--runs 10 --warmup 3`) and validate with `go test ./...` plus
  `go test -tags vmnext_profile_detail ./vm_next`.

### Milestone 6: Closure, trait, and async call fast paths

Status: Done

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

After Milestone 3, the next step was closure callsite profiling rather than a
new broad representation experiment. Closure aggregate counts were high enough
to explain a large part of the remaining decode gap, but prior runtime-wide
cache/collection special cases were fragile. Milestone 6 therefore kept the
narrow changes that directly reduced hot local closure call overhead,
zero-capture closure allocation, and sort comparator callback allocation, while
closing speculative hoisting/trait/fiber items with profiling-backed rationale.

- [x] Reduce closure creation allocation.
  - [x] Add closure callsite profiling by bytecode closure function.
    - Records top closure functions by call count plus creation count, average
      arity, locals, and capture distribution under `ARD_VM_NEXT_PROFILE=1`.
      This is intended to guide targeted lowering/call-path changes and avoid
      repeating broad closure-cache experiments that regressed the full suite.
    - Initial `decode_pipeline` sample shows closure traffic is concentrated in
      a small set of unary decoder functions/closures: `int` (~336k calls,
      ~36k zero-capture creations), one 2-capture anonymous decoder closure
      (~36k calls/creations), `string` (~48k calls, ~12k zero-capture
      creations), one 1-capture anonymous closure (~24k calls/creations), and
      one 2-capture anonymous closure (~12k calls/creations). This strongly
      suggests local/unary closure call lowering or safe non-escaping direct
      calls will be more promising than another broad closure cache.
  - [x] Evaluate capture distribution and zero-capture closure caching. Both
    map-backed and bytecode-indexed cache experiments passed tests but regressed
    the full benchmark suite, so only the profiling histogram was kept.
  - [x] Avoid capture slice allocation for zero-capture closures only if a
    cheaper representation than runtime caching is available.
    - Added a dedicated inline zero-capture closure value (`ValueClosureFunc`)
      that stores the bytecode function ID in `Value.Int` and avoids allocating
      a `ClosureValue` record for zero-capture closures. Hot closure call paths
      use closure parts directly, while compatibility paths still reconstruct a
      `ClosureValue` when needed for host callbacks/Go conversion.
    - Decode profile effect: closure creations remain ~120k, but counted
      closure heap allocations drop from ~120k to ~72k because the ~48k
      zero-capture `int`/`string` decoder closures no longer allocate wrapper
      records.
    - 10-run runtime checkpoint: vm_next aggregate `781.4 ms`
      (`sales_pipeline 77.3`, `shape_catalog 91.5`, `decode_pipeline 357.6`,
      `word_frequency_batch 78.4`, `async_batches 13.3`, `fs_batch 108.9`,
      `sql_batch 54.4`). This is a small aggregate improvement over the
      `CallClosureLocal` checkpoint (`785.6 ms`), mostly from fs noise and small
      collection/sql/decode movement; retain because it removes real allocation
      on a semantically narrow zero-capture case without runtime caching.
  - [x] Evaluate small-arity capture collection fast paths for `OpMakeClosure`.
    Special-casing 0/1/2 captures removed `popArgs` profile counts but did not
    remove captured-slice allocation, and the full 10-run suite did not improve;
    keep the simpler generic helper until representation changes remove the
    underlying allocation.
  - [x] Reuse or compact capture storage where safe.
    - Evaluated shrinking captured `ClosureValue` records by removing the
      redundant `Type` field after zero-capture closures moved inline. Tests
      passed, but the 10-run suite did not improve directionally (`vm_next`
      aggregate about `783.3 ms` vs the `781.4 ms` inline-zero-capture
      checkpoint; `decode_pipeline 358.3 ms` vs `357.6 ms`). Reverted. Further
      captured-closure storage changes should come from a design that also
      removes/reuses the capture slice itself.
    - Evaluated a zero-capture-only `OpMakeClosure` branch after inline
      zero-capture closures landed. Tests passed, but the 10-run suite regressed
      directionally (`vm_next` aggregate about `791.0 ms`, `decode_pipeline
      364.9 ms`). Reverted; the simpler generic `OpMakeClosure` layout remains
      better despite the extra helper call for zero-capture closures.
  - [x] Investigate lowering-level closure hoisting/reuse for decoder
    combinators; autoresearch rejected a runtime-only unary capture inline
    representation because the added branching hurt the aggregate suite.
    - Callsite profiling showed zero-capture `int`/`string` decoders and
      captured anonymous combinator closures dominate decode. The safe
      zero-capture case is now handled by `ValueClosureFunc`; captured decoder
      closures depend on per-call captured decoder/name state, so hoisting or
      reuse would need broader closure lifetime/alias analysis. Do not pursue
      speculative hoisting in Milestone 6 without new compiler analysis.
- [x] Add closure call fast paths for common arities.
  - [x] Add `CallClosureLocal` lowering for calls whose closure target is a
    local. This removes the preceding target `LoadLocal` and keeps closure
    arguments on the stack, matching the existing local-opcode strategy used by
    Result/list/map hot paths.
    - Decode profile effect: `CallClosure` becomes `CallClosureLocal` for the
      hot closure call sites and `LoadLocal` drops by ~456k in `decode_pipeline`
      (`~4.31M` to `~3.85M` in representative profiles).
    - 10-run runtime checkpoint after this change: vm_next aggregate `785.6 ms`
      (`sales_pipeline 77.2`, `shape_catalog 91.2`, `decode_pipeline 357.8`,
      `word_frequency_batch 78.3`, `async_batches 13.1`, `fs_batch 113.6`,
      `sql_batch 54.4`). The same run's current bytecode VM aggregate was
      `627.8 ms`; vm_next remains about `1.25x` slower overall, with decode
      still the largest absolute gap (`357.8 ms` vs `250.7 ms`).
  - [x] Pay special attention to unary closure calls from decode and
    `Maybe`/`Result` mapper paths.
    - Evaluated a narrower `CallClosure1Local` opcode after `CallClosureLocal`
      converted all hot decode closure calls to local-target calls. It kept the
      same opcode count reduction but regressed the 10-run runtime suite
      (`vm_next` aggregate about `791.7 ms` vs the `785.6 ms` `CallClosureLocal`
      checkpoint, with `decode_pipeline` `361.8 ms` vs `357.8 ms`). Reverted;
      keep the more general `CallClosureLocal` path.
    - Also evaluated keeping `CallClosureLocal` but branching internally to a
      unary frame-init helper for `argCount == 1`. This avoided a separate
      opcode but still regressed directionally (`vm_next` aggregate about
      `789.0 ms`, `decode_pipeline 359.9 ms`, `sql_batch 55.2 ms`). Reverted;
      the generic local closure frame path has better aggregate behavior.
- [x] Make trait calls cheaper after bytecode validation.
  - [x] Pre-resolve impl method function IDs in bytecode operands.
    - Closed without new runtime changes. Trait dispatch is not hot in the
      current runtime suite: representative profiles show only one trait call in
      each benchmark, while decode has one trait call versus hundreds of
      thousands of closure/list/result operations. Dynamic impl selection means
      the exact function still depends on the receiver's impl at runtime; a
      precomputed trait-method table may be useful later, but it is not a
      Milestone 6 performance lever.
  - [x] Avoid repeated trait/impl bounds checks in hot dispatch.
    - Intentionally retained defensive checks because trait dispatch is cold and
      earlier validation-pruning experiments showed similar small-check removals
      did not improve the full suite.
- [x] Review sort comparator calls and other high-frequency callback paths.
  - Added `callClosure2` for list sort comparators so sort callbacks use a
    fixed two-argument closure call path instead of allocating a `[]Value` for
    every comparison. This targets `sales_pipeline`, `shape_catalog`, and
    `word_frequency_batch` sort-heavy paths while preserving callback semantics.
  - 10-run runtime checkpoint: vm_next aggregate `780.2 ms`
    (`sales_pipeline 78.2`, `shape_catalog 93.2`, `decode_pipeline 356.6`,
    `word_frequency_batch 77.3`, `async_batches 13.1`, `fs_batch 107.5`,
    `sql_batch 54.3`). Aggregate improved slightly over the inline zero-capture
    closure checkpoint (`781.4 ms`), with the clearest intended win in
    `word_frequency_batch`; retained despite small sales/shape noise.
- [x] Keep fiber spawn/get/wait profiling and safety behavior intact.
  - Existing fiber profiling remained intact, and `async_batches` passed tests
    and benchmarks after closure changes. No fiber-specific fast path was added
    because the Milestone 6 wins came from closure call/creation changes and
    async workloads are already competitive.
- [x] Benchmark `decode_pipeline`, `sql_batch`, `async_batches`, and sort-heavy
  samples.
  - Final Milestone 6 checkpoint after sort comparator callback cleanup:
    vm_next aggregate `780.2 ms` (`sales_pipeline 78.2`, `shape_catalog 93.2`,
    `decode_pipeline 356.6`, `word_frequency_batch 77.3`, `async_batches 13.1`,
    `fs_batch 107.5`, `sql_batch 54.3`). Same-run current bytecode VM aggregate
    was `619.2 ms`, so vm_next remains about `1.26x` slower overall. The largest
    remaining absolute gap is still decode (`356.6 ms` vs `249.5 ms`), with
    additional collection/loop overhead tracked by Milestone 5.

### Milestone 7: Decode pipeline performance

Status: Done

After Milestone 5, `vm_next` beats the current bytecode VM on aggregate, but
`decode_pipeline` remains the largest known benchmark gap. In the best retained
Milestone 5 run, `decode_pipeline` measured `305.7 ms` for `vm_next` versus
`252.6 ms` for the current VM. Treat this as the next targeted performance
milestone instead of continuing to overload collection/loop work.

Current decode profile signals after Milestones 3, 5, and 6:

- High `LoadLocal` / `StoreLocal` / `Jump` volume remains, but several broad
  local/load-store rewrites reduced opcode counts while regressing aggregate
  runtime. Avoid repeating those shapes without a materially different design.
- `TryResult`, `ResultExpectLocal`, `ResultIsOkLocal`, `MakeResultOk`, and
  `Return` remain very frequent in decoder combinator paths.
- `CallExtern` is still high around dynamic decode helpers even after earlier
  signature-based adapter and Result representation improvements.
- Closure calls/creations were improved in Milestone 6, but decode still has
  captured closure-heavy structure.
- List/map iteration is still visible, but direct-store and guarded list
  iteration opcode variants have repeatedly failed aggregate scoring.

M7 progress to date:

- Re-profiled both VMs on `decode_pipeline`. Before M7 changes, the remaining
  gap was strongly concentrated in the decode extern path: `vm_next` profiled at
  about `352 ms` wall with `102 ms` extern time, while the current VM profiled at
  about `286 ms` wall with `56 ms` extern time. `JsonToDynamic` alone accounted
  for most of that extern gap.
- Switched shared host FFI `JsonToDynamic` from `encoding/json` v1
  `Decoder.UseNumber()` to `encoding/json/v2.Unmarshal`, matching the faster
  current-VM parser behavior while preserving dynamic JSON semantics. This
  dropped vm_next profiled `JsonToDynamic` time from about `64 ms` to about
  `28 ms`.
- Changed `DynamicToMap` to keep JSON object keys as `map[string]any` instead of
  copying them through `map[any]any`, and taught vm_next's fast adapter to wrap
  string keys directly as Dynamic keys. This reduced profiled `DynamicToMap`
  time from about `8.1 ms` to about `5.4 ms`.
- Evaluated vm_next-specific direct scalar success paths for stdlib `DecodeInt`
  and `DecodeString`. They improved decode profiles, but were later removed
  during M4 cleanup because VM-local stdlib semantic shortcuts do not scale and
  duplicate stdlib behavior.
- Stored vm_next Dynamic payloads directly in `Value.Ref` instead of allocating a
  `DynamicValue` wrapper for every Dynamic value. This is especially relevant to
  decode, which wraps many JSON list/map elements as Dynamic; it reduced the
  10-run `decode_pipeline` mean from about `261.0 ms` to about `253.7 ms`.
- Simplified vm_next dynamic extern argument extraction to read the inline
  Dynamic payload directly, trimming the hot FFI argument path without changing
  observable Dynamic values.
- Final 10-run runtime-suite checkpoint after these M7 changes: vm_next
  `decode_pipeline` `253.1 ms` versus current VM `248.8 ms`, leaving an
  approximately `4.3 ms` decode gap. The same run had vm_next aggregate
  `546.0 ms` versus current VM aggregate `616.2 ms`.
- Rejected follow-up experiments during this pass:
  - Inlining stdlib `DynamicToList`, `DynamicToMap`, and `ExtractField` success
    checks into vm_next adapters looked plausible from profile data but
    regressed profiled wall time, so it was reverted.
  - Inlining `TryResult` handling directly into the bytecode dispatch loop also
    regressed profiled wall time, likely due to code-layout/instruction-cache
    effects outweighing the removed helper call.
  - Adding a larger zero-capture `CallClosureLocal` inline branch similarly
    regressed profiled wall time and was reverted.
  - Retesting only the `DynamicToList` inline adapter after inline Dynamic
    payloads still regressed profiled wall time, so it was reverted as well.
  - Adding a specialized `ValueResultDynamic` representation reduced profiled
    Result ref accesses by about 48k but did not improve the official
    `decode_pipeline` benchmark, so it was reverted.
  - Storing one- and two-capture closures inline removed all profiled closure
    capture slice allocations in `decode_pipeline`, but increased profiled wall
    time substantially, so it was reverted.

Recommended focus:

- [x] Re-profile `decode_pipeline` from the post-Milestone-5 tree with both
  opcode counters and detailed profile counters enabled.
- [x] Separate decode time into host JSON/dynamic extern work, Result/try
  control flow, closure dispatch/creation, collection iteration, and local loop
  overhead.
- [x] Prefer decode-general improvements that also preserve aggregate stability;
  avoid benchmark-name or payload-specific behavior.
- [x] Investigate whether more direct generated FFI adapters for dynamic/json
  decode externs belong here or should be completed under Milestone 4 first.
  - M7 kept host representation fixes. Later M4 experiments showed that
    hand-maintained direct adapters are not the right architecture; future FFI
    optimization should use generation from stdlib metadata or host-side
    stdlib changes instead.
- [x] Investigate decoder Result/try lowering only if it removes meaningful work
  without recreating previously rejected broad fused try/extern shapes.
  - Tried direct `TryResult` dispatch inlining and a `ValueResultDynamic` shape;
    both failed official benchmark validation and were reverted.
- [x] Keep collection semantics, deterministic map iteration, closure semantics,
  and public Ard behavior unchanged.
- [x] Benchmark with the official runtime suite and track `decode_pipeline`
  separately from aggregate `total_ms`.

### Milestone 8: Scalable FFI performance architecture

Status: In progress

After M4 rejected VM-local direct adapter matrices, the remaining FFI performance
problem should be solved with a scalable architecture rather than hidden
`vm_next` fast paths. The goal is to recover the benefits of non-reflective FFI
where it matters while keeping stdlib semantics and host representation details
out of generic VM adapter code.

Chosen direction: generate adapters from stdlib FFI metadata.

The adapter layer is an internal compiler/runtime concern. External stdlib FFI
host code should remain ignorant of adapter concepts and continue exposing normal
Go functions. The compiler/runtime can generate any non-reflective wrappers it
needs from the same metadata that already defines stdlib FFI bindings.

Accepted design:

1. Generate adapters from stdlib FFI metadata.
   - Use the stdlib FFI registry / generated metadata as the source of truth.
   - Generated code can call typed host functions without `reflect.Call`, but
     should be regenerated whenever stdlib signatures change.
   - `vm_next` should load/register generated adapters; it should not hand
     maintain a growing signature switch.
   - Keep generated adapter behavior signature-driven and metadata-driven, not
     benchmark-driven or binding-name-special-cased.

Rejected designs:

2. Do not make stdlib FFI functions expose a VM-native adapter form.
   - External FFI code should not know about adapters. Adapters are an internal
     compiler/runtime implementation detail.
3. Do not change the vm_next stdlib extern ABI for hot decode functions to
   accept/return Value-like handles.
   - This would create a special lower-level ABI for one family of stdlib calls
     instead of solving FFI generation generally.
4. Do not move decode primitives out of external FFI into VM intrinsics.
   - This does not scale across targets and would make decode semantics a VM
     concern rather than a stdlib/target concern.

Checklist:

- [x] Compare the four designs and pick generated adapters as the primary
  direction.
- [x] Generate `vm_next` stdlib host adapters from the existing stdlib FFI
  metadata generator.
  - The generated adapters type-assert the registered host function, convert
    `Value` arguments to typed Go values, call the host function directly, and
    convert typed returns back to `Value` without `reflect.Call`.
  - Generation uses Go AST construction/printer APIs for the adapter file rather
    than hand-building Go source strings.
- [x] Preserve generic reflective fallback for user/custom externs.
  - If no generated stdlib adapter matches, or if a same-binding override does
    not have the generated stdlib function type, `vm_next` falls back to the
    validated reflective adapter path.
- [x] Keep stdlib semantics outside generic VM adapter code.
  - Generated adapters call the registered host functions; they do not
    reimplement binding behavior such as decode semantics.
- [x] Avoid binding-name special cases in `vm_next/ffi.go`.
  - `ffi.go` only asks the generated adapter registry for an optional direct
    callable; binding-specific cases live in generated code.
- [x] Benchmark at least `decode_pipeline`, `sql_batch`, and full runtime suite.
  - Focus run after generated adapters: `decode_pipeline` vm_next `306.3 ms`
    vs current VM `259.8 ms`; `sql_batch` vm_next `51.9 ms` vs current VM
    `50.2 ms`.
  - Full runtime-suite run after generated adapters: vm_next aggregate about
    `642.8 ms` vs current VM about `680.9 ms` in the same run. `decode_pipeline`
    remains the largest gap at `306.3 ms` vs `262.1 ms`.

Next FFI architecture item:

- [ ] Generalize FFI adapter generation beyond the built-in stdlib.
  - Long-term, any host extern contract should be able to generate the same
    pieces the stdlib now generates: host-facing Go declarations plus internal
    `vm_next` adapter glue.
  - Host implementations should remain ordinary Go functions and should not know
    about adapters, `Value`, stacks, or VM internals.
  - Generated adapters should become the intended production path for custom
    host extern sets, with reflective fallback kept for development, tests,
    dynamic embedding, and unsupported shapes while generator coverage matures.
  - This would let embedders write normal host functions while avoiding
    `reflect.Call` in steady-state execution.

### Milestone 9: Profiling and benchmark tracking

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
remaining reflective extern path simpler to profile before any future generated
FFI adapter work. M4 later rejected hand-maintained VM-local direct adapters.

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

### Milestone 5 completion snapshot

Validation after merging the finalized Milestone 5 branch into `refactor.vm-next`:

- `cd compiler && go test ./...`
- `cd compiler && go test -tags vmnext_profile_detail ./vm_next`

Best retained 10-run runtime-suite measurement from the Milestone 5
autoresearch session:

| Benchmark | vm_next | current bytecode VM |
|---|---:|---:|
| `sales_pipeline` | 47.7 ms | 67.9 ms |
| `shape_catalog` | 57.6 ms | 81.7 ms |
| `decode_pipeline` | 305.7 ms | 252.6 ms |
| `word_frequency_batch` | 25.5 ms | 51.4 ms |
| `async_batches` | 8.4 ms | 14.8 ms |
| `fs_batch` | 106.6 ms | 108.0 ms |
| `sql_batch` | 48.0 ms | 47.2 ms |
| **total** | **599.5 ms** | **623.6 ms** |

Outcome: vm_next is now faster on aggregate (`0.961x` current VM) and much
faster on most pure collection/loop workloads. The remaining major gap is
`decode_pipeline`, which is now tracked as its own follow-up milestone.

### Milestone 4 direct-adapter attempt and removal

Validation after removal:

- `cd compiler && go test ./...`
- `cd compiler && go test -tags vmnext_profile_detail ./vm_next`

Summary:

- Direct adapters for SQL, scalar, Maybe, Dynamic, and raw extern-wrapper shapes
  were implemented and benchmarked.
- SQL profiling showed that direct adapters could remove most reflective SQL
  boundary cost.
- The design was rejected because it embedded stdlib-specific signature and
  representation knowledge in `vm_next`, and because the hand-maintained adapter
  matrix would grow with every stdlib FFI change.
- The direct adapter file and VM adapter hook were deleted. `vm_next` now uses
  the generic validated reflective adapter path for host externs.

Outcome: M4 is closed as a rejected strategy. Future FFI optimization should use
code generation from stdlib FFI metadata or move optimizations into stdlib host
functions rather than adding VM-local fast paths.

### Milestone 7 completion snapshot

Validation:

- `cd compiler && go test ./...`
- `cd compiler && go test -tags vmnext_profile_detail ./vm_next`
- `cd compiler && ./benchmarks/run.sh --mode runtime --runs 10 --warmup 3`

Changes in this checkpoint:

- Shared host `JsonToDynamic` now uses `encoding/json/v2.Unmarshal` instead of
  `encoding/json` v1 decoder + `UseNumber()`.
- `DynamicToMap` keeps JSON object maps as `map[string]any`, avoiding an
  intermediate `map[any]any` copy before vm_next wraps keys as Dynamic values.
- Direct hot success-path adapters for stdlib `DecodeInt` and `DecodeString`
  were evaluated during M7, but later removed during M4 cleanup because they
  embedded stdlib semantic shortcuts in `vm_next`.
- vm_next Dynamic values now store their raw payload directly in `Value.Ref`,
  removing the per-Dynamic wrapper allocation from JSON/list/map decode paths.

10-run mean runtime benchmark snapshot:

| Benchmark | vm_next | current bytecode VM |
|---|---:|---:|
| `sales_pipeline` | 47.5 ms | 66.2 ms |
| `shape_catalog` | 57.5 ms | 80.3 ms |
| `decode_pipeline` | 253.1 ms | 248.8 ms |
| `word_frequency_batch` | 25.6 ms | 51.0 ms |
| `async_batches` | 8.1 ms | 14.3 ms |
| `fs_batch` | 107.2 ms | 108.1 ms |
| `sql_batch` | 47.0 ms | 47.5 ms |
| **total** | **546.0 ms** | **616.2 ms** |

Outcome: the decode gap narrowed from about `53 ms` in the Milestone 5 best run
(`305.7 ms` vs `252.6 ms`) to about `4.3 ms` in this completion checkpoint
(`253.1 ms` vs `248.8 ms`). The large extern and Dynamic wrapper gaps have been
removed; remaining decode difference is small enough that further work should
move to generated FFI adapter work from stdlib metadata or general
interpreter/decoder-combinator architecture rather than more M7-specific tuning.

### Initial notes

- PR #101 established functional parity and executable build support for
  `vm_next`.
- The PR benchmark table showed `vm_next` executables roughly `1.5x` to `5.0x`
  slower than current bytecode VM executables depending on workload.
- A first profiling pass after adding `ARD_VM_NEXT_PROFILE=1` indicated that
  pure-runtime overhead dominates several benchmarks, while FFI reflection is a
  major but narrower issue for decode/sql workloads.
