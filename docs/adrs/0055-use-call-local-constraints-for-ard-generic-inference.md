# 0055: Use Call-Local Constraints for Ard Generic Inference

## Status

Accepted

## Context

ADR 0010 requires every generic function call to use fresh type variables, permits inference from arguments and usage context, and rejects conflicting bindings. The checker implements those semantics today, but generic call resolution is distributed across several paths:

- fresh generic setup;
- generic receiver binding;
- explicit type arguments;
- argument checking and unification;
- expected-return inference;
- a later generic resolution pass;
- contextual special cases for `Maybe` and `Result` constructors.

The ordering and ownership rules differ between ordinary functions, static functions, methods, function values, and builtin constructors. Explicit type arguments on ordinary calls may be applied after context-sensitive arguments have already been checked. Some calls are copied and unified more than once using separate fresh generic scopes.

Issue #322 added expected-return inference for direct contextual expressions:

```ard
struct Key<$T> {}

fn Key::new() Key<$T> {
  Key{}
}

let key: Key<Str> = Key::new()
```

The annotation supplies the only evidence for `$T`, and the checker now retains the resulting specialization for AIR.

The same information is available when a generic call appears as another call's argument:

```ard
fn consume(key: Key<Str>) {}

consume(Key::new())
```

The parameter type `Key<Str>` should provide the expected return type for `Key::new()`. Broadly routing nested calls through expected-parameter checking is not safe under the current architecture, however. It caused regressions in independently generic higher-order combinators such as:

```ard
decode::field(
  "name",
  decode::nullable(decode::string),
)
```

with signatures shaped like:

```ard
fn nullable(decoder: fn(Any) $T!Error) fn(Any) $T?!Error
fn field(name: Str, decoder: fn(Any) $T!Error) fn(Any) $T!Error
```

The inner call must first infer its own `$T` from `decode::string`. Passing an unresolved outer parameter pattern inward too early allowed separate fresh generic scopes to interact before either call had stabilized.

The checker also lacks one finalization point that proves an executable Ard call is fully specialized before AIR. Constraint conflicts retain insufficient provenance to consistently explain which receiver, explicit type argument, value argument, enclosing parameter, or expected return established each incompatible type.

Imported Go generic functions use different assignability, pointer, type-set, and constraint rules. Their inference is related but should not be forced into the Ard call model.

## Decision

Ard generic function calls will use one call-local constraint pipeline. Each Ard call owns its fresh inference variables, constraints, checked arguments, and final specialized function definition.

A nested call must not reuse or mutate the inference variables owned by its enclosing call. Relationships between calls are communicated through immutable solver-variable references and exported equations, never shared `*TypeVar` pointers.

Each call has a stable call identity. A variable reference crossing a call boundary identifies `(call ID, declaration-scoped generic ID)`. The parent solver owns a bounded worklist for its nested call subtree. A child returns its checked result and any equations involving immutable variable references; it does not directly bind a parent variable. The parent resolves exported equations and resumes children as additional information becomes available.

The worklist performs an occurs check before binding a variable to a type containing that same variable, reports cyclic inference equations, and terminates when no binding or deferred argument changes. A call cannot finalize while a reachable variable owned by that call remains unresolved.

The checker remains responsible for completing inference. AIR consumes concrete call-site bindings and does not infer missing types.

### Call-local inference state

The implementation will introduce a call-local abstraction conceptually similar to:

```go
type CallInference struct {
    Source      *FunctionDef
    Fresh       *FunctionDef
    Variables   map[GenericID]*InferenceVariable
    Constraints []CallConstraint
    Arguments   []CheckedArgument
}
```

The exact Go API may differ, but it must preserve these invariants:

- every call receives fresh variables;
- variables have call- and declaration-scoped identity rather than only names such as `$T`;
- the shared source `FunctionDef` is never mutated by call-site inference;
- one call produces at most one final specialized `FunctionDef`;
- independently generic nested calls do not share mutable inference state;
- all binding sources are validated as constraints rather than silently overwriting earlier bindings;
- nominal struct applications unify by canonical declaration identity and ordered type arguments, never by pointer identity, type name alone, or declaration-field traversal.

