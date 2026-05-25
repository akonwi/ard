# Vaxis counter

The smallest Ard userland FFI demo in this repo.

Ard owns the counter state and event loop. `ffi.go` owns the Vaxis terminal boundary.

## Run

```sh
ard run main.ard
```

## Build

```sh
ard build --out counter main.ard
./counter
```

## Controls

- up/right/`k`/`l`/`+`: increment
- down/left/`j`/`h`/`-`: decrement
- `r`: reset
- `q` or ctrl-c: quit
