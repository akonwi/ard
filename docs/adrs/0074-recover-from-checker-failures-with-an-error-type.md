# 0074: Recover From Checker Failures With an Error Type

## Status

Accepted. Partially supersedes ADR 0007.

## Context

ADR 0007 asked the checker to recover from local errors with placeholder
values, and to *halt* after critical errors (undefined variables were the
example) so that later checks would not cascade. In practice:

- **Halting hid real errors.** `c.halted` made every later expression and
  statement check return `nil`, for the rest of the module. One undefined
  variable silenced every diagnostic after it, in the CLI and in the LSP
  (#523). Untyped empty `[]` / `[:]` literals halted in the same way.
- **Failures still cascaded.** A failed expression is `nil`. Most consumers
  already handle `nil` silently, but a few turn it into misleading follow-on
  diagnostics:
  - `Block.Type()` skipped failed statements, so a block whose final
    expression failed took the type of an earlier statement or `Void`. The
    enclosing function, closure, or match then reported a spurious
    mismatch such as "this expression has type `Void`" (#522).
  - A `let`/`mut` whose initializer failed was never bound, so every later
    use reported `Undefined variable`.
  - The empty-collection placeholder `[Void]` spread into unrelated
    mismatches whenever the halt did not hide them.

The checker had no way to say "this already failed and was reported".

## Decision

Represent an already reported failure explicitly, and never halt.

### The error type

Add a checker-only error type (`invalidType`, displayed as `<unknown>`),
modeled on the checker-only `Never` type:

- It is compatible with every type in both directions (`areCompatible`), it
  gives no evidence to generic unification, and it is ignored when merging
  branch result types, the same way `Never` is.
- It is only produced alongside an error diagnostic, so it never reaches AIR:
  lowering does not run for programs with errors.
- No checked expression carries it, even nested inside a composite type such
  as the inferred return type of a closure whose body failed.
  `checkExpr` turns such a result into `nil`, which stays the checker's single
  "failed expression" result. The error type appears in only two places:

1. **Blocks.** When a block's final value statement produces no checked
   result *and* fails (reports an error, or silently propagates an earlier
   one), the block's type is the error type. Return-type and branch checks
   against it pass silently. A block that ends in a declaration, assignment,
   or loop is unaffected and keeps its `Void` type.
2. **Bindings.** A `let`/`mut` whose initializer fails still binds its name:
   - with its declared type when it has an annotation, which is still
     trustworthy and keeps later uses fully checked
   - with the error type otherwise. References, calls, and assignments
     through that name then fail silently.

### Other recoverable failures

- An untyped empty `[]` / `[:]` reports its diagnostic and fails like any
  other expression, instead of returning a `[Void]` placeholder.
- A construct whose type can only be inferred from something that already
  failed is itself a failed expression, rather than reporting an unresolved
  generic:
  - a generic struct literal whose type arguments depend on a failed field
    value
  - a callback whose return type is inferred from a failed body, for example
    `res.map(fn(v) { missing })`. A callback with a known return type keeps
    it.

### No halting

Remove `halted`. Every error is reported and checking continues, so a module
reports all of its independent errors in one pass.

## Consequences

- One root error produces one diagnostic. The tests that recorded
  follow-on `Void` mismatches now expect only the root diagnostic.
- Later declarations and statements are always checked. The LSP keeps
  showing diagnostics below the first error.
- A failed binding stays resolvable for hover and navigation. Its type is
  shown as `<unknown>` unless it was annotated.
- A checker path that wants a new kind of silent failure must count it with
  `failSilently()` so enclosing blocks can tell a propagated failure from a
  successful check.
- New consumers of `Block.Type()` or `Symbol.Type` must treat the error type
  as "already reported". Using `areCompatible` and `commonResultType` does
  this automatically.
- ADR 0007's placeholder approach to recovery still applies. Its rule that
  critical errors halt checking is replaced by this ADR.

## Related

- `docs/adrs/0007-use-explicit-checker-error-recovery.md`
- #522, #523, #525
