# Go Backend Idiomatic Lowering Roadmap

Status: living design note, not an ADR.

This note tracks the direction for making Ard's Go backend produce Go-native shapes where that improves interoperability with ordinary Go code. It should be updated as work lands or decisions change.

## Goal

Make Ard-defined values participate naturally in Go's type system, especially method sets and interfaces, while preserving Ard's source-level semantics.

The key idea is not to make Ard "just Go". The goal is to lower Ard constructs to idiomatic Go when the Go representation is a faithful implementation of Ard semantics, and to keep explicit Ard runtime representations when they are semantically better.

## Non-goals

- Do not replace `runtime.Result[T, E]` with Go's `(T, error)` tuple internally. Ard `Result` is intentionally a first-class value that can be passed around.
- Do not replace `runtime.Maybe[T]` with nil pointers, comma-ok tuples, or bare Go option conventions internally. Ard `Maybe` is intentionally explicit and value-shaped.
- Do not try to support every Go interop shape at once. Direct-Go interop can remain incremental.
- Do not reintroduce a universal `runtime.Object`, registry-driven adapter layer, or all-values-as-`any` backend.

## Current gaps that matter most

### Some impl methods still require standalone fallbacks

Compatible Ard impl methods lower directly as Go receiver methods, and internal Ard calls and trait dispatch use those methods. The backend no longer emits a duplicate standalone helper plus a receiver wrapper for the common case.

Standalone lowering remains only when Go cannot legally or safely attach the natural method: selector collisions with fields or other methods, names reserved for generated behavior, methods on concrete generic instantiations, and generic methods that require stronger constraints than their receiver type provides. Eliminating those fallbacks requires explicit collision diagnostics or a stable method-name policy rather than duplicate declarations.

### Trait values use native Go interfaces

ADR 0061 makes `Trait` and `mut Trait` share one canonical Go interface. Mutable trait values capture the current dynamic object rather than forwarding replacement of a trait-typed storage slot. Copying, rebinding, equality, hashing, `Any`, and generic behavior therefore follow Go interface semantics directly.

Natural implementations expose the trait's natural Go methods. Selector collisions and other legal Ard implementations use collision-proof generated receiver methods behind the same native interface. The backend emits no mutable-trait handle, adapter, vtable, registry, registration initializer, or storage projection hook. `mut Trait -> Trait` is representation-free. Explicit `.@` uses one isolated reflective helper to shallow-copy the hidden dynamic concrete value into an ordinary trait; reflection is not involved in normal storage, copying, or dispatch.

### Generated names and packages are artifact-oriented

Generated code is currently optimized as a deterministic executable artifact, not as a stable Go library API. Names like `main_ard__User` and `main_ard__greet` are safe but not idiomatic.

Desired direction: separate internal collision-proof names from a Go-facing ABI for public Ard APIs. Library-oriented output can use exported Go identifiers where Ard visibility permits it.

### Struct fields are usually unexported

User Ard structs generally lower to Go structs with unexported fields. That limits use from pure Go code.

Desired direction: for public Go-facing Ard types, expose fields or constructors/accessors in a way that respects Ard visibility while still being usable from Go.

## Preserved Ard runtime shapes

### Result

Keep `runtime.Result[T, E]` as the canonical internal lowering of Ard `T!E`. It is an Ard semantic improvement over Go's multi-return error convention because it is a first-class value.

If Go-facing adapters are needed later, they should be wrappers at the boundary, not the internal representation.

### Maybe

Keep `runtime.Maybe[T]` as the canonical internal lowering of Ard `T?`. It preserves explicit absence and avoids conflating optionality with pointer/reference semantics.

If Go-facing adapters are needed later, they should be wrappers at the boundary, not the internal representation.

## Proposed work sequence

1. **Lower Ard impl methods as Go methods where possible.**
   - Generate real receiver methods for compatible impls.
   - Preserve existing standalone helper lowering where method lowering is not possible or would change semantics.
   - Ensure pointer/value receivers match Ard `mut` receiver behavior.
   - Implementation notes:
     - AIR records receiver/method metadata for impl methods.
     - Eligible local struct/enum/union methods emit their bodies directly as Go receiver methods, using value or pointer receivers according to Ard receiver mutability.
     - Internal Ard calls, trait dispatch, and mutable-trait forwarding call those receiver methods directly.
     - Standalone fallbacks remain when Go cannot legally attach the method, when a struct field or reserved generated method has the same selector name, when multiple Ard methods collide on one Go method name, when the receiver is a concrete generic instantiation, or when a method requires stronger generic constraints than the receiver type provides.

2. **Generate Go interfaces for Go-representable Ard traits.**
   - Emit the canonical interface in the package that defines the Ard trait.
   - Treat trait definitions as method requirements only; do not encode implementation mutability in the trait interface.
   - Represent mutating implementations with pointer receiver methods, so `*T` satisfies the same interface when mutation is required.
   - Keep `mut Trait` valid at use sites as an addressability/mutability requirement for the instance being used.
   - Lower `Trait` and `mut Trait` to the same canonical Go interface and use Go interface copying/rebinding semantics.
   - Avoid changing traits whose method signatures need Ard-only runtime adaptation.
   - Initial implementation notes:
     - The Go target now emits a native Go interface declaration for each Go-representable Ard trait object type.
     - Immutable trait object types use that interface when every known implementation can satisfy it with generated Go receiver methods.
     - Traits with mutating receiver implementations use pointer receiver methods, so `*T` satisfies the same interface used by ordinary and mutable trait source types.
     - Project/dependency FFI boundaries adapt top-level `Trait`, `Trait?`, and `Trait!E` returns where practical, but fall back to the old `any` representation for container-shaped FFI signatures that are not recursively adapted yet.
     - ADR 0061 supersedes the forwarding-table design. Fallback traits remain native interfaces by using collision-proof generated methods rather than `any` and concrete type switches.

