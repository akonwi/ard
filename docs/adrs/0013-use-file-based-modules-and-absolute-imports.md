# 0013: Use File-Based Modules and Absolute Imports

## Status

Accepted

## Context

Ard needs a module system that keeps source organization simple and imports easy to resolve. The language also needs clear visibility rules so modules can expose constants and declarations without accidentally exposing mutable state.

A project may have an `ard.toml` manifest with a project name. Standard library modules live under the `ard/*` namespace.

## Decision

Treat each `.ard` file as a module. A file can be a complete program or can be imported by another module.

Imports use the `use` keyword at the top of a file:

```ard
use ard/io
use my_project/utils as helpers
```

Imported modules are referenced by the last path segment by default, or by an explicit alias when `as name` is provided.

Import paths are absolute from the project root and omit the `.ard` file extension. Ard does not support relative imports.

The project root is determined by `ard.toml` when present. If no manifest is present, the project name defaults to the root directory name.

Visibility rules are:

- functions, structs, enums, and traits are public by default
- `private` makes those declarations accessible only within the same file
- immutable `let` variables are public
- mutable `mut` variables are private module state
- struct fields are public by default
- methods are public by default and can be made private with `private`

## Consequences

- Module resolution remains predictable and project-root based.
- Source files map directly to modules, simplifying compiler and tooling behavior.
- Import aliases provide local naming control without changing module identity.
- Mutable module state is not exposed across module boundaries by default.
- The lack of relative imports avoids ambiguity but requires project-root-oriented paths.
- Future dependency management must preserve or intentionally extend the absolute import model.

## Related

- `backlog/ard-dependency-system/README.md`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
