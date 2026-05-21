# vaxis-ard

Userland Ard Go-FFI demos using [Vaxis](https://github.com/rockorager/vaxis).

The examples intentionally keep Vaxis behind a small project-owned `ffi.go` layer. Ard owns the app state and behavior; Go owns the terminal capability boundary.

## Examples

- `counter/` — smallest smoke test for project Go FFI + Vaxis rendering/input.
- `todo/` — interactive todo list with add/delete/toggle/edit behavior.
- `tic-tac-toe/` — richer version of Ard's sample tic-tac-toe game with Vaxis rendering.

Each example is a standalone Ard project.
