# Backlog

This directory holds design notes for larger work that is not currently being implemented.

## Active notes

- [`ard-dependency-system/`](./ard-dependency-system/) — design sketch for future git-backed dependency management.

## Recently completed

- Go sample/server integration coverage from issue #119 is now represented in compiler tests and example smoke tests:
  - `compiler/main_test.go` asserts sample stdout, scripted stdin, and server routes.
  - `examples/server/test_server.py` exercises the HTTP example in CI.
  - `examples/tic-tac-toe/test_tic_tac_toe.py` exercises the Vaxis TUI example in CI.
