# Vaxis tic-tac-toe

A Go-FFI demo for Ard's Go target. The Ard program owns the tic-tac-toe game
state and rules; terminal rendering and input come from the
[Vaxis](https://git.sr.ht/~rockorager/vaxis) Go library, called directly through
the project's own Go code — no Ard wrapper package in between.

## Run

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

## How the FFI works

The Ard program calls Vaxis two ways. Most of it is **direct**: it imports the
Go library and calls it like any other Go package, with `*vaxis.Vaxis` appearing
in Ard as `mut vaxis::Vaxis`.

```ard
use go:git.sr.ht/~rockorager/vaxis as vaxis

fn draw_board(term: mut vaxis::Vaxis, ...) {
  term.Window().Clear()   // direct method calls
  term.Render()
}
```

The handful of things that genuinely need Go live in the project's own package
`ffi/`, declared by `go.mod` as `module tic_tac_toe` and imported with the
`use go:<project>/ffi` shorthand (where `<project>` is the Ard project name, which
is also the Go module path):

```go
// ffi/host.go
package ffi

import "git.sr.ht/~rockorager/vaxis"

// New constructs vaxis.Options (a partial struct literal Go allows but Ard does not).
func New(title string) (*vaxis.Vaxis, error) { /* ... */ }

// DrawText builds vaxis.Cell values per grapheme.
func DrawText(vx *vaxis.Vaxis, x, y int, text string) { /* ... win.SetCell ... */ }

// ReadKey type-switches the vaxis.Event interface, which Ard cannot express.
func ReadKey(vx *vaxis.Vaxis) (string, error) { /* range vx.Events() ... */ }
```

```ard
use go:tic_tac_toe/ffi

let term = ffi::New("Ard Vaxis Tic-Tac-Toe").expect("open terminal")
ffi::DrawText(term, 2, 1, "Hello")
```

Go shapes map to Ard at the boundary: `(*vaxis.Vaxis, error)` becomes
`(mut vaxis::Vaxis)!Str`, `(string, error)` becomes `Str!Str`. The compiler wires
the project's Go module into the generated build (`require` + `replace`), so
Vaxis is fetched as an ordinary Go dependency — there is no separate Ard
dependency.
