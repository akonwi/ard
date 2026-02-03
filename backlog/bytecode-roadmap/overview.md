# Roadmap Overview

Goal: generate portable binaries by compiling Ard programs to bytecode and embedding the runtime.

## Phase 0: Prerequisites

- Complete AST lowering so backends do not inspect checker.Type.
- Stabilize runtime.Object, Result, Maybe, Dynamic APIs used by all backends.

## Phase 1: Lowered IR Stabilization

Deliverables:
- No VM code path performs type assertions on checker.Type.
- Checker nodes carry all metadata required for execution (field types, method dispatch, match shapes).
- A small conformance test suite that executes the same program via current VM and lowered VM.

Exit criteria:
- VM can execute entirely from lowered nodes without reading checker.Type.

## Phase 2: Bytecode Emitter

Deliverables:
- A bytecode instruction set with:
  - expressions (literals, ops, calls)
  - control flow (if, loops, break/continue)
  - data structures (list/map/struct literals)
  - pattern matching
- An emitter from lowered nodes to bytecode.
- A bytecode verifier (basic stack discipline, arity checks).

Exit criteria:
- A representative subset of stdlib samples compiles to bytecode and runs in a new bytecode VM.

## Phase 3: Bytecode VM

Deliverables:
- A bytecode VM that reuses runtime.Object and result handling.
- Deterministic behavior matching the interpreter VM for the test suite.
- Bytecode serialization format and a loader.

Exit criteria:
- End-to-end compile + run from bytecode for multiple samples and tests.

## Phase 4: Binary Packaging

Deliverables:
- CLI mode: compile to bytecode and embed in a single binary.
- Optional: separate bytecode file + runtime runner.

Exit criteria:
- A binary can run without source files or the interpreter frontend.
