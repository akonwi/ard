# Vaxis tic-tac-toe

A larger userland FFI demo for Ard's Go target using [Vaxis](https://github.com/rockorager/vaxis).

The Ard program owns the tic-tac-toe game state and rules. `ffi.go` is the whitelisted Go capability layer that owns terminal rendering/input through Vaxis.

## Run

Requires an `ard` binary installed from the Go-FFI compiler branch:

```sh
cd /path/to/ard/compiler
go install
```

Then from this directory:

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
extern type Terminal
extern fn tui_open() Terminal!Str = { go = "TuiOpen" }
extern fn tui_close(term: Terminal) Void!Str = { go = "TuiClose" }
extern fn tui_clear(term: Terminal) Void = { go = "TuiClear" }
extern fn tui_draw_text(term: Terminal, x: Int, y: Int, text: Str) Void = { go = "TuiDrawText" }
extern fn tui_flush(term: Terminal) Void!Str = { go = "TuiFlush" }
extern fn tui_read_key(term: Terminal) Str!Str = { go = "TuiReadKey" }
```
