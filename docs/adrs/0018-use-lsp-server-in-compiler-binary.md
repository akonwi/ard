# 0018: Use LSP Server in Compiler Binary

## Status

Accepted

## Context

Ard has a working compiler pipeline (parse → checker → AIR → codegen), a formatter, and file-based modules. Developers currently get feedback only by running `ard check`, `ard run`, or `ard test` from the terminal. There is no editor integration that surfaces diagnostics, completions, hover type info, or go-to-definition while editing.

Adding editor support requires implementing the Language Server Protocol (LSP), the industry standard for editor-agnostic language intelligence. The LSP server needs to:

- Reuse the existing compiler pipeline (parser, type checker) without duplicating logic.
- Support incremental document changes — re-parsing and re-checking only the affected file(s) when possible.
- Communicate over stdin/stdout via JSON-RPC 2.0, the standard LSP transport.
- Be bundled into the `ard` binary as a subcommand (`ard lsp`) so users get it with a single install.

### Go LSP library landscape

The LSP protocol is JSON-RPC 2.0 with a well-defined message schema. Several Go options exist:

| Library | Description |
|---------|-------------|
| `go.lsp.dev/protocol` | Typed struct definitions for every LSP request, notification, and type. Matches the spec closely. Maintained by the spec author's ecosystem. |
| `go.lsp.dev/jsonrpc2` | JSON-RPC 2.0 transport layer (connections, handlers, cancellation). Pairs with `go.lsp.dev/protocol`. |
| `go.lsp.dev/uri` | Document URI parsing and file-path conversion. |
| Build from stdlib | LSP is just JSON-RPC 2.0 over stdin/stdout. Could use `encoding/json` directly. |
| Sourcegraph `go-lsp` | Older LSP type definitions. Less maintained and predates the current spec iteration. |

`go.lsp.dev/protocol` is the most complete and well-maintained option. It provides correctness guarantees by matching the spec schema and eliminates the need to define all LSP types by hand. The project can adopt `go.lsp.dev/protocol` for type definitions and build a lightweight JSON-RPC 2.0 dispatcher using the stdlib `net/rpc/jsonrpc` or a minimal custom handler, avoiding a hard dependency on `go.lsp.dev/jsonrpc2` if the abstraction feels heavy.

### Constraints

- The compiler currently has one-shot pipeline: read file → parse → check → lower → codegen. The LSP needs a persistent server that incrementally re-checks documents.
- The checker already supports error recovery (ADR 0007), which is essential for producing diagnostics on incomplete or partially-edited code.
- The module resolver and project root detection already exist in `checker/` and can be reused.
- No concurrency model exists in the compiler today; the LSP server must introduce thread-safe document state.

## Decision

### 1. Create a new `compiler/lsp/` package

A new `lsp` package will contain the LSP server implementation. It is a new top-level package alongside `frontend/`, `checker/`, `air/`, `backend/`, etc.

### 2. Expose via `ard lsp` subcommand in `main.go`

The existing CLI gains an `lsp` subcommand that starts the LSP server:

```ard
ard lsp
```

The server reads and writes JSON-RPC 2.0 messages on stdin/stdout. This is the standard LSP transport and works with all LSP-compatible editors.

### 3. Use `go.lsp.dev/protocol` for LSP type definitions

Adopt `go.lsp.dev/protocol` to provide typed Go structs for every LSP message type. This eliminates the maintenance burden of hand-defining LSP types and ensures correctness against the protocol spec.

JSON-RPC 2.0 transport and connection management will use a lightweight custom dispatcher built on the standard library's `encoding/json`. If the concurrency and handler abstractions in `go.lsp.dev/jsonrpc2` prove worthwhile, it may be added later without structural changes since the protocol types are the same.

### 4. Architecture

```
                    ┌─────────────────────┐
                    │     LSP Client       │
                    │  (editor / IDE)      │
                    └──────┬───┬───────────┘
                           │   │  JSON-RPC 2.0
                           │   │  (stdin/stdout)
                    ┌──────┴───┴───────────┐
                    │   lsp.Server         │
                    │   ┌───────────────┐  │
                    │   │  Handler map  │  │
                    │   │ (method → fn) │  │
                    │   └───────┬───────┘  │
                    │           │          │
                    │   ┌───────┴───────┐  │
                    │   │  Document     │  │
                    │   │  Cache        │  │
                    │   │ (uri → doc)   │  │
                    │   └───────┬───────┘  │
                    │           │          │
                    │   ┌───────┴───────┐  │
                    │   │  Compiler     │  │
                    │   │  Pipeline     │  │
                    │   │ (parse→check) │  │
                    │   └───────────────┘  │
                    └─────────────────────┘
```

