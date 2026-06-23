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

### Ard impl methods need Go method-set support

Ard impl methods have historically lowered as standalone functions with explicit receiver parameters. That preserves execution semantics, but it does not add methods to the generated Go type's method set. As a result, Ard-defined structs cannot naturally satisfy Go interfaces.

Current direction: when an Ard impl method has a Go-representable receiver and signature, also lower it as a real Go method while keeping the standalone helper for existing internal call paths.

### Ard traits are not Go interfaces

Plain Ard trait objects currently rely on `any`, generated dispatch, and special forwarding tables for mutable trait references. This is workable internally, but it does not line up with Go's interface model.

Desired direction: generate one canonical Go interface for each Go-representable Ard trait in the package that defines the trait. Ard traits define required methods only; whether an implementation mutates should be represented by ordinary Go receiver choice (`T` vs `*T`), not by a distinct trait object shape. A source-level `mut Trait` position remains valid and means the value used for that trait position must be mutable/addressable when required, but it should not lower to a separate forwarding-table runtime representation long term.

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
   - Initial implementation notes:
     - AIR now records receiver/method metadata for impl methods.
     - The Go target emits wrapper methods around existing standalone impl helpers for eligible local struct/enum/union receivers.
     - Wrappers are skipped when Go cannot legally attach the method, when a struct field has the same Go selector name, or when multiple Ard impl methods would collide on one Go method name.
     - Internal Ard calls and trait dispatch still use the existing standalone helper functions.

2. **Generate Go interfaces for Go-representable Ard traits.**
   - Emit the canonical interface in the package that defines the Ard trait.
   - Treat trait definitions as method requirements only; do not encode implementation mutability in the trait interface.
   - Represent mutating implementations with pointer receiver methods, so `*T` satisfies the same interface when mutation is required.
   - Keep `mut Trait` valid at use sites as an addressability/mutability requirement for the instance being used.
   - Retire the Go-target-specific mutable trait-reference forwarding-table representation once pointer receiver interface dispatch covers the semantics.
   - Avoid changing traits whose method signatures need Ard-only runtime adaptation.
   - Initial implementation notes:
     - The Go target now emits a native Go interface declaration for each Go-representable Ard trait object type.
     - Immutable trait object types use that interface when every known implementation can satisfy it with generated Go method wrappers.
     - Traits with mutating receiver implementations now remain eligible for native interface lowering when there are no `mut Trait` use sites that still require the transitional forwarding-table representation; pointer receiver method wrappers let `*T` satisfy the same trait interface.
     - Project/dependency FFI boundaries adapt top-level `Trait`, `Trait?`, and `Trait!E` returns where practical, but fall back to the old `any` representation for container-shaped FFI signatures that are not recursively adapted yet.
     - ADR 0023's forwarding-table design is now considered a transitional Go-target implementation detail to retire, not the desired long-term trait representation.

3. **Let Ard structs satisfy Go interfaces naturally.**
   - Once methods are real Go methods, direct-Go interface assignability can rely on Go method sets.
   - This should enable Ard-defined adapter types for Go APIs without handwritten Go companion wrappers in common cases.
   - Initial implementation notes:
     - The checker now derives a Go-compatible method set for Ard-defined structs and uses it when checking assignability to direct-Go interfaces.
     - The Go target declares and lowers uncalled inherent impl methods so their generated Go method wrappers exist even when the method is only needed by pure Go interface dispatch.
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
     - Type declarations are now emitted with their owning Ard module's generated Go file when AIR records an owner; synthetic compiler-generated helper types still emit with the entry module. Trait object interface declarations are emitted with the trait-defining module when AIR records a trait owner, and trait interfaces now use natural Go names such as `Renderable`, `internalDrawable`, or stdlib `ToString` instead of synthetic `ardTrait_*` names. Trait object type lowering is now module-context-aware so cross-module trait references can be package-qualified when module package emission is enabled; current default output remains single-package until the rest of cross-module references are package-ready.
     - The Go backend now has shared helpers for mapping an Ard module path to a sanitized Go package name, output directory, and generated import path. The source writer also creates nested directories before writing generated files, preparing for module packages to move into subdirectories.
     - Public user-defined struct fields now lower to exported natural Go field names such as `first_name` -> `FirstName`; private structs keep unexported natural field names such as `secret_key` -> `secretKey`. Standard-library generated structs temporarily keep their existing field names to avoid breaking FFI companion code while the natural naming work is rolled out incrementally.

5. **Add boundary adapters only where semantics differ.**
   - `Result` and `Maybe` stay internal Ard values.
   - Optional Go wrappers can translate to/from `(T, error)`, pointers, or comma-ok conventions when explicitly desired.

## Open questions

- Should Go method lowering eventually replace standalone impl functions, or should both forms remain permanently for internal call stability?
- How should Ard visibility map to exported Go identifiers in a single generated package?
- Should generated Go interfaces for traits be exported only for public traits?
- How should method name collisions be handled when multiple Ard traits define the same method name for one type?
- What syntax or metadata should request Go-facing wrappers for `Result`/`Maybe`, if any?

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0023-represent-mutable-trait-references-with-forwarding-tables.md`
- `docs/adrs/0024-preserve-maybe-semantics-in-go-lowering.md`
- `docs/adrs/0028-use-direct-go-imports-for-ffi.md`
