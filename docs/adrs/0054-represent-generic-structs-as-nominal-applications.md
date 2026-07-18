# 0054: Represent Generic Structs as Nominal Applications

## Status

Proposed

## Context

Ard currently represents both a struct declaration and a concrete use of that declaration with `checker.StructDef`. Specializing a generic struct such as `Box<Int>` recursively copies the declaration's fields and substitutes its type parameters in the copied graph.

That eager representation fails for recursive generic types whose recursion passes through a finite runtime indirection. For example:

```ard
struct Context<$T> {
  state: $T,
  handlers: [fn(mut Context<$T>)],
}
```

Specializing `Context<Int>` copies `Context`, then its `handlers` field, then the callback parameter `Context<$T>`, and then the struct again. The struct copier has a recursion guard, but function-type copying currently starts a new guard. The checker therefore exhausts the Go runtime stack instead of accepting the finite representation or reporting a bounded diagnostic.

Threading the existing guard through function copying would stop the crash, but it would retain the deeper problem: an applied nominal type is represented by a recursively copied declaration. Copies can diverge from the canonical declaration, equality and generic inference inspect fields unnecessarily, and AIR must recover nominal identity from structurally duplicated checker values.

A memoized eager copier is also insufficient for recursive applications that transform their arguments:

```ard
struct Chain<$T> {
  next: fn(Chain<[$T]>),
}
```

Expanding `Chain<Int>` produces `Chain<[Int]>`, then `Chain<[[Int]]>`, without reaching a finite set of applications. Go rejects the equivalent declaration as an instantiation cycle even though the value layout passes through a function.

Ard needs to distinguish three questions:

1. What is the canonical declaration of a named struct?
2. What type arguments are applied at a particular use site?
3. Does recursion create an invalid by-value layout or an unbounded generic instantiation cycle?

## Decision

Represent a generic struct type as a nominal application of one canonical declaration rather than as a deep copy of that declaration.

Conceptually, the checker model becomes:

```go
type StructDef struct {
    Name          string
    ModulePath    string
    GenericParams []string
    Fields        map[string]Type
    // declaration-owned methods, traits, visibility, and source metadata
}

type StructType struct {
    Def      *StructDef
    TypeArgs []Type
}
```

The exact Go names may differ during implementation. The required invariant is:

> A canonical struct declaration owns field templates and declaration metadata. An applied struct type references that declaration and owns only its ordered type arguments.

Canonical declarations are not cloned during generic specialization.

### Nominal identity

A struct application is identified by its canonical declaration and ordered type arguments:

```text
Context<Int> == Context<Int>
Context<Int> != Context<Str>
first::Context<Int> != second::Context<Int>
```

Fields do not participate in named struct identity. Type equality and generic unification compare the canonical declaration and corresponding type arguments rather than recursively comparing field maps.

Repeated applications may be interned by declaration identity and type arguments, but correctness must not depend on pointer equality between application values. Applications containing unresolved mutable type variables must not use cache keys that can become stale after binding.

### Declaration resolution

Top-level struct declarations continue to be hoisted before their fields are populated. A generic reference encountered while its declaration is being resolved creates a nominal application immediately:

```ard
fn(mut Context<$T>)
```

becomes conceptually:

```text
fn(mut StructType{Def: Context, TypeArgs: [$T]})
```

Resolving that reference does not force the declaration's fields to expand, return a bare unspecialized declaration, or emit a recursive-specialization diagnostic merely because the declaration is in progress.

The same rule supports mutually recursive declarations:

```ard
struct A<$T> {
  callback: fn(B<$T>),
}

struct B<$T> {
  callback: fn(A<$T>),
}
```

### Lazy field specialization

Field lookup obtains the field template from the canonical declaration and substitutes the application's type arguments only into that field type.

For `Context<Int>`, the declaration field:

```text
handlers: [fn(mut Context<$T>)]
```

is observed as:

```text
handlers: [fn(mut Context<Int>)]
```

Substitution transforms the type arguments of nested struct applications; it does not recursively copy or expand their declarations.

Struct literals, property access, method lookup, trait conformance, and diagnostics use the canonical declaration for metadata and the application environment for concrete types.

### Generic parameter identity

Substitution environments are scoped to the declaration that owns their parameters. The implementation may initially use a declaration-local ordered name map, but caches and intermediate representations must not assume a type-parameter name is globally unique.

AIR type parameters should be identified by their owning generic definition and parameter position rather than only by names such as `$T`.

### Recursive value layout

Representing recursive nominal references does not make every recursive value layout valid. After declarations are resolved, the checker validates direct struct-containment cycles separately from type resolution.

A recursive layout is valid only when every cycle crosses an edge whose runtime representation has size independent of the referenced struct. On the Go target, representation-indirect edges include:

- function values;
- lists and maps;
- channels, senders, and receivers;
- mutable references;
- supported foreign pointer or reference forms.

For example, these are finite:

```ard
struct Context<$T> {
  callback: fn(Context<$T>),
}

struct Node<$T> {
  children: [Node<$T>],
}
```

Direct value containment remains invalid:

```ard
struct Node<$T> {
  next: Node<$T>,
}
```

