# Vaxis tic-tac-toe

A larger dependency + FFI demo for Ard's Go target using the reusable `vaxis` Ard package.

The Ard program owns the tic-tac-toe game state and rules. Terminal rendering/input is provided by the `vaxis` dependency, which owns the Go FFI companion for [Vaxis](https://github.com/rockorager/vaxis).

## Run

From this directory, restore the locked dependency cache first:

```sh
ard deps fetch
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

## Dependency surface

`ard.toml` declares the direct Git dependency used by this example:

```toml
[dependencies]
vaxis = { git = "https://github.com/akonwi/vaxis-ard.git", commit = "76f7c1b" }
```

`ard.lock` records the resolved commit, dependency graph, and cache integrity. `ard deps fetch` materializes the locked package in Ard's shared cache; the project no longer checks in `.ard/vendor`.

Ard code imports and calls the dependency directly:

```ard
use vaxis

let term = vaxis::new("Ard Vaxis Tic-Tac-Toe").expect("open terminal")
vaxis::draw_text(term, 2, 1, "Hello")
vaxis::close(term).expect("close terminal")
```
