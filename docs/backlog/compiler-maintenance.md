# Compiler Maintenance Backlog

Status: living backlog, not an ADR.

This document tracks concrete opportunities to improve compiler performance,
reliability, and ownership boundaries. Update an item's status as work starts or
lands, and link the resulting issue, pull request, commit, or ADR when available.

Statuses: `planned`, `in progress`, `completed`, or `deferred`.

## Current priorities

| ID | Priority | Status | Opportunity |
| --- | --- | --- | --- |
| COMP-001 | P0 | completed | Cache checked embedded standard-library modules |
| COMP-002 | P1 | completed | Make AIR fully typed and Go lowering read-only |
| COMP-003 | P1 | completed | Finish the ADR 0057 AIR representation migration |
| COMP-004 | P0 | completed | Contain module imports within package roots and propagate filesystem errors |
| COMP-005 | P1 | completed | Complete LSP cancellation and dependency-aware invalidation |
| COMP-006 | P1 | completed | Stop silently returning incomplete project-wide references and renames |
| COMP-007 | P2 | completed | Remove the fallback Go importer |
| COMP-008 | P2 | completed | Consolidate unsafe-catch result analysis |
| COMP-009 | P2 | completed | Retire checker-node fallback type rules |
| COMP-010 | P0 | in progress | Stop requesting unused Go expression type information |

## 2026-08-08 profile audit

The current compiler at `45d799456c0ffa3b942e5ef7150fe4a349227975`
was profiled on Linux/amd64 with Go 1.26.0 and an Intel Xeon 2.60 GHz CPU.
The audit used three valid projects of different shapes:

| Workload | Revision | Ard files | Ard lines | Ard bytes |
| --- | --- | ---: | ---: | ---: |
| `examples/vaxis-demo` | compiler revision above | 1 | 1,259 | 38,101 |
| `tinear` | `251c8a5139fd6e63e1679282e0356001f54e62ee` | 34 | 8,949 | 291,239 |
| `maestro/server` | `52e57e1d216a61730b504c160c54a5bfb5c9779f` | 49 | 6,460 | 219,684 |

Tinear's sibling Vaxis checkout was pinned to
`24ba647481e9d881463f041093560d821fedc167`. The `site` repository at
`0fefe2ab8df49b32ba9b64eb3d55e2b90ed8a7f0` and the repository's
`examples/chi-server` were excluded because they still use the pre-ADR-0057
implicit reference syntax and do not check with the current language rules.
They are compatibility exclusions, not failed performance workloads.

Five separate `ard check` processes produced these median wall times and peak
resident-memory ranges:

| Workload | Median wall time | Peak RSS range |
| --- | ---: | ---: |
| `vaxis-demo` | 1.10 s | 449-500 MiB |
| `tinear` | 1.14 s | 474-500 MiB |
| `maestro/server` | 3.52 s | 940-1,024 MiB |

Command, run from each project root:

```sh
for i in 1 2 3 4 5; do
  /usr/bin/time -f "run=$i wall_s=%e max_rss_kb=%M" \
    /path/to/ard check main.ard >/dev/null
done
```

`ARD_PIPELINE_PROFILE=1 ard build` separated the compiler-owned stages from
the Go toolchain. Frontend loading and checking took 0.96-1.04 seconds for
`vaxis-demo`, 1.08-1.16 seconds for Tinear, and 3.38-3.55 seconds for Maestro.
AIR lowering took only 2.7-3.7 milliseconds, 40-44 milliseconds, and 57-77
milliseconds respectively. Go build time was additional and cache-dependent.
This makes the frontend, rather than AIR lowering, the material compiler-owned
build stage on all three workloads.

The reproducible cold-frontend benchmark below used three benchmark runs of
three measured iterations. The table records the median run:

| Workload | Time | Allocated bytes | Allocations |
| --- | ---: | ---: | ---: |
| `vaxis-demo` | 941.96 ms/op | 746.72 MB/op | 7,727,281 allocs/op |
| `tinear` | 954.63 ms/op | 805.09 MB/op | 8,215,779 allocs/op |
| `maestro/server` | 3,493.21 ms/op | 1,700.66 MB/op | 16,357,016 allocs/op |

