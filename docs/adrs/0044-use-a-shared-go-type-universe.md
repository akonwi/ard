# 0044: Use a Shared Go Type Universe

## Status

Proposed

## Context

Direct Go interop resolves imported package metadata through `GoPackagesResolver` (`compiler/checker/go_packages_resolver.go`), which invokes `packages.Load` once per imported Go package path. Every `packages.Load` call type-checks the requested package and its entire dependency graph into fresh `*types.Package` objects — a distinct go/types object "universe" per load.

go/types compares named types by object identity. When two Ard-visible Go types come from different loads, anything that spans both universes fails even though Go itself accepts it: interface satisfaction, assignability, convertibility, and `types.Identical`. The canonical failure was:

```ard
use go:net/http
use go:github.com/go-chi/chi/v5 as chi

let router = chi::NewRouter()
http::ListenAndServe(addr, router)
// Type mismatch: Expected http::Handler, got mut chi::Mux
```

`*chi.Mux` implements `http.Handler`, but the `http.ResponseWriter` and `*http.Request` named types inside `ServeHTTP`'s signature had different identities in chi's universe than in net/http's.

The current mitigation is cross-universe canonicalization (`goAssignableAcrossUniverses` and `translateGoType` in `compiler/checker/foreign_type.go`): when `types.AssignableTo` fails, the checker structurally re-resolves named types by (package path, name) into the other side's universe and re-checks within a single universe. This is sound — all loads share one project root and build configuration, so an import path always denotes one declaration — but it has structural costs:

- It was a comparison-layer patch. Every future feature that touches go/types (explicit conversions, embedded field promotion, method-set queries, cross-package generic instantiation) would have had to route through the translation layer or silently inherit the identity problem.
- It could not resolve the neither-imports-the-other case: when the interface's package and the implementer's package do not (transitively) import each other, no universe contains both declarations, so translation found no anchor and conservatively rejected code Go accepts.
- Each load re-type-checked shared dependencies (chi's load re-checked net/http), duplicating work and memory.

## Decision

Resolve all of a program's Go imports out of a single go/packages load session, so every Go type the compiler sees shares one object universe.

### A1: Whole-program import pre-scan

Before checking begins, collect every `use go:` path the program will need and issue one batched Go package metadata query for all roots:

- parse the entry module (already done by the frontend);
- walk the Ard import graph shallowly, parsing each reachable `.ard` file only for its `use` statements — no checking — including git-dependency modules the module resolver can already locate;
- union all `go:` paths and prime the resolver with a single batch load.

`GoPackageResolver.ResolveGoPackage(path)` keeps its interface and serves everything from the primed session. All `GoPackage` metadata and `ForeignType.GoType` references are then minted from one universe before any checker binding happens, which is the property the current per-path design cannot provide: today, module A's exported signatures can bake in universe-1 types before module B's imports trigger universe-2.

The production implementation queries the modern Go toolchain once for each
root package's compiler export file, then decodes every root through
`gcexportdata.Read` with one shared imports map. This preserves the single
`go/types` universe without asking `go/packages` to materialize syntax,
expression type information, or dependency package fields Ard does not consume.
External `GOPACKAGESDRIVER` configurations continue to use `packages.Load`, and
missing or incompatible export data falls back to full source loading.

Dependency-module replacements are supplied through a synthesized `-modfile`
rather than `packages.Config.Overlay`. `go/packages` conservatively invalidates
compiler export data for the entire graph whenever any overlay exists, even when
the overlay only changes `go.mod`; using a read-only modfile preserves the exact
locked/path dependency graph while allowing Go's normal export cache to remain
valid. The synthesized module inputs are cached by content solely to give the Go
command a stable modfile path; package metadata itself is never persisted, so Go
remains responsible for source, build-tag, toolchain, and dependency
invalidation. If the consumer's checksums are not yet complete for a replaced
dependency, the resolver discards the readonly attempt and retries the entire
package batch with private writable module files. Go may complete those temporary
files through normal checksum verification, but neither project/dependency files
nor the immutable shared cache are modified.

`use go:` is currently the only mechanism that introduces a Go package path. That single-mechanism property is what makes the pre-scan a complete oracle; any future feature that introduces Go paths outside `use` statements must feed the pre-scan.

### A2: Fail loudly on post-prime misses

A cache miss after priming indicates the pre-scan was incomplete, which is a compiler bug. The resolver reports it as an internal error rather than silently issuing a fresh load into a new universe. (The alternative — reload the union and restart checking — is deferred until a driver supports restart; silently creating a second universe is the one behavior that must never happen.)

Per-package load errors must map back to their paths: one broken import cannot poison the rest of the batch. `packages.Load` reports errors per package, so this is bookkeeping, not architecture.

### A3: LSP session reuse

The LSP snapshot engine re-checks on edits, and edits can add imports. The session becomes a long-lived cache keyed by (go.mod content hash, build tags):

- reused across snapshots while the key is stable;
- fully invalidated when `go.mod` changes or a previously unseen `go:` path appears, then re-primed with the new union;
- guarded by a mutex — the resolver cache is shared across concurrent analysis.

This is where the design pays off most (per-path loads dominate LSP latency today) and costs most (invalidation discipline).

### Relationship to cross-universe translation

Canonicalization ships first and remains correct; this decision supersedes it in stages:

1. Cross-universe translation is the bridge, correct wherever universes overlap.
2. A1 and A2 land behind the unchanged resolver interface. The translation layer stays as a safety net; its tests become regression tests asserting the fallback no longer fires.
3. After A3 is stable, the translation layer (`translateGoType` and helpers) is deleted.

## Consequences

- Go type identity holds program-wide: interface satisfaction, assignability, and equality need no translation layer, and the neither-imports-the-other rejection disappears.
- Total load work and memory shrink — shared dependencies are type-checked once instead of once per importing load — but latency is front-loaded into one batch load. CLI builds benefit immediately; the LSP needs A3's session reuse to avoid regressing first-diagnostic latency.
- The pre-scan becomes a stated invariant: every mechanism that introduces a Go package path must be visible to it.
- A post-prime resolver miss is an internal compiler error, converting silent unsoundness into a reportable bug.
- `translateGoType` and its supporting code (~250 lines plus tests) are deleted once the migration completes.
- Future go/types-adjacent features (conversions, embedded promotion, method-set queries) can rely on plain identity semantics without knowing about universes.

## Related

- `docs/adrs/0028-use-direct-go-imports-for-ffi.md`
- `docs/adrs/0035-use-go-packages-for-ffi-resolution.md`
- `docs/adrs/0039-support-explicit-go-interface-interop.md`
- `docs/adrs/0043-rebuild-lsp-on-snapshot-analysis.md`
- `compiler/checker/go_packages_resolver.go`
- `compiler/checker/foreign_type.go`
- Pull request #261 (cross-universe canonicalization)
