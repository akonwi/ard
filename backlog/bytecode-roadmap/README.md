# Bytecode Backend Roadmap

This folder tracks outstanding bytecode-related backlog items.

Active backlog docs:
- `lowering.md`: remaining AST lowering work to decouple backends
- `go-backend.md`: optional Go codegen notes and pitfalls
- `performance.md`: VM/FFI profiling and runtime optimization backlog
- `value-native-vm.md`: sketch for eliminating `runtime.Object` and moving to a Go-native VM value model

Historical/reference docs moved to `compiler/docs`:
- `compiler/docs/air-architecture.md`
- `compiler/docs/bytecode-roadmap-overview.md`
- `compiler/docs/bytecode-vm-plan.md`
- `compiler/docs/ast-lowering-for-backends.md`
- `compiler/docs/async-eval-design.md`
- `compiler/docs/checker-error-recovery.md`
