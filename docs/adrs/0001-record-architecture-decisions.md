# 0001: Record Architecture Decisions

## Status

Accepted

## Context

Ard is a monorepo with compiler, language tooling, website, examples, and editor integration. Architecture and design notes already exist in focused areas such as `compiler/docs/` and `backlog/`, but there is no project-wide convention for recording durable architecture decisions.

Without a consistent decision log, future contributors may need to infer why important choices were made from code, pull requests, or scattered notes.

## Decision

Use Architecture Decision Records (ADRs) for significant project-wide architecture decisions.

ADRs will live in `docs/adrs/` and use sequential filenames:

```text
NNNN-short-title.md
```

Each ADR should use this structure:

- `# NNNN: Title`
- `## Status`
- `## Context`
- `## Decision`
- `## Consequences`
- `## Related`

The initial accepted statuses are `Proposed`, `Accepted`, `Superseded`, and `Deprecated`.

## Consequences

- Important decisions have a stable place to be discovered later.
- Detailed design notes can remain in area-specific docs, with ADRs linking to them when they become durable decisions.
- Contributors should add a new ADR when a change establishes or revises project-wide architecture or long-lived conventions.
- Lightweight implementation details do not need ADRs.

## Related

- `docs/README.md`
- `compiler/docs/`
- `backlog/`
