---
name: tree-sitter
description: Tree-sitter grammar and highlighting maintenance for Ard (tree-sitter/grammar.js and tree-sitter/queries/*.scm), including syncing highlights to Zed. Use when updating the grammar, fixing tree-sitter highlight errors, or revising Ard syntax highlighting queries.
---

# Tree Sitter (Ard)

## Overview

Maintain the Ard Tree-sitter grammar and highlight queries, validate with `npm run generate` and `tree-sitter highlight`, and sync highlights into the Zed extension.

## Workflow

1) Grammar changes
- Edit `tree-sitter/grammar.js`.
- Run `cd tree-sitter && npm run generate`.
- If generation warns about conflicts, keep the list minimal and document in commit if needed.
- Avoid recursive cycles like `primary_expression -> member_expression -> primary_expression`, which can cause slow or non-terminating parses.

2) Highlight queries
- Edit `tree-sitter/queries/highlights.scm` first.
- Validate with `tree-sitter highlight path/to/file.ard --quiet`.
- If you see “Invalid node type X”, check `tree-sitter/src/node-types.json` to confirm the node exists.
- If you see “Impossible pattern”, simplify the query (avoid deep nested patterns on ambiguous nodes).

3) Zed sync
- Copy `tree-sitter/queries/highlights.scm` to `zed-plugin/languages/ard/highlights.scm`.
- Keep Zed queries as a strict subset of Tree-sitter queries when stability is uncertain.

## CLI setup notes

- Ensure `~/.config/tree-sitter/config.json` includes the repo path (e.g., `/Users/akonwi/Developer/agent/ard`), or pass `--scope source.ard`.
- If `tree-sitter highlight` uses the wrong grammar, remove conflicting tree-sitter repos or adjust config order.
---
