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
| COMP-008 | P2 | planned | Consolidate unsafe-catch result analysis |
| COMP-009 | P2 | planned | Retire checker-node fallback type rules |

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

Separate recursive walkers collect successful and error result types. They
duplicate alias tracking and enumeration of every checked expression and match
form, making traversal support easy to update on only one side.

Bounded outcome:

- Use one traversal that returns both successful and error value types.
- Preserve one alias environment per path.
- Add exhaustive tests for expression families handled by the analysis.

## COMP-009: Retire checker-node fallback type rules

Collection method nodes can derive return types both from resolved function
signatures and from hard-coded method-kind switches. Production constructors
already carry resolved signatures, so the fallback rules are a second source of
truth that can drift from embedded standard-library declarations.

Bounded outcome:

- Require a resolved return type when constructing collection method nodes.
- Migrate expected checker test nodes.
- Remove fallback type switches and redundant precomputed fields where unused.

## Profile-guided watchlist

The initial audit also considered AIR lexical-scope map cloning, repeated
closure self-reference walks, and the checker's multiple top-level declaration
passes. They are not prioritized because they were small or absent in the
current vaxis profile. Reconsider them only with a representative benchmark or
profile demonstrating material cost.
