# Compiler Docs

This directory contains implementation notes and design records for the Ard compiler.

## Current architecture references

- [`air-architecture.md`](./air-architecture.md) — AIR, the backend-facing representation after checking.
- [`go-emission-architecture.md`](./go-emission-architecture.md) — current Go/native backend architecture.
- [`ffi.md`](./ffi.md) — extern bindings and target companion modules.
- [`modules.md`](./modules.md) — module/import behavior.
- [`formatting.md`](./formatting.md) and [`formatter-algorithm.md`](./formatter-algorithm.md) — formatter behavior and implementation.
- [`testing.md`](./testing.md) — Ard test framework design and current behavior.
- [`javascript-runtime-semantics.md`](./javascript-runtime-semantics.md) — JS target runtime semantics.
- [`javascript-try-lowering.md`](./javascript-try-lowering.md) — JS `try` lowering.
- [`javascript-emission-architecture.md`](./javascript-emission-architecture.md) — intended JS backend architecture.

## Language feature notes

- [`async-eval-design.md`](./async-eval-design.md) — implemented async `eval`/generic fiber behavior.
- [`chained-comparisons.md`](./chained-comparisons.md) — chained comparison syntax and semantics.
- [`error-handling.md`](./error-handling.md) — `Result`, `Maybe`, and `try` user-facing behavior.
- [`generics.md`](./generics.md) — generic syntax and checker behavior.
- [`traits.md`](./traits.md) — trait design and behavior.

## Historical / deferred notes

- [`checker-error-recovery.md`](./checker-error-recovery.md) — historical/research note for checker recovery improvements.
- [`maybe-implementation.md`](./maybe-implementation.md) — historical implementation sketch for Maybe/generics.
- [`for-loop-expressions.md`](./for-loop-expressions.md) — deferred proposal.
