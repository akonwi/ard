# 0021: Represent Module-Level Lets as AIR Globals

## Status

Accepted

## Context

Ard allows module-level immutable `let` bindings. The checker exposes those bindings as module symbols, so functions in the same module and importing modules can type-check references to them.

AIR previously modeled executable top-level statements as a synthetic script function. That is sufficient for script-style programs, but it is not sufficient for module-level bindings that are referenced from ordinary functions. When a function is lowered independently—especially from an imported module during whole-program lowering—the binding is not in the function's local scope and may not have been lowered as part of the root script. This can produce lowering errors such as `unknown local refresh_event` even though checking succeeded.

Treating module-level bindings as script locals would make their availability depend on entrypoint selection and lowering order. It also obscures the fact that immutable module-level bindings are module-owned values, not function locals.

## Decision

Represent immutable module-level `let` bindings in AIR as module globals.

A module global should record:

- the owning module
- the source name
- the resolved type
- a target-neutral initializer region containing the lowered expression and its initializer-wide local table
- mutability metadata

AIR expressions should be able to load a module global directly. Lowering should resolve references to same-module globals when lowering functions, closures, and initializers. Lowering should also ensure globals are declared for imported modules before imported module functions, methods, and trait impls that reference them are lowered. Global initializer expressions should be lowered after the relevant module declarations and trait impl declarations are available, before AIR validation.

Backends should emit module globals as target-level module/package declarations and lower AIR global loads to references to those declarations. Imported module globals should be emitted in the owning module and referenced through the target's existing module import mechanism where applicable.

Global initializers may reference functions, methods, traits, and other globals, but cyclic global initialization is invalid. Sequential source name resolution rejects direct forward references; target-neutral detection of cycles mediated through function bodies remains future AIR validation work. AIR models each initializer as a global-owned executable region: `GlobalInitializer` keeps the initializer expression together with every `Local` addressed by that expression. This supports target-neutral block, match, loop, closure, and other statement-producing expressions without pretending that the initializer is a source function.

Mutable top-level bindings are represented by the same global form. Mutability remains checker-enforced metadata on the global and controls assignment validation and target visibility.

## Consequences

- Module-level immutable bindings become usable from functions in the same module and from functions reached through whole-program imported-module lowering.
- AIR more accurately represents module-owned values instead of relying on synthetic script locals.
- Target backends must emit globals in deterministic module order and support global-load expressions.
- AIR validation must validate global table entries and global-load references.
- Initializer expressions stay target-neutral, and their global-owned local environment gives setup statements and closure captures the same stable `LocalID` meaning they have in functions.
- Global declaration and global initializer lowering are distinct phases so imported function and method bodies can refer to globals before their initializer values are lowered.
- Direct forward-reference cycles fail during checking; transitive function-mediated cycles currently reach the target compiler and remain a future AIR diagnostic.
- Cross-module global references require module dependency discovery so JavaScript and future multi-file targets import the owning module when needed.

## Related

- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0013-use-file-based-modules-and-absolute-imports.md`
- `compiler/air`
- `compiler/go`
- `compiler/javascript`
