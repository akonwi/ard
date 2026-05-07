# vm Execution Performance Completion Record

Status: complete

This document records the completed vm execution-performance work. That earlier
backlog is now closed; future vm performance or userland-FFI work should be
tracked as new, focused milestones.

## Goal

Improve real `compiler/vm` runtime performance without changing Ard
semantics, benchmark inputs, benchmark expected outputs, deterministic map
iteration, or observable collection mutation behavior.

The official benchmark command used for retained checkpoints was:

```bash
cd compiler && ./benchmarks/run.sh --mode runtime --runs 10 --warmup 3
```

Standard correctness checks were:

```bash
cd compiler && go generate ./std_lib/ffi
cd compiler && go test ./...
cd compiler && go test -tags vmnext_profile_detail ./vm
```

## Completed milestones

All execution-performance milestones are complete.

1. **Bytecode execution foundation**
   - vm runs through lowered bytecode with verifier coverage and parity
     tests.
   - Bytecode execution became the default vm path.
2. **Interpreter allocation overhead**
   - Removed avoidable temporary argument slices and reduced frame/local/stack
     allocation pressure.
   - Added profiling counters for frames, locals, stacks, arg slices, calls, and
     opcodes.
3. **Value representation and access paths**
   - Added value allocation/ref-access profiling.
   - Retained representation changes only where profiling plus the full runtime
     suite justified them.
4. **Direct FFI adapter attempt**
   - Hand-maintained VM-local direct adapters were tried, measured, then
     rejected and removed.
   - The rejection decision: VM internals must not accumulate stdlib-specific
     binding behavior or semantic shortcuts.
5. **Collection and iteration fast paths**
   - Added local collection/loop opcodes and map iteration improvements while
     preserving deterministic iteration order and map/list semantics.
   - vm beat the current bytecode VM on aggregate in the best retained
     Milestone 5 run.
6. **Closure, trait, and async call paths**
   - Added closure profiling and retained targeted zero-capture/closure-call and
     sort-callback improvements.
   - Trait/fiber paths were kept conservative where profiles showed they were
     not hot.
7. **Decode pipeline performance**
   - Switched shared host JSON decoding to `encoding/json/v2`.
   - Avoided dynamic map key conversion copies.
   - Stored Dynamic payloads inline in `Value.Ref`.
   - Removed VM-local decode semantic shortcuts during FFI cleanup.
8. **Scalable FFI performance architecture**
   - Generated vm stdlib FFI adapters from declared extern metadata.
   - Moved vm FFI bridge generation to `compiler/vm/ffi/generate.go`;
     `compiler/std_lib/ffi` remains the stdlib Ard project's host-code package.
   - Made generation part of local and CI validation.
   - Removed arbitrary reflective `reflect.Call` host extern dispatch from
     vm.
9. **Profiling and benchmark tracking**
   - Completed throughout the work rather than as a final separate pass.
   - `ARD_VM_PROFILE=1` reports opcode counts, calls, extern binding time,
     frame/local/stack allocation counters, value allocation counters, and
     optional detailed profile sections.

## Key retained architectural decisions

### Keep VM FFI generic

Generic VM FFI machinery belongs in `compiler/vm/ffi.go`. Stdlib-specific
or generated adapter details must live outside generic VM dispatch code.

The VM should not reimplement stdlib behavior for names such as `DecodeInt`,
`DecodeString`, SQL bindings, or other hot externs. Generated adapters may be
binding-aware because they are generated from extern metadata, but they must call
the registered host function rather than duplicate its semantics.

### Generated adapters are the vm host-extern path

The production vm host-extern path is generated adapter code:

```text
Ard extern declarations
  -> generated Go host contract
  -> ordinary Go host implementation
  -> generated vm adapter glue
  -> OpCallExtern calls the generated adapter
```

Generated adapters:

- type-assert the registered host function to the generated signature
- convert `Value` arguments to typed Go values
- call the registered host function directly
- convert typed returns back to `Value`
- preserve panic wrapping and Result/Maybe/error behavior
- avoid `reflect.Call`

