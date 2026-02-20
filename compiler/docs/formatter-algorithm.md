# Ard Formatter Algorithm (Wadler/Prettier Style)

This document defines the formatter architecture we are moving to.

## Why move

The current formatter prints many constructs directly as strings and uses local width heuristics.
That works for simple cases, but it is hard to keep wrapping behavior consistent across nested constructs.

We are moving to a document-IR based formatter (the same family of algorithms used by Prettier and described in "A prettier printer").

## Core idea

Formatting is split into two phases:

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

## Incremental rollout plan

We will migrate construct families incrementally while keeping the formatter usable at every step.

1. **Engine foundation**
   - Introduce Doc nodes and printer
   - Keep existing parser and command entry points

2. **High-impact constructs**
   - Function parameters and calls
   - List/map/struct literals
   - Trailing commas and multiline wrapping

3. **Block constructs**
   - `if`, loops, `match`, `try`
   - Canonical brace + indentation handling via Doc groups

4. **Comments and trivia**
   - Keep comments conservative initially
   - Later improve parser/trivia attachment and line-suffix behavior

5. **Full migration**
   - Remove legacy string-assembly paths once all syntax is lowered to Doc
   - Keep format idempotence and sample/std_lib regression tests

## Non-goals (current stage)

- Perfect comment placement preservation
- Lossless source reconstruction

Those require richer comment/trivia location data and are planned separately.
