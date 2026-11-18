# 0084: Instantiate Generic Function References from Expected Types

## Status

Accepted. Extends ADR 0055 and ADR 0010.

## Context

ADR 0010 makes every generic function call instantiate fresh type variables, and ADR 0055 defines how one call collects evidence for them. Neither says what a generic named function means when it is not called but used as a value:

```ard
fn same(x: $T) $T {
  x
}

fn apply(f: fn(Int) Int, x: Int) Int {
  f(x)
}

let m: Int? = Maybe::new(4)
m.map(same)
apply(same, 4)
let f: fn(Int) Int = same
```

The checker rejected every such reference as an unresolved generic (#593), even where the expected function type fixes every type parameter. The workaround was to wrap the call in an anonymous function. The Go backend already accepted type arguments on a function reference, so only the checker and AIR lacked a rule.

Static functions raise the same question through more paths (#607). `fn Cell::wrap(value: $T) Cell<$T>` can be named as `Cell::wrap`, `feature::Cell::wrap`, or through a type alias. An alias may apply type arguments, as in `type IntCell = Cell<Int>`. An owner path never carries type arguments of its own, but an alias does, and some rule must decide what they mean for the static function.

## Decision

A reference to a generic Ard function used where a function type is expected is instantiated from that type. It is treated like a call whose only evidence is the expected type: `same` where `fn(Int) Int` is expected means `same` with `$T = Int`.

### Instantiation

The reference owns fresh call-local variables for the function's own generics, following ADR 0055's ownership rules. The shared declaration is never mutated.

Evidence enters in this order:

1. Type arguments applied by an alias owner (see below).
2. The expected parameter types, left to right.
3. The expected return type.

Each piece of evidence constrains the variables; none overwrites an earlier binding. A fully resolved expected part is unified with the declared part. A part that still contains variables another call owns is used only for the positions it resolves.

Two special cases:

- **Variables of an enclosing call are skipped, not linked.** In `m.map(same)` the expected type is `fn(Int) $U`, where `$U` belongs to `map`. `$T = Int` comes from the parameter. The reference never binds `$U`; the enclosing call binds `$U` from the reference's specialized return type, as with any other argument. This keeps call ownership as ADR 0055 defines it.
- **A `Void` expected return is not evidence.** `fn(Int) Void` accepts `same` through the existing result-discarding conversion (ADR 0055, "Compatibility and coercion"), so the function's own return type is not constrained by it.

Generics of an enclosing declaration are rigid, concrete evidence. Inside `fn through(x: $U) $U`, `let f: fn($U) $U = same` binds `$T = $U`.

The specialization must fix every variable. Otherwise the reference reports the existing "Unresolved generic" diagnostic. That includes:

- no expected type (`let f = same`);
- an expected type that is not a function type (`let f: Any = same`);
- an expected type whose relevant parts belong to an enclosing call not yet fixed.

The last case follows ADR 0055's left-to-right rule. With `fn apply(f: fn($A) $A, x: $A)`, `apply(same, 4)` is unresolved, just as `apply(fn(n) { n }, 4)` is. Later arguments do not provide context to earlier ones. `apply_to(4, same)` with the function last, or `apply<Int>(same, 4)`, resolves. A return-only generic against an open callback return, as in `call(make)` with `fn make() $T` and `fn call(f: fn() $R) $R`, is likewise unresolved.

### Conflicts

Conflicting evidence is a type mismatch, not an unresolved generic:

- `let f: fn(Int) Str = same` is a mismatch between the annotation's `fn(Int) Str` and the specialization `fn(Int) Int`. The parameter fixed `$T = Int` first, and the return then conflicts.
- A different parameter count, or different parameter mutability or variadic metadata, is a mismatch against the generic signature: `let f: fn(Int, Int) Int = same` reports `fn(Int, Int) Int` against `fn($T) $T`.
- A conflict that leaves a variable open is also reported as a mismatch, not as unresolved.

In argument position the mismatch is reported as an incorrect argument type for that parameter. With an annotation or other expected value, it is a type-mismatch diagnostic with the structured labels from ADR 0052: the expected type on the annotation and the reference's type on the expression.

A completed specialization goes through the same checks as a call's:

- comparable requirements are recorded, so an enclosing generic function becomes `comparable` in Go when the referenced function needs it;
- a direct mutable generic parameter cannot specialize to an Ard trait.

### Where an expected type reaches a reference

A reference is instantiated wherever the checker already checks an expression against an expected type. These positions route a generic function reference through that expected type:

- call and builtin-method arguments, including nullable function parameters;
- annotated bindings and reassignment of a function-typed variable;
- the final expression of a block with an expected value, including function bodies, `if`/`else` branches, and `match` arms;
- struct fields, including nullable fields, generic struct fields, and field assignment;
- list, map, and tuple elements;
- the argument of `Maybe::new` when the expected type is a `Maybe` of a function type, as in `let m: (fn(Int) Int)? = Maybe::new(same)`. A bare `Maybe::new(same)` has no function type to instantiate from.

The expectation is attached to the reference node itself and does not reach nested expressions.

### Static function paths and alias owners

A static function is named through any path that denotes its owner type:

- `Box::wrap` locally;
- `feature::Box::wrap` from another module;
- a local or imported type alias, such as `Alias::wrap` or `feature::Alias::wrap`;
- an alias re-exported through another module.

These paths work both as calls and as values. Generic static functions used as values follow the instantiation rule above.

A static function declared locally under the exact name `owner::fn` takes precedence over a module or Go package named `owner`, for values as for calls.

An alias that applies type arguments to a generic owner binds them to the static function's generics **of the same name**, the way a receiver binds a method's generics. With `type IntCell = Cell<Int>`:

```ard
fn Cell::wrap(value: $T) Cell<$T> { ... }
fn Cell::tag(value: $T, label: $L) Cell<$T> { ... }

IntCell::wrap(1)                                // $T = Int from the alias
IntCell::wrap("s")                              // error: expected Int, got Str
IntCell::wrap<Str>("s")                         // error: generic T already bound to Int
let w = IntCell::wrap                           // fn(Int) Cell<Int>; no annotation needed
let t: fn(Int, Str) Cell<Int> = IntCell::tag    // $T from the alias, $L from the expected type
let u = IntCell::tag                            // error: unresolved generic
```

Alias bindings are evidence like any other and must agree with explicit type arguments, call arguments, and the expected type.

Binding is by name only. A static function whose generics are not named like the owner's parameters, such as `fn Cell::make(v: $U) Cell<$U>`, gets nothing from the alias. `IntCell::make("s")` is a `Cell<Str>`, and `let f = IntCell::make` is unresolved.

### The resulting value

Once every variable is fixed, the reference's type is a function type with the referenced function's own generics substituted. It declares no generics of its own but keeps its generic bindings. A binding such as `let w = IntCell::wrap` therefore keeps `fn(Int) Cell<Int>`, and calling `w(2)` infers nothing again. Inside a generic declaration it may still mention that declaration's generics, as `fn($U) $U` does in `through` above; those belong to the enclosing declaration, not to the reference.

The checker completes the specialization; AIR does not infer. AIR lowers a specialized reference as an `ExprFunctionRef` to the single generic definition. Its type arguments follow the declaration's generic parameter order, and the Go target emits an explicit instantiation such as `Same[int]`. A non-generic reference lowers as before.

### Explicit type arguments on a reference: deferred

`let f = same<Int>` does not parse today, and this decision does not add it. Every case it would cover already has a spelling:

- annotate the binding: `let f: fn(Int) Int = same`;
- put the type arguments on the enclosing call: `apply<Int>(same, 4)`.

If it is added later, these are the intended rules:

- **Parsing.** The parser accepts call type arguments only when `<` directly follows the name with no space (plus a few lookahead exclusions, such as `<<`), and when the token after the closing `>` is `(` (spaces are allowed there, a newline is not); otherwise it rewinds and reads `<` as less-than. Static paths such as `Maybe::new<Int>(...)` use the same rule. A reference form would also accept a closing `>` followed by a token that cannot continue an expression: a newline, `)`, `,`, `]`, `}`, or end of input. No valid program reads differently, because a comparison chain cannot end in `>` before those tokens. A dedicated AST node is preferable to type arguments on `Identifier`, so consumers that do not handle it fail visibly rather than dropping the arguments.
- **Semantics.** Explicit arguments supply all of the function's own generics in declaration order. They are bound in the same order as on a call (ADR 0055, "Explicit and receiver-derived bindings"): after any alias-owner bindings, which act like receiver bindings, and before the expected type. An explicit argument that disagrees with an alias binding is rejected as already bound, as `IntCell::wrap<Str>("s")` is today. A conflict with the expected type is a mismatch.

## Non-goals

This decision does not:

- support first-class references to unspecialized generic Go functions (ADR 0058); they still report that the Go function cannot be used as a value;
- make a generic function reference polymorphic, so a value has exactly one specialization;
- let a later argument provide context to an earlier function-valued argument;
- change ADR 0055's call pipeline or ADR 0010's syntax.

## Consequences

- Generic functions can be passed and stored directly wherever a function type is expected, without wrapper closures.
- References follow the same left-to-right limits as anonymous functions: `apply(same, 4)` and `apply(fn(n) { n }, 4)` are both unresolved.
- Static functions are values through every owner path, and their generic forms follow the same rule.
- An alias of an applied generic is a usable shorthand for both calls and values. Because binding is by name, it depends on a static function reusing its owner's generic parameter names.
- Fixing every type parameter leaves the value with no generics of its own, so it behaves like a non-generic function value after binding.
- Positions that do not check against an expected type, such as `let f = same`, still need an annotation.

## Related

- GitHub issue #593
- GitHub issue #607
- PR #608
- PR #610
- `docs/adrs/0010-support-dollar-prefixed-generics.md`
- `docs/adrs/0052-adopt-structured-labeled-diagnostics.md`
- `docs/adrs/0055-use-call-local-constraints-for-ard-generic-inference.md`
- `docs/adrs/0058-preserve-go-variadic-callable-types.md`
- `website/src/content/docs/advanced/generics.md` ("Generic Functions as Values")
- `website/src/content/docs/guide/structs.md` ("Static Functions")
- `compiler/checker/generic_function_values.go`
- `compiler/checker/static_function_owners.go`
- `compiler/air/lower.go` (`lowerFunctionValueRef`)
- `compiler/go/generic_function_value_test.go`
- `compiler/go/module_static_function_value_test.go`
