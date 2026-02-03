# Bytecode VM Plan

## Instruction Model

- Stack-based VM to mirror current interpreter behavior.
- A small set of opcodes that map directly to runtime.Object operations.

Core opcode groups:
- Constants and literals (Int, Float, Str, Bool, Void)
- Local/global load/store
- Binary ops and comparisons
- Calls (local, module, extern)
- Control flow (jump, jump-if, loop)
- Data constructors (list, map, struct)
- Pattern matching (match enter, arm check, bind)

## Runtime Integration

- Reuse runtime.Object, Result, Maybe, Dynamic
- Preserve error semantics for panic and Result/Maybe propagation
- Keep FFI registry intact, call through the same interface

## Emitter Strategy

- Emit bytecode from lowered nodes
- Use symbol table for locals and module references
- Emit explicit stack effect annotations for each opcode

## Debugging/Tooling

- Bytecode disassembler for debugging
- Optional source maps for error reporting
- Verification pass for stack depth and call arity

## Milestones

- MVP opcodes to execute literals, arithmetic, and function calls
- Add control flow and pattern matching
- Add structs, lists, maps, and enums
- Plug in async runtime calls