The stdlib generation hook is:

```go
//go:generate go run ../../vm/ffi/generate.go
```

from `compiler/std_lib/ffi/doc.go`. It emits both host declarations and the
`vm` adapter lookup into the stdlib host package:

```text
compiler/std_lib/ffi/ard.gen.go
compiler/std_lib/ffi/vm_adapters.gen.go
```

`vm` owns only the generic adapter registry/API and bridge implementation
that lets generated host adapter packages convert to and from VM values.

### FFI adapter generation is vm-specific

The adapter registry/API and bridge generator live under `compiler/vm/ffi`.
They are not colocated with stdlib host code because the generated adapters are
runtime plumbing, while `compiler/std_lib/ffi` is the stdlib Ard project's host
implementation package.

The generated host package defaults to `ffi`, matching the intended project
layout where a project's `/ffi` directory contains host code. The stdlib follows
that same shape as an Ard project under `compiler/std_lib/ffi`. The legacy
`compiler/ffi` package remains for the current bytecode VM and Go-oriented
runtime paths.

### Leave the current bytecode VM alone

The generated vm adapter architecture was scoped to `vm`. The current
bytecode VM FFI registry and wrapper generator were intentionally left alone.

### Testing and CI require generation

Generation is now part of the compiler validation flow. CI runs stdlib FFI
generation before tests and fails if generated files are stale.

Local full validation should use:

```bash
cd compiler && go generate ./std_lib/ffi && go test ./...
```

For vm detailed profile coverage, also run:

```bash
cd compiler && go test -tags vmnext_profile_detail ./vm
```

Unit tests that existed only to validate arbitrary ad-hoc reflective FFI were
removed. Tests may still override stdlib bindings such as `Print` or
`HTTP_Serve` as long as the override matches the generated stdlib signature.

## Important benchmark checkpoints

Representative retained checkpoints:

- **Milestone 5 best aggregate:** vm `599.5 ms` vs current bytecode VM
  `623.6 ms` in the same run (`0.961x`, `24.1 ms` faster aggregate).
- **Milestone 7 decode checkpoint before later FFI architecture cleanup:**
  `decode_pipeline` vm `253.1 ms` vs current VM `248.8 ms`; aggregate
  vm `546.0 ms` vs current VM `616.2 ms`.
- **Generated adapter checkpoint:** focused run after generated adapters:
  `decode_pipeline` vm `306.3 ms` vs current VM `259.8 ms`; `sql_batch`
  vm `51.9 ms` vs current VM `50.2 ms`.
- **No reflective fallback checkpoint:** focused run after removing vm
  reflective host extern dispatch: `decode_pipeline` vm `298.8 ms` vs
  current VM `258.5 ms`; `sql_batch` vm `51.4 ms` vs current VM `51.3 ms`.

The decode benchmark remains the clearest remaining gap, but the original
execution-performance backlog is complete: the remaining work is future targeted
performance or userland FFI tooling, not an open milestone in this closed plan.

## Rejected directions to avoid repeating

Do not retry these without materially new evidence or a different design:

- Hand-maintained direct adapter matrices in `vm`.
- Binding-name semantic shortcuts in generic VM FFI code.
- VM intrinsics for decode primitives as a replacement for externs.
- A special Value-like ABI only for hot decode externs.
- Broad fused try/result or extern opcodes that previously regressed the full
  suite.
- Broad local alias/load-store rewrites that reduced opcode counts but regressed
  aggregate runtime.
- Zero-capture closure caches that add runtime cache checks to every closure
  expression.

## Future work

Future work should be tracked separately. Good follow-ups include:

- Wire the general extern bridge generator into userland Ard project tooling so
  custom host extern sets can generate the same host contract and vm adapter
  glue as the stdlib.
- Continue decode-focused profiling if closing the remaining decode gap becomes
  important.
- Improve generated adapter conversion helpers where list/map/struct conversion
  still falls back to reflective helper machinery internally.
- Keep profiling and benchmark snapshots as part of each future focused
  milestone rather than maintaining a broad performance backlog.
