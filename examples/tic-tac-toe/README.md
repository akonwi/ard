# Vaxis tic-tac-toe

A larger dependency + FFI demo for Ard's Go target using the reusable `vaxis` Ard package.

The Ard program owns the tic-tac-toe game state and rules. Terminal rendering/input is provided by the `vaxis` dependency, which owns the Go FFI companion for [Vaxis](https://github.com/rockorager/vaxis).

## Run

From this directory, materialize dependencies first:

```sh
ard deps fetch
ard run main.ard
```

Or build:

```sh
ard deps fetch
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

## Dependency surface

`ard.toml` declares the local path dependency used by this example:

```toml
[dependencies]
vaxis = { git = "git@github.com:akonwi/vaxis-ard.git", commit = "76f7c1b" }
```

Ard code imports and calls the dependency directly:

```ard
use vaxis

let term = vaxis::new("Ard Vaxis Tic-Tac-Toe").expect("open terminal")
vaxis::draw_text(term, 2, 1, "Hello")
vaxis::close(term).expect("close terminal")
```