Receiver generic IDs remain owned conceptually by the receiver declaration, but each method call creates fresh local inference slots for those IDs. A receiver value constrains the call-local slots; no mutable receiver application or source method state is reused.

### Constraint and obligation kinds

The pipeline distinguishes three relationships:

1. **Unification constraints** may bind call-owned variables by matching type structure.
2. **Compatibility obligations** validate substituted types but do not bind variables unless an explicit inference rule authorizes it.
3. **Conversion obligations** define their inference contribution independently and materialize a coercion only after solving.

“All constraints agree” therefore means that unification produces one consistent binding for each variable and that all compatibility and conversion obligations accept the substituted result. General compatibility is not used as an implicit source of generic bindings.

### Constraint sources

Constraints retain their semantic source and source span. Sources include:

- the receiver type of a method call;
- an explicit type argument;
- a supplied value argument and its declared parameter;
- an enclosing parameter used as context for a nested call;
- the expected return type supplied by an annotation, assignment, return, branch, or other contextual expression.

Constraint ordering controls when information becomes available for checking. It does not establish a precedence rule that permits one source to overwrite another. All applicable constraints must agree.

For example:

```ard
fn identity(value: $T) $T {
  value
}

let value: Str = identity(1)
```

The argument constrains `$T` to `Int`. The annotation adds an incompatible `Str` return constraint and must produce a conflict rather than rebinding `$T`.

Conflict diagnostics identify both sources using the structured labeled diagnostic model from ADR 0052. Constraints have deterministic phase and source ordering. The newly rejected constraint is the primary label; the earliest incompatible constraint that established the binding is related. Parameter or declaration spans may be additional related labels. The checker emits one root conflict for each connected variable/equation component and suppresses derivative mismatches caused by that conflict. This policy does not require stable diagnostic codes.

### Inference phases

An Ard call is resolved in this semantic order:

1. Resolve the callable and create fresh call-local variables.
2. Add receiver-derived constraints.
3. Add explicit type-argument constraints.
4. Process supplied arguments in source order.
5. Independently check an argument when possible, then classify its relationship using the checked argument type and currently substituted parameter type.
6. Record context-dependent arguments as deferred; contextually check and classify them only when receiver, explicit, earlier-argument, or enclosing expected-context evidence makes the parameter sufficiently concrete.
7. Recompute provisional classifications after substitution when a newly solved type changes the applicable structural or conversion relationship.
8. Add the call's expected-return constraint.
9. Solve unification constraints and exported nested-call equations without overwriting bindings.
10. Validate compatibility and conversion obligations against substituted types.
11. Materialize accepted coercions and enforce mutability and addressability.
12. Synthesize omitted arguments using solved parameter types.
13. Require complete specialization.
14. Produce one specialized `FunctionDef` carrying the completed bindings.

An implementation may use a worklist rather than literal separate loops, but it must preserve the observable ordering and ownership rules. Ordinary later arguments do not retroactively provide context to an earlier deferred argument. This preserves ADR 0010's left-to-right refinement rule: later arguments and callbacks may observe bindings from earlier arguments, but not the reverse.

### Deferred contextual arguments

Some expressions need a sufficiently concrete expected parameter type before they can be checked accurately. These include:

- anonymous functions whose parameter types are inferred;
- empty lists and maps;
- generic calls whose type variables occur only in their return type;
- other existing context-sensitive literal or constructor forms.

Explicit type arguments and receiver constraints are installed before these expressions are checked.

Ordinary nested calls that can infer from their own arguments are checked in their own solver first. The enclosing parameter context then validates the nested result or supplies bindings that remain unresolved. This preserves higher-order composition such as:

```ard
decode::field("name", decode::nullable(decode::string))
```

while enabling:

```ard
consume(Key::new())
```

If both enclosing and nested calls remain underconstrained, the child exports equations over immutable `(call ID, generic ID)` references. The parent subtree worklist owns and resolves those equations. Neither solver receives the other's mutable `TypeVar`, and neither call finalizes until its reachable call-owned variables are solved or diagnosed. Cyclic equations are subject to the same occurs check and bounded-worklist termination rules.