```sh
ARD_BENCH_INPUT=/absolute/path/to/main.ard \
go test ./go -run '^$' \
  -bench '^BenchmarkGoPipeline/frontend_load_and_check$' \
  -benchmem -benchtime=3x -count=3
```

The Maestro CPU and allocation profiles attribute the cold cost primarily to
`go/packages` parsing and Go type checking. `go/types.recordTypeAndValue` alone
accounts for 35.08% of allocation space, while
`go/packages.(*loader).loadPackage` accounts for 71.46% cumulatively. This led
to COMP-010; the older internal candidates remain deferred below.

## COMP-001: Cache checked embedded standard-library modules

**Tracking:** [PR #370](https://github.com/akonwi/ard/pull/370)

**Status:** completed in PR #370 with CI passing.

Every normal checker invocation auto-imports six embedded modules. Each lookup
previously read, parsed, and checked immutable embedded source again. An initial
allocation profile attributed roughly 55% of allocations beneath
`Checker.Check` to embedded-module loading.

Bounded outcome:

- [x] Cache parse/check results once per embedded module path.
- [x] Keep the cache safe for concurrent LSP checks.
- [x] Verify generic specialization cannot mutate shared module definitions.
- [x] Preserve the existing standard-library validation gate.
- [x] Record before/after `BenchmarkGoPipeline/warm_check` results here.

Median of five runs at 500 iterations per run:

| Metric | Before | After | Change |
| --- | ---: | ---: | ---: |
| Time | 3,249,758 ns/op | 1,997,057 ns/op | -38.5% |
| Allocated bytes | 1,698,280 B/op | 762,769 B/op | -55.1% |
| Allocations | 15,366 allocs/op | 9,359 allocs/op | -39.1% |

Command: `go test ./go -run '^$' -bench '^BenchmarkGoPipeline/warm_check$'
-benchmem -benchtime=500x -count=5`.

## COMP-002: Make AIR fully typed and Go lowering read-only

**Status:** completed. AIR lowering now resolves contextual executable types,
AIR validation rejects incoherent local, constructor, try, and function-result
types, and Go lowering consumes AIR without recursively inferring types or
mutating `Program.Types`.

The checker also warns when source explicitly declares `Void?` or
`Maybe<Void>`. These types remain valid because they can represent
absence/presence, and inferred `Maybe<Void>` and `Result<Void, E>` remain
warning-free.

Bounded outcome:

- [x] Reproduce contextual typing edge cases at the AIR level.
- [x] Resolve contextual local, list, `Maybe`, `Result`, and panic expression
  types in AIR lowering.
- [x] Make AIR validation reject incoherent executable types required by
  targets while accepting assignable named Go types.
- [x] Remove Go-side recursive type inference and type-table mutation.
- [x] Verify Go lowering leaves serialized AIR byte-identical.
- [x] Warn on explicit `Void?` and `Maybe<Void>` without rejecting the module or
  warning on inferred types.

The `go_ast_lower` benchmark remained effectively neutral. Median of five runs
at 500 iterations per run was 3,539,270 ns/op before and 3,497,064 ns/op after;
both recorded 16,404 allocations/op. Command: `go test ./go -run '^$' -bench
'^BenchmarkGoPipeline/go_ast_lower$' -benchmem -benchtime=500x -count=5`.

## COMP-003: Finish the ADR 0057 AIR representation migration

**Status:** completed. AIR now represents reference type identity solely with
`TypeReference`. Binding mutability, local live-storage provenance, and capture
modes remain separate storage semantics. Foreign descriptor-value boundaries
use the per-parameter `ABIParamMode` projection, while exact foreign pointer
types retain their canonical foreign type identity; result adaptation continues
to use the single `ForeignResultShape` projection.

The migration removed `Param.Mutable`, `ReturnReference`, `ParamMutable`,
`ElemMutable`, `FieldInfo.Mutable`, the function-level foreign ABI flag, and the
duplicate foreign-expression argument mode. AIR validation checks ABI mode
cardinality and rejects descriptor-value projections unless the parameter is a
reference to a supported slice or map descriptor. Contradictory legacy
reference combinations are no longer structurally representable.

Bounded outcome:

- [x] Normalize descriptor parameter ABI projection alongside the existing
  normalized foreign result shape.
- [x] Keep binding mutability and capture modes as storage semantics.
- [x] Migrate Go lowering to canonical reference types and normalized ABI
  metadata without compatibility fallbacks.
- [x] Remove compatibility fields after producers and consumers migrate.
- [x] Reject malformed ABI/type combinations during AIR validation.
- [x] Verify AIR, checker, Go target, and full compiler tests pass.

## COMP-004: Contain imports and propagate filesystem errors

**Status:** completed. Module resolution now validates logical import segments
before constructing filesystem paths, verifies the resulting file remains
within its owning package root, and preserves unexpected filesystem errors.
Package roots, package manifests, and every component of a resolved module path
must not be symlinks. Symlinks above the declared package root remain valid, as
do path dependencies outside the application root when their own roots and
module paths satisfy the same policy.

Bounded outcome:

- [x] Reject empty, `.`, `..`, and absolute module paths.
- [x] Verify cleaned destinations remain inside the owning package root.
- [x] Reject symlinked package roots, manifests, module directories, and module
  files without rejecting symlinked ancestors above the root.
- [x] Return unexpected filesystem errors at the resolution boundary.

## COMP-005: Complete LSP cancellation and invalidation

Before this work, running diagnostics used a background context and semantic
requests resynchronized every overlay. Concurrent cache misses may still
duplicate complete checks.

**Status:** completed. Superseded per-document diagnostic jobs now cancel
their debounce timer or running analysis, propagate that context through the
snapshot engine, and exit without publishing cancellation as an analysis error.
The analysis workspace now owns overlays incrementally, with document metadata
and overlay transitions synchronized for coherent snapshots. Synthetic
completion and signature analysis derives from one immutable workspace snapshot.
Concurrent requests for one content signature now share a single engine-owned
check while retaining independent cancellation; failures are shared with current
waiters but remain retryable instead of entering the cache. Authoritative
analysis records revisioned forward and reverse Ard import edges, allowing
document changes to schedule diagnostics only for the changed document and its
transitive open dependents without allowing older snapshots to restore removed
edges. Incomplete dependency information caused by unanalyzed, missing,
unresolved, or malformed modules falls back to scheduling every open document;
open and close notifications also retain broad scheduling.

Bounded outcome:

- [x] Cancel superseded per-document diagnostic work.
- [x] Make the analysis workspace authoritative for incremental overlay updates.
- [x] Deduplicate concurrent checks for one content signature.
- [x] Track reverse dependencies and schedule only affected open documents.

## COMP-006: Make incomplete reference searches explicit

**Status:** completed. Project-wide references and rename now traverse every
Ard source under the project root except intentionally excluded generated,
version-control, and dependency directories. Traversal honors request
cancellation and propagates directory, file, and analysis failures to the LSP
client instead of returning accumulated locations. Rename constructs a
workspace edit only after the complete reference search succeeds.

Bounded outcome:

- [x] Propagate traversal errors.
- [x] Remove the arbitrary cap or return an explicit incomplete-search error.
- [x] Never apply a workspace rename from a known-partial result.

## COMP-007: Remove the fallback Go importer

**Status:** completed. A checker without an explicit Go resolver now creates the
same project-aware `GoPackagesResolver` used by production, configured with the
module root, build tags, and dependency checkout roots before priming the full
import closure. Recursive module checks reuse that resolver. Starting a later
default session invalidates checked user modules that could retain foreign types
from the prior `go/types` universe, while explicit resolver owners retain their
existing cache lifecycle. Standard-library-heavy checker tests share one
pre-primed resolver to avoid repeated `go/packages` loads.

Bounded outcome:

- [x] Give direct checker tests a shared primed resolver fixture.
- [x] Default Go-importing checks to a project-aware, primed resolver.
- [x] Preserve one Go type universe across recursive and cached module checks.
- [x] Delete `ImporterGoPackageResolver` and its fallback branch.

## COMP-008: Consolidate unsafe-catch result analysis

**Status:** completed. Unsafe-catch validation now collects successful and error
result value types in one traversal. Each alias stores the combined analysis,
and branch-local walks clone that single environment so updates cannot leak
between paths. Focused tests cover direct results, canonical constructors,
user-defined result modules, aliases, and every supported branching expression
family.

Bounded outcome:

- [x] Use one traversal that returns both successful and error value types.
- [x] Preserve one alias environment per path.
- [x] Add exhaustive tests for expression families handled by the analysis.

## COMP-009: Retire checker-node fallback type rules

**Status:** completed. List, map, maybe, and result method nodes now store the
return type resolved from their specialized standard-library signature, and
`Type()` returns only that value. The private function-definition references,
hard-coded return-type switches, and element/key/value metadata used only by
those fallbacks have been removed. Checker expectations now assert resolved
method return types.

Bounded outcome:

- [x] Require a resolved return type when constructing collection method nodes.
- [x] Migrate expected checker test nodes.
- [x] Remove fallback type switches and redundant precomputed fields where unused.

## COMP-010: Stop requesting unused Go expression type information

**Status:** implementation and verification complete on
`perf/compiler-profile-audit`; awaiting merge.

`GoPackagesResolver` previously requested `packages.NeedTypesInfo`, which asks
`go/types` to retain expression-level identifier, use, selection, and
type-and-value maps. Ard reads `packages.Package.Types`, `GoFiles`, and
`Errors`, but never reads `TypesInfo`; `packages.NeedTypes` already provides the
package type information needed by the FFI bridge.

The implementation removes only `NeedTypesInfo`. The full compiler suite and
the Vaxis, Tinear, and Maestro audit checks pass. The same cold-frontend
benchmark produced these median changes on the implementation branch:

| Workload | Time before | Time without `NeedTypesInfo` | Change | Bytes before | Bytes without `NeedTypesInfo` | Change |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `vaxis-demo` | 941.96 ms/op | 669.61 ms/op | -28.9% | 746.72 MB/op | 425.28 MB/op | -43.0% |
| `tinear` | 954.63 ms/op | 777.85 ms/op | -18.5% | 805.09 MB/op | 474.13 MB/op | -41.1% |
| `maestro/server` | 3,493.21 ms/op | 2,094.41 ms/op | -40.0% | 1,700.66 MB/op | 899.38 MB/op | -47.1% |

Allocation counts fell only 1.7-2.5%; the material gain comes from no longer
building and retaining the large unused maps.

Bounded outcome:

- [x] Remove `packages.NeedTypesInfo` without changing the shared `go/types`
  universe or requested build configuration.
- [x] Verify direct, generic, interface, callback, local-boundary, build-tag,
  and dependency-checkout Go FFI resolution.
- [x] Run the full compiler suite and check all three audit workloads.
- [x] Repeat the cold-frontend benchmark and record the branch results here.

## Profile-guided watchlist

The 2026-08-08 audit re-measured the earlier candidates:

- AIR lexical-scope map cloning accounted for 0.40% of CPU samples and 0.83%
  of allocation space in a 100-iteration Maestro AIR-lowering profile.
- `functionDirectlyReferences` and `canInlineClosureFunction` had no CPU
  samples in 100-iteration Go-lowering profiles for Vaxis, Tinear, or Maestro.
  The entire closure-use collection was at most 0.75% cumulatively.
- Each checker top-level declaration or validation pass accounted for at most
  0.06% of CPU samples in a 100-iteration Maestro project-check profile;
  `hoistTopLevelFunctionSignatures` was the largest allocation contributor at
  0.17%.
- Re-running `CollectGoImportPaths` with an already primed concrete resolver
  was visible in a synthetic cached-check loop (6.89% of CPU samples and
  27.44% of allocation space), but it was only 0.33% and 1.14% respectively in
  the cold frontend profile. The LSP's resolver wrapper already bypasses this
  per-check scan, so no production-critical item is promoted yet.

These remain deferred because they are small, absent, or not material on the
measured production paths. Reconsider them only when a representative profile
shows a larger cost or the resolver lifecycle changes.
