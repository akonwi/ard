# Ard Formatter Algorithm (Wadler/Prettier Style)

This document defines the formatter architecture used by Ard.

## Core idea

Formatting is split into three phases:

1. Parse source to AST
2. Lower AST to a formatting Document IR ("Doc")
3. Print the Doc with a line-fitting algorithm

The Doc describes *choices* (flat vs wrapped) instead of directly writing text.

## Doc nodes

The formatter uses a small set of commands:

- `text("...")`: literal text with no line break
- `concat(...)`: sequence of docs
- `group(doc)`: try to print flat; if it does not fit, print in break mode
- `line`: space in flat mode, newline in break mode
- `softline`: nothing in flat mode, newline in break mode
- `hardline`: always newline
- `indent(doc)`: increase indentation for nested broken lines
- `ifBreak(broken, flat)`: print one branch based on current mode

This lets us model things like trailing commas naturally: `ifBreak(",", "")`.

## Printing algorithm

The printer runs a stack machine with two modes:

- `flat`: prefer one-line output
- `break`: emit newlines at `line`/`softline`

For each `group`, we run a `fits(...)` simulation at the current column:

- If the group fits within `printWidth`, print it in `flat` mode
- Otherwise print it in `break` mode

This makes wrap decisions composable and consistent even for deep nesting.

## Current behavior

The formatter applies Doc-based printing across Ard syntax, including declarations, block constructs, and expression constructs.

- Wrapping decisions are based on `group` + `fits(...)`, not ad hoc per-node width checks.
- Trailing commas are controlled through Doc branching (`ifBreak`).
- Indentation and multiline braces are emitted through Doc indentation and hardline commands.
- Formatting is idempotent and validated against formatter tests and stdlib golden/idempotence coverage.

## Non-goals (current stage)

- Perfect comment placement preservation
- Lossless source reconstruction

Those require richer comment/trivia location data and are planned separately.
