# 0043: Rebuild the LSP on Snapshot-Based Analysis

## Status

Accepted

Implementation status: the engine (Workspace/Snapshot/memoized parse and
check, shared Go resolver), checker span table, and the hover, definition,
references, document-highlight, rename (local symbols), and member-completion
features are live on the snapshot/span path with legacy heuristics as
fallback. Remaining migration work: cross-file rename on the span path,
signature-help/static-completion/document-symbol/code-action ports, context
cancellation inside Analyze, trimming FileAnalysis to stop retaining whole
Checker instances in the bounded cache, and deletion of the legacy paths at
cutover (each legacy test scenario needs a span-path equivalent first).

## Context

The current LSP implementation (`compiler/lsp`, ~9.8k lines) grew feature-by-feature without a shared analysis layer. Reading the code confirms structural problems:

- **No caching.** `parseAndCache()` re-parses on every request. Hover alone constructs a fresh `ModuleResolver` + `GoPackagesResolver` and runs a full `checker.Check()` in three separate places. `go/packages` loads are the most expensive operation in the compiler (hundreds of milliseconds to seconds for projects with real Go dependencies) and are repeated per request, per feature.
- **Five bespoke resolution engines.** Hover (~2.5k lines), definition, references, document highlight, and rename each re-implement position-to-symbol resolution as heuristic parse-AST walks. They drift from the checker's real semantics; hover test failures are structural, not incidental.
- **Partial resilience.** `dispatch` and diagnostics recover panics, but feature requests run in bare goroutines whose panics can kill the process. There is no watchdog, no log destination, and no self-recovery story. A crashed LSP is a terrible user experience.

Requirements for the replacement:

1. Feature parity with the current server.
2. Resilience: the server must not crash, and must recover when analysis fails.
3. Performance: no naive re-parse/re-check of everything per change; interactive operations must be fast, and nothing should take more than a few seconds.

## Decision

Rebuild the LSP in three layers: transport, snapshot-based analysis engine, and checker-backed semantic resolution.

```text
lsp/            transport: jsonrpc2, lifecycle, doc sync, request routing, panic guards
analysis/       engine: Workspace -> immutable Snapshot, memoized parse/check, cancellation
checker (+API)  semantic source of truth: position-indexed symbol/reference table
```

### Feature parity list

initialize/shutdown/exit, didOpen/didChange/didSave/didClose, published diagnostics (debounced), hover, definition, references, document symbols, completion (including import completion), formatting, code actions, signature help, document highlight, rename and prepare-rename.

### Snapshot engine

- A **Workspace** holds open-document overlays plus disk state. Every change bumps a revision and yields an immutable **Snapshot**.
- Feature requests run against a snapshot. They never mutate shared state and never re-parse.
- Memoization is keyed by content hash:
  - **Parse layer**: file content hash → parsed AST. One keystroke re-parses one file.
  - **Check layer**: module + transitive dependency hashes → checked result. Editing a module does not re-check unrelated modules.
  - **Go metadata layer**: one long-lived `GoPackagesResolver` per project, invalidated only when `go.mod`, `go.sum`, or `ffi/` change.
- **Debounce and cancellation**: document changes schedule a re-check after a short debounce (~150–200ms); a newer revision cancels in-flight checks via context. Stale diagnostics are discarded by revision comparison.

### Checker as the single resolution engine

Add an opt-in `CheckOptions` mode in which the checker records a position-indexed table mapping source spans to resolved entities (variable definitions, functions, struct fields, module references, foreign symbols, types). Features consume this table instead of re-deriving semantics:

- hover: span lookup → render type/signature
- definition: span lookup → definition location
- references / highlight / rename: inverted index over the same table
- signature help: enclosing call span → function definition

One resolution path serves five features, so parity bugs from drift disappear by construction. The mode is off outside the LSP, so compile performance is unaffected.

### Resilience contract

- `recover()` at every goroutine boundary: per-request wrappers, diagnostics workers, and background check tasks. A panic yields an LSP internal error for that request and a log entry; the server keeps running.
- A panic in the check layer poisons only that snapshot's memo entry; the next revision rebuilds cleanly, giving self-recovery without restart.
- Structured logging to a file so failures are diagnosable.
- Never exit the process outside the `exit` notification; malformed input gets JSON-RPC errors.
- A watchdog cancels any single request exceeding a deadline (~5s) and replies with an error rather than hanging the editor.

### Performance budget

| Operation | Target |
| --- | --- |
| didChange → parse | < 10ms |
| didChange → diagnostics published | < 500ms after debounce (warm Go cache) |
| hover / definition / highlight | < 50ms |
| completion / signature help | < 100ms |
| references / rename (project-wide) | < 1s |
| cold start (first go/packages load) | seconds, once, async; editor stays responsive |

### Migration plan

1. **Phase 0 – parity harness.** Extract the existing `server_test.go` scenarios into a transport-level golden suite that can run against either implementation. Triage the currently failing hover expectation first.
2. **Phase 1 – engine and lifecycle.** New `analysis` package (Workspace, Snapshot, memoized parse/check, shared Go resolver, debounce/cancel). Wire transport, document sync, diagnostics, and formatting.
3. **Phase 2 – checker span table.** `CheckOptions` recording mode plus tests in `checker`.
4. **Phase 3 – features on the table.** Port in order of leverage: definition → hover → highlight/references/rename → signature help → completion → document symbols → code actions → import completion. Each feature ports with its golden tests.
5. **Phase 4 – hardening.** Panic-injection tests, stale-revision races, watchdog, load test against `examples/vaxis-demo` (real Go dependencies).
6. **Phase 5 – cutover.** Replace the old implementation and keep the golden suite.

## Consequences

- The LSP becomes crash-resistant by construction, with self-healing analysis state.
- Interactive latency stops scaling with project size per keystroke: one file re-parses, checks are incremental and debounced, and Go package metadata is loaded once per project.
- Feature code shrinks substantially because position resolution is shared through the checker's span table instead of five bespoke AST walks.
- The checker gains an LSP-facing responsibility (span recording) guarded behind an option; it becomes the single source of semantic truth for tooling.
- During migration both implementations coexist briefly; the golden suite defines parity.

## Related

- `docs/adrs/0018-use-lsp-server-in-compiler-binary.md`
- `docs/adrs/0035-use-go-packages-for-ffi-resolution.md`
- `compiler/lsp`
- `compiler/checker`
