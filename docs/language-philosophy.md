# Ard Language Philosophy

Ard is designed to be readable, unsurprising, type-safe, and sound. The language should make common program behavior clear from the source code, reject invalid programs early, and avoid hidden control-flow or value-state surprises where possible.

## North Star

Ard is its own language, not Go with different syntax. Go is the default and reference target today, but Ard's semantics should be defined independently from Go whenever the distinction matters. The compiler's AIR exists as the semantic boundary between the frontend and backend so that Ard can support Go well without being permanently coupled to Go as the only possible target.

The Go backend should preserve Ard semantics first. Generated Go should be as idiomatic as practical when Ard concepts map naturally to Go concepts, but human readability of generated Go is not the primary language goal. When an Ard feature has no one-to-one Go equivalent, the backend should generate whatever correct lower-level Go is needed to implement the Ard semantics.

## Core Values

### Readability

Ard source should be easy to read left-to-right. Syntax and standard-library APIs should favor clarity over cleverness, and there should usually be one obvious way to express a common operation.

### Unsurprising semantics

Language features should behave consistently across modules, functions, methods, and generic code. Special cases should be rare, explicit, and justified by a clear semantic boundary such as Go interop.

### Type safety and soundness

Ard should use static checking to prevent invalid programs where it can. Runtime failures should be represented explicitly where possible rather than hidden behind implicit nulls, unchecked casts, or ambient exceptions.

## First-Class Ard Semantics

These features are part of Ard's semantic identity and should not be weakened merely because Go represents them differently:

- Nullability is explicit through `Maybe`.
- Errors are values through `Result`.
- Mutability and immutable access are explicit in the language.
- Modules are file-based and forward-looking, with public/private visibility as part of the source model.
- Traits express shared behavior. They are related to Go interfaces in the Go backend, but they are Ard traits first and may include semantics that are closer to traits in languages like Rust.
- Generics are core language features. Ard may support generic forms that Go does not support directly, including generic methods on structs.

## Simple Scalar Types

Ard intentionally keeps scalar types small and simple:

- `Bool`
- `Str`
- `Int`
- `Float`

`Int` and `Float` map to the target's ordinary large/default scalar types. In the Go backend, `Int` lowers to `int` and `Float` lowers to `float64`.

Additional scalar-like types such as bytes and runes may exist when they carry distinct language or interop meaning, but the default numeric model should remain simple.

## Relationship to Go

Go is Ard's default target language and the current practical runtime platform. Ard should interoperate with Go directly and well, especially because access to existing Go libraries is one of the language's major strengths.

However, Go is a target, not the definition of Ard. The compiler should not reject or reshape valuable Ard features solely because Go lacks a direct equivalent. Instead:

- If an Ard feature maps naturally to Go, lower it naturally.
- If an Ard feature does not map naturally to Go, preserve Ard semantics with generated helper code or lower-level translation.
- If a Go API exposes Go-specific concepts such as nil pointers, package state, interfaces, panics, or zero values, make that interop boundary explicit rather than pretending those concepts are native Ard semantics.

Generated Go should be idiomatic where that does not compromise Ard semantics. But generated Go is compiler output, not the primary user-facing artifact.

## FFI Direction

Interop with Go should be straightforward:

1. Prefer direct `use go:` imports when a Go package's API is representable in Ard.
2. Use ordinary Go shim packages when an external API needs adaptation before it is pleasant or safe to call from Ard.
3. Keep Ard-facing APIs explicit about semantic adaptation: convert Go errors into `Result`, absence into `Maybe`, and nil/panic-prone behavior into explicit wrappers or unsafe interop boundaries.

The best FFI design lets Ard call Go without unnecessary ceremony while still keeping Ard's own safety and readability promises intact.
