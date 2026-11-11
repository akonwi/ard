---
name: compiler-performance
description: Profile and speed up the Ard compiler (checker, AIR lowering, Go backend, parser) without changing its output. Use when a compile, check, build, test run, or LSP response is slow, when optimising or reviewing a performance change, or when adding code to a hot path such as type walks, the type interner, name planning, or import-graph lookups.
---

# Compiler performance

The compiler's output is the contract: generated Go must stay byte-identical,
diagnostics unchanged, and `go test ./...` green on every change. Speed comes
from doing less work, not from tuning the runtime.

## Measure first

```bash
# stage split: frontend.load, air.lower, go.validate_lower_render, go.compile_link
ARD_PIPELINE_PROFILE=1 ard build main.ard --out /tmp/x

# CPU profile of the whole process
ARD_CPU_PROFILE=/tmp/ard.prof ard build main.ard --out /tmp/x
go tool pprof -top -nodecount=40 /tmp/ard.prof

# in-process medians for the compiler-owned stages, plus a hash of the output
cd compiler && go run ./internal/compilebench -input <path>/main.ard -reps 5 \
  -cpuprofile /tmp/cpu.prof -memprofile /tmp/mem.prof
```

- Profile allocation as well as CPU. When allocation is high, GC and
  `madvise` dominate the flat profile and only allocating less reduces them.
- Record the `compilebench` hash before changing anything and require every
  change to reproduce it. Generated Go is deterministic, so any hash change is
  an output change, not a perf change, and needs its own justification.
- Compare two binaries from the same toolchain, run alternately, and read
  medians. The machine is noisy; a single wall-clock number proves nothing.
- Use a large real program (the Kit CLI, ~120 files) as the benchmark. The
  hot paths are superlinear; small samples never show them.

## Do less work

Each rule below removed a measured multi-second cost in this codebase.

- **Memoise idempotent "ensure declared" passes on completion.** Set the memo
  only after a full successful pass. A pass re-entered while running must still
  rescan: declaration order allocates AIR IDs, and IDs become generated names.
- **No `reflect.DeepEqual` on structs in a loop.** Write a field-by-field
  equality that keeps the nil-vs-empty slice distinction, and add a test that
  reflects over the struct's field count so a new field cannot be missed
  (`air/type_info_equal_test.go` is the model).
- **Compute sequences in one ordered pass**, never by a definition where item
  *n* recomputes items 1..n-1. Pin the old semantics in a test, including the
  edge cases the recursion handled implicitly (`go/names_test.go`).
- **No whole-program scan per item.** Precompute counts or lookups once, keyed
  by `(module, name)` or similar, and store them on the `lowerer` / planner.
- **Cache sorted views of immutable maps.** Sorting is mandatory where map
  order could reach an ID or a name; cache the slice per `*checker.Program`.
- **Allocate copies on the first change, not up front.** Walks that return the
  original when nothing changed must not build a copy first. Callers rely on
  pointer identity (`!=`) to detect "unchanged"; test that contract.
- **Pool cycle-guard maps and skip them for leaves.** Use the existing
  `typeTraversalSeenPool` (`checker/scope.go`, `air/lower.go`) and
  `moduleSeenPool` (`checker/methods.go`); `clear()` on release. Gate the map
  with `typeWalkRecurses`, which lists exactly the kinds a walk descends into.
- **Cheap check before expensive pure check** in boolean expressions. A nil
  test before a type-graph walk is free; the reverse runs the walk every time.
- **Build keys with `strings.Builder` and `strconv`**, not `fmt.Sprintf` or
  `+=`. Pin the exact spelling in a test (`air/lower_keys_test.go`).
- **Presize slices from the input** when the ratio is known (the lexer uses
  `len(source)/4+1`, ~4.5 bytes per token).
- **Return nil, not an empty map**, when callers only range or index.

## Do not

- **Do not drop pooled `seen` maps above a size threshold.** Type walks
  routinely visit hundreds of types; the large maps are the ones worth
  reusing. Dropping those above 64 entries doubled allocation.
- **Do not memoise `hasGenericsInType` on `*StructDef`.** `struct` is a block
  statement, so a non-generic struct inside a generic function can reference
  the outer `T`; recursive declarations have partially filled `Fields` while
  being checked; and names recorded on non-generic functions propagate via
  `finalizeComparableConstraints`. A per-type summary needs a design.
- **Do not add package-level caches of pure functions** (`sync.Map` over
  `sanitizeGoIdentifier` and the like). They leak across compiles and LSP
  sessions, and the real cost is almost always algorithmic. Scope caches to
  the struct that owns one compile.
- **Do not tune `GOGC`, `GOMAXPROCS`, or `go build -p`.** They move benchmark
  numbers without making the compiler do less.
- **Do not apply `-ldflags=-w`** to cut link time. It drops DWARF; whether
  `ard build` should do that is a product decision to raise, not a fix.
- **Do not let map iteration reach an AIR ID, a generated name, or a
  diagnostic** without a sort. Determinism is load-bearing.

## Validate every change

```bash
cd compiler
go run ./internal/compilebench -input <path>/main.ard -reps 3   # hash unchanged
go test ./... -timeout 30m          # serially; concurrent runs time out in ./go
go vet ./...
go build -o /tmp/ard-lsp-test . && cd .. && python3 scripts/lsp-harness.py   # if checker or FFI resolution changed
```

Commit as `perf(<scope>): …` per `commit-format`. State the stage's
before/after, how output identity was verified, and name the one change whose
correctness argument is not mechanical.
