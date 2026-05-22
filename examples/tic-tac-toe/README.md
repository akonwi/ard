# Vaxis tic-tac-toe

A larger userland FFI demo for Ard's Go target using [Vaxis](https://github.com/rockorager/vaxis).

The Ard program owns the tic-tac-toe game state and rules. `ffi.go` is the whitelisted Go capability layer that owns terminal rendering/input through Vaxis.

## Run

From this directory:

```sh
ard run main.ard
```

Or build:

```sh
ard build --out ttt main.ard
./ttt
```

## Test

```sh
./test_tic_tac_toe.py
```

The smoke test builds the Ard Go target, runs the TUI under a PTY, sends keys, and asserts on rendered output.

## Controls

- arrows or `h/j/k/l`: move selection
- `1`-`9`: jump to a square
- enter/space: play selected square
- `r`: restart
- `q` or ctrl-c: quit

## FFI surface

Ard sees only this narrow API:

```ard
extern type Terminal = "*vaxis.Vaxis"
extern fn tui_open() Terminal!Str = "TuiOpen"
extern fn tui_close(term: Terminal) Void!Str = "TuiClose"
extern fn tui_clear(term: Terminal) Void = "TuiClear"
extern fn tui_draw_text(term: Terminal, x: Int, y: Int, text: Str) Void = "TuiDrawText"
extern fn tui_flush(term: Terminal) Void!Str = "TuiFlush"
extern fn tui_read_key(term: Terminal) Str!Str = "TuiReadKey"
```
