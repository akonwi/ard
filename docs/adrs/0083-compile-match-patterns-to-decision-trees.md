# 0083: Compile Match Patterns to Decision Trees and Match on Tuples

## Status

Accepted. Extends ADR 0077 and ADR 0042. Amends the match diagnostics of
ADR 0052.

## Context

Tuples (ADR 0077) could be destructured but not matched (#573). Deciding on
two values at once meant destructuring first and nesting one match inside
another:

```ard
match left {
  value => {
    match right {
      other => value == other,
      _ => false,
    }
  },
  _ => right.is_none(),
}
```

Supporting tuple subjects needs patterns that nest, and exhaustiveness and
reachability checking over combinations of values. The existing match
implementation could do neither. Each subject kind had its own checker node,
AIR node, and Go lowering (`OptionMatch`, `ResultMatch`, `BoolMatch`,
`EnumMatch`, `IntMatch`, `StrMatch`, `UnionMatch`, `ForeignTypeMatch`), and
each reasoned about one value of one kind. The per-kind code had drifted
apart:

- `Int` arms were stored in maps and emitted sorted, literals before ranges.
  `match 5 { 1..10 => a, 5 => b, _ => c }` ran `b`, although the
  documentation says the first match wins. A leading `_` did not win either,
  for `Int` and enum matches.
- A repeated `Int` literal or `Maybe` binding arm compiled silently, and the
  last one won.
- `Int` and union subjects were evaluated once per arm test, so
  `match next() { ... }` could call `next` several times.
- `ok(1)` panicked the checker. `Result::ok(x)` was rejected, though the
  guide used it. `_` was rejected in `Bool` and `Result` matches.
- Names bound by patterns were not indexed, so hover, definition, and rename
  did not reach them.
- Duplicate arms were errors for some kinds and warnings for others.

Gleam's compiler solves the same problem with one representation for every
pattern, compiled into a decision tree that drives exhaustiveness checking,
reachability checking, and code generation. It follows Jules Jacobs' "How to
compile pattern matching", by way of Yorick Peterse's implementation.

## Decision

### Tuple patterns

A tuple can be the subject of a match. A tuple pattern is a parenthesized,
comma-separated list of patterns, one per element:

```ard
match (left, right) {
  (ok(a), ok(b)) => a + b,
  (err(e), _) => fail(e),
  (_, err(e)) => fail(e),
}

match fetch() {
  (200, body) => handle(body),
  (404, _) => not_found(),
  (code, _) => fail(code),
}
```

- The pattern must have as many elements as the tuple. As with tuple
  literals, `(p)` is grouping, not a one-element tuple.
- Each element pattern is checked against its element's type. Tuple patterns
  nest for tuples inside tuples.
- The subject is any tuple-typed expression. When it is written as a tuple
  literal, like `(left, right)`, its elements are evaluated once each, left
  to right, and no tuple is built unless an arm binds the whole subject.
- Tuple patterns appear only in match arms. `let a, b = value` stays the
  destructuring form.

The alternative considered was Gleam's multiple subjects,
`match left, right { ok(a), ok(b) => ... }`. Ard separates arms with commas
and Gleam does not, so unparenthesized pattern lists read poorly on one line,
and nested tuples would still need parentheses, giving tuples two spellings.

### Patterns nest

Every pattern is one of:

| Pattern | Matches |
| --- | --- |
| `_` | any value |
| a name | any value, and binds it |
| `true`, `false` | a `Bool` |
| `5`, `-5`, `1..10`, `E::v` | an `Int` equal to the literal, in the inclusive range, or equal to the variant's value |
| `'a'` | a `Rune` |
| `"text"` | a `Str` |
| `E::v`, `v` | an enum variant |
| `ok(p)`, `err(p)`, `Result::ok(p)`, `Result::err(p)` | a `Result` whose value matches `p` |
| `some(p)`, `none` | a `Maybe` whose present value matches `p`, or an absent one |
| `T(p)`, `pkg::T(p)` | a union member whose value matches `p` |
| `pkg::T(name)` | an `Any` or Go interface value whose dynamic type is `T` (ADR 0042) |
| `(p1, p2, ...)` | a tuple whose elements match |

The pattern inside a constructor can be any pattern for the value it holds,
so `ok(200)` and `(some(1..9), _)` are valid. Values of other types (structs,
lists, `Float64`, and so on) can be matched only with `_` or a name.

A bare name is resolved by the type it matches: `none` in a `Maybe` position
and an enum variant name in an enum position are patterns; any other name
binds. A name is bound at most once per pattern.

### Existing top-level forms keep their meaning

At the top of an arm, the forms Ard already had keep working:

- In a match on a `Maybe` where some arm is a bare name, that name binds the
  present value and `_` matches the absent one, as before. Without such an
  arm, `_` is an ordinary catch-all.
- A bare `ok` or `err` binds the value under that name.
- A bare union member name, `Str` or `pkg::Type`, binds the value as `it`.

Inside a tuple or constructor pattern these forms would be ambiguous with
bindings, so nested positions spell the value out: `some(_)`, `ok(_)`,
`Str(_)`, or a binding in place of `_`. The checker reports the bare form
there with the spelling to use.

### Semantics

Arms are tried in source order, and the first arm whose pattern matches
runs. The subject is evaluated exactly once, before any arm is tried.

### Decision trees

The checker compiles each match into a decision tree over *match variables*:
the subject, and parts read from it (a tuple element, a `Maybe`'s present
value, a `Result`'s value or error, a union member's value, a dynamic value
narrowed to a foreign type). Each arm starts as a list of checks, "variable is
pattern". The compiler switches on a variable tested by the first remaining
arm and splits every arm across the branches:

- `Bool`, closed enum, `Maybe`, `Result`, and union variables switch on their
  constructors. A branch keeps the arms that name its constructor, with checks
  on the constructor's value in place of the original check, plus the arms
  that do not test the variable. Constructors no arm names share one
  fallback branch.
- `Int`, `Rune`, `Str`, open-enum, and dynamic variables have unbounded
  values, so they get a binary test taken from the first arm: one branch for
  values that pass it and a fallback for the rest. An arm whose test overlaps
  stays in the passing branch with its test; one whose test is covered drops
  out of the fallback. This handles overlapping literals and ranges without
  special cases.
- Tuple checks always hold, so they become checks on the elements.

An arm with no checks left matches, so the first one ends its branch. A
branch with no arms left means a value no arm matches.

- **Exhaustiveness.** A match whose tree has such a branch is an error. The
  diagnostic lists the missing cases as Ard patterns, built from the path to
  each failing branch, for example ``add a case for `(ok(_), err(_))` ``.
- **Reachability.** An arm that ends no branch can never run. It is reported
  as an *unreachable match arm* warning (`unreachable_match_arm`). When one
  earlier arm alone covers it, the warning points at that arm. Repeated arms
  and arms after a catch-all are unreachable arms, so the separate duplicate
  arm diagnostics of match subjects are replaced. They were errors for enum,
  `Bool`, `Str`, and `Rune` matches and warnings for unions and foreign
  types; all are now warnings, because an unreachable arm does not make the
  program unsound. Conditional `match {}` keeps its own diagnostics.

### Pipeline

- **Checker.** One `checkPattern` resolves every pattern against the type of
  its match variable and records each binding for hover, definition, and
  rename. One `PatternMatch` node replaces the per-kind nodes and carries the
  arms in source order, the match variables, and the decision tree.
- **AIR.** `ExprMatch` replaces `ExprMatchEnum`, `ExprMatchInt`,
  `ExprMatchStr`, `ExprMatchUnion`, `ExprMatchForeignType`, and
  `ExprMatchResult`. Its arguments are the subjects and its payload holds the
  match variables (each read from a subject or a parent variable), the
  decision tree with typed tests, and the arms with the locals their
  bindings define. Backends need no pattern logic of their own.
- **Go backend.** The tree becomes nested `if` statements. Match variables
  are pure reads of the subject temporaries (field reads, `Maybe` values,
  type assertions after a passing test), so they are rebuilt where needed
  rather than stored. An arm that ends exactly one branch is emitted at that
  branch. An arm that ends several, typically a trailing catch-all, is
  emitted once after the tree: each of its branches records it in a selector
  that picks its body, so no arm body is duplicated. Arm bindings are made in
  one short declaration, so every value is read before any new name can
  shadow the subject.
- `OptionMatch` and `ExprMatchMaybe` remain only as the internal form of
  `Maybe` field projection (ADR 0066), which the checker synthesizes and no
  user match produces.

### Not included

Alternatives (`p1 | p2`), guards (`p if cond`), and binding a whole
sub-pattern (`p as name`) are left for later. The decision tree has room for
them: Gleam represents guards as a tree node and alternatives as extra rows.
Member access (`pair.0`), tuple equality, tuples as map keys, struct and list
patterns, and tuple patterns outside match arms are not part of this
decision.

## Consequences

- Matches over several values no longer need nesting, and missing
  combinations are reported precisely.
- Programs that relied on the old order of `Int` or enum arms run
  differently: the first matching arm in source order now runs. Every such
  arm is now reported as unreachable, so affected code is easy to find.
  None of the standard library, samples, examples, or Cooper's 106 files
  with matches is affected.
- A match evaluates its subject once, which fixes subjects with side
  effects in `Int` and union matches.
- Some programs that were rejected now compile: `_` in `Bool` and `Result`
  matches, `Result::ok(x)` patterns, and a bare name as a catch-all binding
  in any match. A bare name that is not a `Result` constructor in a `Result`
  match used to be ignored with a warning; it now binds the whole value.
- Non-exhaustive match messages now name the missing patterns, and several
  missing cases are reported in one diagnostic. Duplicate-arm errors became
  unreachable-arm warnings.
- The generated Go for every match changes shape, from `switch` statements
  to nested `if` statements.
- The formatter and the tree-sitter grammar already accepted tuple and
  nested patterns as expressions and need no changes.

## Related

- `docs/adrs/0005-use-result-maybe-and-try-for-error-handling.md`
- `docs/adrs/0012-represent-optional-values-with-maybe.md`
- `docs/adrs/0042-support-dynamic-foreign-type-tests.md`
- `docs/adrs/0052-adopt-structured-labeled-diagnostics.md`
- `docs/adrs/0066-project-maybe-fields-in-try-and-match.md`
- `docs/adrs/0077-add-tuple-types-for-go-multi-result-interop.md`
- `docs/adrs/0078-evaluate-operands-in-source-order.md`
- Jules Jacobs, "How to compile pattern matching"
- Gleam's `compiler-core/src/exhaustiveness.rs`
- `compiler/checker/match.go`, `match_patterns.go`, `match_decision.go`
- `compiler/air/match.go`, `compiler/air/lower_match.go`
- `compiler/go/lower_match.go`
