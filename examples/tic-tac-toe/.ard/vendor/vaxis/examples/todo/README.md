# Vaxis todo

A small Ard userland FFI demo using Vaxis for terminal rendering/input.

Ard owns todo state, selection, add/delete/toggle behavior, and rendering decisions. `ffi.go` is the whitelisted Go capability layer around Vaxis.

## Run

```sh
ard run main.ard
```

## Build

```sh
ard build --out todo main.ard
./todo
```

## Controls

- up/down or `k`/`j`: move selection
- enter: toggle selected item
- `e`: edit selected item
- `a`: add a new empty todo and start editing it
- `d`: delete selected item
- while editing: type text, space inserts a space, backspace deletes, enter saves, `q` cancels
- `r`: reset sample todos
- `q` or ctrl-c: quit
