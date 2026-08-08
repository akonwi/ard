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
| COMP-002 | P1 | planned | Make AIR fully typed and Go lowering read-only |
| COMP-003 | P1 | planned | Finish the ADR 0057 AIR representation migration |
| COMP-004 | P0 | planned | Contain module imports within package roots and propagate filesystem errors |
| COMP-005 | P1 | planned | Complete LSP cancellation and dependency-aware invalidation |
| COMP-006 | P1 | planned | Stop silently returning incomplete project-wide references and renames |
| COMP-007 | P2 | planned | Remove the fallback Go importer |
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

The Go backend currently repairs weak local and expression types by recursively
searching function bodies. It can also append `Maybe` types to `Program.Types`
after AIR validation. This duplicates frontend type inference, makes backend
cost depend on repeated tree scans, and weakens AIR's role as the validated
frontend/backend contract.

Bounded outcome:

- Reproduce the existing server-edge fixtures at the AIR level.
- Resolve contextual local, list, `Maybe`, and `Result` types in AIR lowering.
- Make AIR validation reject unresolved types required by targets.
- Remove Go-side recursive type inference and type-table mutation.

## COMP-003: Finish the ADR 0057 AIR representation migration

`TypeReference` is canonical, but AIR and Go lowering still consume parallel
compatibility metadata such as `Param.Mutable`, `ReturnReference`,
`ParamMutable`, and `ElemMutable`. Some separate storage metadata is necessary,
but reference type identity and foreign ABI shape should not compete.

Bounded outcome:

- Define one normalized foreign ABI classification for parameters and results.
- Keep binding mutability and capture modes as storage semantics.
- Remove compatibility fields as consumers migrate.
- Reject contradictory AIR combinations during validation.

## COMP-004: Contain imports and propagate filesystem errors

Ard import tokens permit dots, while module resolution joins unchecked module
segments to a package root. A path containing `..` can escape that root.
Resolution also treats non-`IsNotExist` `Stat` errors as success until a later
read fails.

Bounded outcome:

- Reject empty, `.`, `..`, and absolute module paths.
- Verify cleaned destinations remain inside the owning package root.
- Define and test the policy for symlinks crossing package boundaries.
- Return unexpected filesystem errors at the resolution boundary.

## COMP-005: Complete LSP cancellation and invalidation

Every document change schedules diagnostics for every open document. Running
diagnostics use a background context, semantic requests resynchronize every
overlay, and concurrent cache misses may duplicate complete checks.

Bounded outcome:

- Cancel superseded per-document diagnostic work.
- Make the analysis workspace authoritative for incremental overlay updates.
- Deduplicate concurrent checks for one content signature.
- Track reverse dependencies and schedule only affected open documents.

## COMP-006: Make incomplete reference searches explicit

Project-wide references and rename scan at most 2,000 Ard files and discard
recursive directory-read errors. They can therefore return plausible but
incomplete results without warning.

Bounded outcome:

- Propagate traversal errors.
- Remove the arbitrary cap or return an explicit incomplete-search error.
- Never apply a workspace rename from a known-partial result.

## COMP-007: Remove the fallback Go importer

Checkers created without options use `go/importer`, while production compilation
uses the primed `go/packages` resolver. This leaves tests and production with
different build-context and type-universe behavior.

Bounded outcome:

- Give direct checker tests a shared primed resolver fixture.
- Require a resolver whenever a checked program imports Go.
- Delete `ImporterGoPackageResolver` and its fallback branch.

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