Mutually recursive direct containment is also invalid. Inline wrappers such as `Maybe<T>` and `Result<T, E>` do not break a layout cycle when their backend representation embeds the contained value.

The checker builds a graph of direct-layout struct edges and diagnoses self-loops or strongly connected components. Diagnostics should identify the field path forming the cycle and suggest an indirect representation.

This validation must follow the backend representation contract rather than infer indirection only from surface syntax.

### Generic instantiation cycles

Layout indirection and generic instantiation are validated independently. Recursive applications that preserve their arguments are supported:

```text
Context<T> -> Context<T>
A<T> -> B<T> -> A<T>
```

Cycles that transform an argument into an endlessly distinct application are rejected:

```ard
struct Chain<$T> {
  next: fn(Chain<[$T]>),
}
```

This is rejected even though the function makes its value layout finite, because following the cycle requires `Chain<T>`, `Chain<[T]>`, `Chain<[[T]]>`, and so on. The Go target rejects the equivalent generic declaration as an instantiation cycle.

The diagnostic for an instantiation cycle is distinct from the diagnostic for recursive by-value layout and identifies the transformed argument path.

### Type transformation

Generic substitution becomes a finite transformation over type expressions:

- type parameters are replaced from the declaration-scoped environment;
- list, map, function, union, result, maybe, and reference components are transformed recursively;
- a struct application retains its canonical declaration and transforms only its type arguments;
- canonical struct fields are never traversed merely to substitute an application.

Existing overlapping struct-copy and generic-replacement paths should converge on this behavior. Transformations of recursive non-struct composites remain memoized where necessary.

### AIR representation

AIR interns generic struct declarations once. A concrete application is identified by:

```text
(generic definition ID, ordered argument TypeIDs)
```

AIR does not include expanded fields in the identity key of a nominal struct application. It registers placeholders before lowering recursive signatures so regular recursive references can point back to an existing type identity.

A generic declaration's fields may reference applications of the same or another generic definition using AIR type parameters. For the accepted `Context<$T>` example, the callback parameter refers to the same generic definition applied to the same type parameter.

The existing AIR self-reference key shortcut must not collapse distinct applications such as `Context<Int>` and `Context<Str>`. Recursive-layout legality remains a checker validation and is not inferred from AIR key generation.

### Go lowering

The Go backend emits ordinary nominal generic declarations and applications. The motivating example should lower conceptually to:

```go
type Context[T any] struct {
    State    T
    Handlers []func(*Context[T])
}
```

The exact pointer shape follows Ard's existing mutable-reference lowering contract.

No backend is required to support a checker-accepted recursive layout unless its representation provides the same finite indirection guarantees. Backend-independent checker rules should therefore use Ard's defined representation semantics, with target validation permitted where a target cannot honor them.

## Consequences

- Function-indirected recursive generic structs type-check without constructing recursive declaration copies.
- Mutually recursive generic declarations with stable argument flow become representable.
- Direct infinite-size struct layouts and transformed generic instantiation cycles receive bounded, specific diagnostics.
- Struct equality and unification become consistently nominal.
- Field specialization becomes demand-driven and avoids unnecessary graph copying.
- Checker code that assumes every struct type is a `*StructDef` must be migrated to distinguish declarations from applications or use shared helpers that expose canonical declaration metadata.
- Generic struct literals, inferred type arguments, property access, methods, traits, mutable references, diagnostics, LSP type display, AIR interning, and Go lowering require regression coverage.
- Existing deep-copy helpers may remain temporarily for function-generic or non-struct transformations, but they must no longer define generic struct application semantics.
- This is a type-model migration rather than a local recursion-guard fix.

## Implementation Plan

1. Introduce nominal struct applications and declaration/application helper APIs.
2. Make explicit and inferred generic struct uses carry complete ordered arguments.
3. Specialize field templates lazily for property access and struct literals.
4. Change struct equality and generic unification to declaration-plus-arguments semantics.
5. Permit nominal references while struct declarations are being populated.
6. Add direct-layout cycle validation and generic instantiation-cycle validation.
7. Update method and trait lookup to bind the receiver application's arguments while retaining declaration-owned metadata.
8. Intern AIR struct applications by generic definition and argument IDs, with owner-scoped type parameters.
9. Add Go execution coverage for recursive callback fields, lists of callbacks, mutable callback parameters, and mutual generic recursion.
10. Remove the obsolete recursive-specialization diagnostic and eager generic struct-copy paths once all callers use nominal applications.

## Validation

At minimum, implementation must cover:

- the complete reproduction from issue #318;
- self-reference through function parameters and returns;
- self-reference through lists, maps, channels, and mutable references;
- mutual recursion through indirect fields;
- distinct `Context<Int>` and `Context<Str>` applications;
- same-named declarations from different modules;
- rejection of direct and mutually recursive by-value layouts;
- rejection of argument-transforming instantiation cycles;
- generic struct literals and inferred type arguments;
- generic receiver methods and trait implementations;
- AIR identity and generated Go compilation/runtime behavior.

## Related

- GitHub issue #318
- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0022-use-mut-for-mutable-references.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `compiler/checker/checker.go`
- `compiler/checker/scope.go`
- `compiler/checker/top_level_types.go`
- `compiler/checker/type_equal.go`
- `compiler/air/lower.go`
