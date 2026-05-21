# Project Docs

This directory holds project-wide documentation that applies across the Ard monorepo.

## Architecture Decision Records

Architecture Decision Records (ADRs) live in [`adrs/`](./adrs/). Use ADRs to record significant decisions that affect project architecture, long-term conventions, or cross-cutting implementation direction.

Detailed feature design notes can continue to live near the relevant area, such as `compiler/docs/` for compiler internals or `backlog/` for larger future work. When a design note leads to a durable decision, add or update an ADR and link back to the supporting material.

## Adding an ADR

1. Copy the structure from the existing ADRs.
2. Use the next sequential number: `NNNN-short-title.md`.
3. Keep the title as `# NNNN: Title`.
4. Include these sections:
   - `## Status`
   - `## Context`
   - `## Decision`
   - `## Consequences`
   - `## Related`
5. Prefer concise records of the decision and tradeoffs over long design documents.

Common statuses are `Proposed`, `Accepted`, `Superseded`, and `Deprecated`.