**Document Cache**: An in-memory map from document URI to parsed state. Each entry holds the source text, parsed AST, and checked module. On `textDocument/didOpen` and `didChange`, the cache entry is updated and a re-check is scheduled.

**Diagnostics Publishing**: After each re-check, diagnostics are published to the client via `textDocument/publishDiagnostics` notification. The checker's existing diagnostic format is converted to LSP `Diagnostic` types.

**Project Awareness**: The server detects the project root using `checker.FindProjectRoot` at startup. Modules outside the project root are handled as standalone files with limited cross-module analysis.

### 5. Initial LSP capabilities

| Capability | LSP Method | Notes |
|---|---|---|
| Document sync (full) | `textDocument/didOpen`, `didChange`, `didSave`, `didClose` | Start with full-sync; incremental sync can follow |
| Diagnostics | `textDocument/publishDiagnostics` | Push on change; reuse checker diagnostics |
| Hover | `textDocument/hover` | Show type signature and doc comments |
| Go to Definition | `textDocument/definition` | Navigate to symbol declaration |
| Find References | `textDocument/references` | Search workspace for symbol usages |
| Document Symbols | `textDocument/documentSymbol` | Outline of functions, types, variables |
| Completion | `textDocument/completion` | Basic keyword and symbol name completion |
| Formatting | `textDocument/formatting` | Reuse existing `formatter.Format()` |
| Signature Help | `textDocument/signatureHelp` | Show function parameter info on `(` |

### 6. Incremental check model

On each document change:

1. Update the source text in the document cache.
2. Re-parse the changed file to produce a new AST.
3. Re-run the checker on the module containing the changed file.
4. Convert checker diagnostics to LSP diagnostics and publish.
5. If the change affects other modules (e.g., a public API signature changed), those modules are scheduled for re-check.

The initial model uses full re-check of the affected module rather than fine-grained incremental checking. This is simpler to implement correctly and is fast enough for typical Ard projects. Fine-grained incremental checking (reusing previous checker state) can be pursued later if latency becomes a concern.

### 7. Concurrency

The LSP server manages concurrency with a simple model:

- A single goroutine processes incoming requests sequentially (LSP does not require parallelism for correctness).
- Document cache access is guarded by a mutex.
- Diagnostics are published on the same goroutine after each check cycle.
- Long-running operations (e.g., full workspace symbol search) can be offloaded to a worker pool if needed, but the initial implementation stays single-threaded for simplicity.

### 8. Go dependency

The `go.lsp.dev/protocol` dependency is added to `go.mod`. It is a pure-Go, zero-dependency type definitions package. This keeps the compiler's dependency footprint minimal.

## Consequences

- **Positive**: Developers get real-time editor feedback (diagnostics, hover, go-to-definition) using a single `ard` binary install.
- **Positive**: The LSP server reuses the existing parser and checker, avoiding a separate analysis codebase.
- **Positive**: The existing formatter is immediately available document formatting.
- **Positive**: The project gains a concurrency and document-state foundation that can serve future tooling (e.g., a `ard repl` with live checking).
- **Negative**: The initial incremental model re-checks the whole module on each edit. For large files or deep module trees, this may introduce latency. Monitoring and optimization (e.g., incremental checker state) is deferred until profiling shows the need.
- **Negative**: Adding `go.lsp.dev/protocol` introduces one new Go dependency to the compiler. It is a lightweight, zero-dependency package, so the impact is minimal.
- **Negative**: The LSP server must be maintained alongside the compiler pipeline. API changes in `parse` or `checker` may require updating the LSP handler code.
- **Risk**: Single-threaded request processing may cause noticeable latency for expensive operations (full workspace symbol search). Mitigation: start single-threaded and add a worker pool if needed.

## Related

- ADR 0007: Use Explicit Checker Error Recovery — error recovery enables diagnostics on incomplete code.
- ADR 0004: Use Canonical Ard Formatting — the formatter reused by `textDocument/formatting`.
- ADR 0013: Use File-Based Modules and Absolute Imports — module resolution reused by the LSP workspace.
- `compiler/frontend/load.go` — the current one-shot load pipeline that the LSP will adapt for incremental use.
- `compiler/checker/checker.go` — the checker whose diagnostics will become LSP diagnostics.
- `compiler/main.go` — will gain the `ard lsp` subcommand.
- `go.lsp.dev/protocol` — LSP type definitions.
