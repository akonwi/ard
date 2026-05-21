# Vaxis tic-tac-toe

A userland Ard Go-FFI demo using [Vaxis](https://github.com/rockorager/vaxis).

The Ard program owns the tic-tac-toe game state and rules. `ffi.go` is the project-owned Go boundary for terminal rendering and input.

## Run

```sh
cd tic-tac-toe
ard run main.ard
```

## Build

```sh
cd tic-tac-toe
ard build --out ttt main.ard
./ttt
```
