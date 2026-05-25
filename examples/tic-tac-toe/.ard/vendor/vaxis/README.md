# vaxis

Reusable Ard bindings for [Vaxis](https://github.com/rockorager/vaxis).

This package is intended to exercise Ard's vendored dependency design. It owns
both the Ard API surface and the Go FFI companion for the Go target.

## Package shape

```text
vaxis/
  ard.toml
  go.mod
  vaxis.ard
  ffi/
    host.go
```

Consumers will eventually depend on this package with an Ard dependency alias:

```toml
[dependencies]
vaxis = { path = "../vaxis-ard" }
```

and import it as:

```ard
use vaxis
```

## API sketch

```ard
let vx = try vaxis::new("My TUI")
vaxis::clear(vx)
vaxis::draw_text(vx, 2, 1, "Hello from Vaxis")
try vaxis::render(vx)
try vaxis::close(vx)
```

## Existing examples

The old standalone experiments remain under `examples/counter/`,
`examples/todo/`, and `examples/tic-tac-toe/`. They still own their FFI directly
today; the goal is to migrate consumers to this root package once Ard dependency
support exists.