### Explicit and receiver-derived bindings

Explicit type arguments are constraints available before argument checking:

```ard
make<Int>([])
apply<Str>(fn(value) { value })
```

This allows empty collections and anonymous function parameters to observe the explicit specialization.

Method receiver bindings use the same pipeline. A generic receiver supplies constraints for the method's receiver-owned parameters before value arguments are checked. Explicit arguments, receiver bindings, value arguments, and contextual types must agree.

Methods continue to follow ADR 0010: they may use receiver-declared generics but do not introduce independent method-owned generic parameters.

### Completeness and specialization

No checked call may reach AIR with unresolved call-owned variables. At finalization, no type reachable from the specialized signature, `GenericBindings`, checked or synthesized arguments, result type, or coercion nodes may contain an inference variable owned by that call. The checker emits an uninferred-generic diagnostic at the call site when the worklist is exhausted and any such variable remains open.

Calls in an enclosing generic definition may specialize to types containing the enclosing definition's variables. Those variables belong to the enclosing declaration rather than the call and are not considered unresolved call-owned inference variables.

Repeated calls to the same declaration may produce different specializations without modifying or sharing mutable state through the source definition.

### Compatibility and coercion

Conversion obligations contribute only explicitly defined inference evidence:

- For `T` passed to `Maybe<$U>`, infer `$U` from `T`, then materialize one `Maybe` wrapping conversion. An already-`Maybe<T>` argument follows ordinary structural unification and is not wrapped again.
- For `fn(P...) R` passed to `fn(P...) Void`, infer and validate parameter types and mutability while deliberately excluding the source return from equality; materialize a result-discarding adapter after solving.
- Foreign interface conversion validates implementation after substitution. It does not infer a generic merely from the broad interface target unless an existing explicit interface-inference rule supplies evidence.
- Named scalar and other FFI conversions contribute inference according to their existing directional conversion rules, not general compatibility.
- Mutable-reference and addressability checks run after type inference and can still reject an otherwise solved call.

A conversion must not retroactively replace an incompatible binding. Compatibility-only obligations never bind variables.

### Builtin constructors

`Maybe::new`, `Result::ok`, and `Result::err` currently contain bespoke contextual behavior. The initial implementation may adapt those constructors to the call-local pipeline without immediately removing every builtin-specific checker path.

Their existing behavior must be preserved. From the initial implementation, every builtin adapter must still guarantee call-local definitions without shared mutation, complete call-owned bindings before AIR, the same explicit/argument/context conflict policy, consistent nested parameter context, and concrete solved types in synthesized omitted arguments. Migrating their internals fully onto shared inference machinery remains optional.

### Go generic calls

Imported Go generic inference remains separate. Go calls use Go assignability, type parameter constraints, type sets, pointer rules, and foreign ABI metadata that do not map directly to Ard generic functions.

Low-level type traversal or diagnostic utilities may be shared, but this ADR does not require Go generic calls to use the Ard call-local solver.

## Consequences

- Calls such as `consume(Key::new())` can infer return-only generics from the enclosing parameter.
- Explicit type arguments are visible while checking closures and empty collections.
- Receiver, explicit, argument, parameter-context, and expected-return evidence follows one deterministic model.
- Higher-order generic combinators retain independent call-local scopes.
- Conflicts can identify both the source that established a binding and the incompatible constraint.
- Shared generic definitions are not mutated during specialization.
- AIR receives completed checker bindings rather than compensating for missing inference.
- Existing duplicated call-resolution paths must be consolidated, making this a substantial checker refactor rather than a local feature addition.
- Deferred argument checking or a bounded worklist adds implementation complexity.
- Some builtin constructor paths may remain adapters during the initial migration.
- Go generic inference remains intentionally separate.

## Non-goals

This decision does not:

- introduce global Hindley-Milner inference;
- solve constraints across an entire module or unrestricted expression tree in one global solver; bounded solving across one call and its nested-call subtree is part of this decision;
- infer types in AIR or a backend;
- change Ard's `$T` generic syntax;
- add trait bounds or new generic constraints;
- change generic struct application semantics from ADR 0054;
- unify imported Go generic inference with Ard inference;
- introduce independent method-owned generic parameters.