3. **Let Ard structs satisfy Go interfaces naturally.**
   - Once methods are real Go methods, direct-Go interface assignability can rely on Go method sets.
   - This should enable Ard-defined adapter types for Go APIs without handwritten Go companion wrappers in common cases.
   - Initial implementation notes:
     - The checker now derives a Go-compatible method set for Ard-defined structs and uses it when checking assignability to direct-Go interfaces.
     - The Go target declares and lowers uncalled inherent impl methods so their generated Go receiver methods exist even when the method is only needed by pure Go interface dispatch.
     - Trait impl methods continue to use the existing trait-impl declaration path to avoid duplicate wrapper collisions.

4. **Generate natural Go packages for Ard modules.**
   - Lower Ard modules directly to idiomatic Go packages instead of adding a wrapper ABI layer over artifact-oriented names.
   - Ard visibility is public-by-default: functions, structs, enums, traits, and methods are public unless marked `private`; `let` module variables are public by default; `mut` module variables are private by default; fields of public structs are public.
   - Map public Ard declarations to exported Go declarations and private Ard declarations to unexported Go declarations.
   - Since each Ard file is a module, map each generated Go package to one Ard file/module. There are no Ard submodules or internal modules to model separately.
   - Use the Ard module/file basename as the Go package name when it is already a valid Go package identifier, including names like `foo_bar`.
   - Deterministically sanitize invalid package names: replace invalid identifier characters with `_`, handle leading digits by prefixing a valid character, suffix Go keywords with `_`, collapse repeated underscores, and use a fallback such as `module` when sanitization would otherwise produce an empty name.
   - Apply stable Ard-to-Go identifier conversion for declarations, e.g. `make_user` -> `MakeUser` for public names and `format_name` -> `formatName` for private names.
   - Prefer diagnostics for public API name collisions after Go naming/sanitization, because suffixing exported user declarations would make the public API surprising.
   - Do this as the normal Go target behavior rather than introducing a separate library/build mode; executable entry modules still generate the required Go `main` entrypoint.
   - Initial implementation notes:
     - AIR now carries declaration privacy for globals, functions, and named types so the Go backend can distinguish exported and unexported API names.
     - The Go backend has shared helpers for deterministic Go package-name sanitization and Ard-to-Go identifier conversion; package names like `foo_bar` are preserved, invalid characters become `_`, leading digits are prefixed, and Go keywords get a trailing `_`.
     - Public Ard type names now lower directly to natural exported Go identifiers, e.g. `struct User` -> `type User`; private type names lower to unexported natural identifiers, e.g. `private struct internal_config` -> `type internalConfig`. Colliding type names conservatively fall back to the legacy artifact-style names until generated packages are separated by Ard module.
     - Type declarations are now emitted with their owning Ard module's generated Go package when AIR records an owner; synthetic compiler-generated helper types emit with the module that needs them. Trait object interface declarations are emitted with the trait-defining module when AIR records a trait owner, and trait interfaces now use natural Go names such as `Renderable`, `internalDrawable`, or stdlib `ToString` instead of synthetic `ardTrait_*` names. Trait object, named type, composite literal, enum variant, enum cast, function, and global reference lowering are module-context-aware, and the Go backend now emits Ard modules as separate Go packages by default.
     - The Go backend has shared helpers for mapping an Ard module path to a sanitized Go package name, output directory, and generated import path. Generated source is written into nested package directories for non-entry modules, while executable entry modules keep the root `main` package and import generated module packages.
     - Public functions and globals now lower to natural exported Go identifiers such as `make_user` -> `MakeUser` and `default_name` -> `DefaultName`; private functions/globals lower to unexported natural names. Ard value declarations use safe generated aliases such as `fmt_1` or `len_1` when their natural Go names would collide with generated imports, Go builtins, or existing top-level Go declarations. Enum variants lower to natural constant names such as `DirectionDown`, with generated aliases for collisions. Compiler-generated script, closure, and standalone method helper functions keep legacy artifact names.
     - Public Ard struct fields now lower to exported natural Go field names such as `first_name` -> `FirstName`; private structs keep unexported natural field names such as `secret_key` -> `secretKey`. Standard-library Ard structs follow the same field naming rules as project source.

5. **Add boundary adapters only where semantics differ.**
   - `Result` and `Maybe` stay internal Ard values.
   - Optional Go wrappers can translate to/from `(T, error)`, pointers, or comma-ok conventions when explicitly desired.

## Open questions

- How should selector collisions and concrete generic-instantiation methods be diagnosed or represented so the remaining standalone method fallbacks can be removed?
- How should Ard visibility map to exported Go identifiers in a single generated package?
- Should generated Go interfaces for traits be exported only for public traits?
- How should method name collisions be handled when multiple Ard traits define the same method name for one type?
- What syntax or metadata should request Go-facing wrappers for `Result`/`Maybe`, if any?

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0023-represent-mutable-trait-references-with-forwarding-tables.md`
- `docs/adrs/0024-preserve-maybe-semantics-in-go-lowering.md`
- `docs/adrs/0028-use-direct-go-imports-for-ffi.md`