## Implementation Plan

The implementation will land as one pull request while remaining internally staged. These stages are non-mergeable implementation checkpoints: focused tests must pass at each checkpoint, while the completed pull request must satisfy every global invariant in this ADR.

1. Introduce call-local state, immutable variable identities, obligation kinds, and provenance behind existing call entry points without changing inference behavior.
2. Move fresh-variable creation and final specialization into the call-local abstraction.
3. Move receiver-derived and explicit type-argument bindings into the pipeline.
4. Move ordinary left-to-right argument inference into the pipeline and remove the duplicate generic resolution pass.
5. Move expected-return inference from issue #322 into the pipeline.
6. Adapt contextual `Maybe` and `Result` constructors to satisfy call ownership, conflict, and completeness invariants.
7. Add completeness enforcement and deterministic conflict reporting, preserving currently accepted programs except calls that retain unresolved call-owned variables and should already have failed before AIR.
8. Add bounded nested-call equations and deferred checking required for closures, empty collections, and return-only nested calls.
9. Enable nested parameter-context inference only after decoder regressions and call-scope isolation tests pass.
10. Validate every call form and generic-heavy downstream project before merge.

The pull request does not need to expose each stage as a separate commit, but the implementation should preserve these boundaries so nested inference is not layered on top of the duplicated legacy paths.

## Validation

At minimum, implementation must cover:

- direct expected-return inference from an annotated binding;
- return-only inference from an enclosing function parameter;
- nested `Maybe`, `Result`, list, map, fixed-array, function, and nominal struct shapes;
- independently generic nested calls with the same generic names and distinct call identities;
- a nested case where both parent and child initially remain unresolved;
- generic calls inside enclosing generic functions;
- generic receivers and receiver conflicts;
- explicit type arguments with closures and empty collections;
- explicit type arguments conflicting independently with receiver, argument, parameter-context, and return evidence;
- an earlier argument binding `$T` so a later closure observes it;
- an earlier deferred closure or literal not being retyped from a later conflicting argument;
- deterministic primary and related labels for two-way, three-way, and cross-file conflicts;
- nominal applications matched by canonical declaration and ordered arguments, including recursive applications, transformed nested arguments, non-interned equivalent applications, and same-named structs from different modules;
- `T` to `Maybe<$T>` inference and wrapping without double-wrapping an existing `Maybe<T>`;
- omitted `Maybe<$T>` arguments after earlier inference and an uninferred diagnostic when no evidence exists;
- non-`Void` callbacks accepted for compatible generic `Void` callback parameters and rejection of incompatible callback parameters;
- foreign interface and named-scalar conversions involving generic parameters;
- mutable-reference or addressability rejection after otherwise successful inference;
- function-value, static, method, named, variadic, and omitted-argument calls producing one final specialization;
- two differently specialized calls retaining distinct definitions;
- unresolved call-owned variables found in parameters, callback returns, nested nominal arguments, synthesized arguments, and coercion nodes producing checker diagnostics before AIR;
- existing `Maybe::new`, `Result::ok`, and `Result::err` behavior under direct, nested, explicit, and conflicting contexts;
- higher-order decoder combinations including `nullable`, `list`, and `field`;
- no call-owned variable or mutable source `FunctionDef` reaching AIR;
- checker and Go runtime parity for completed specializations.

The complete compiler suite must pass. Downstream validation must include `decode`, `tinear`, `sql`, and `maestro/server` because those projects exercise higher-order and nested generic composition.

## Related

- GitHub issue #325
- GitHub issue #322
- PR #326
- `docs/adrs/0010-support-dollar-prefixed-generics.md`
- `docs/adrs/0052-adopt-structured-labeled-diagnostics.md`
- `docs/adrs/0054-represent-generic-structs-as-nominal-applications.md`
- `compiler/checker/checker.go`
- `compiler/checker/scope.go`
- `compiler/checker/functions_test.go`
- `compiler/air/lower.go`
