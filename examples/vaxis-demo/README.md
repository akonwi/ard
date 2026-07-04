# Vaxis UI Demo

A full port of the [vaxis](https://git.sr.ht/~rockorager/vaxis) `ui` framework demo
(`_examples/ui/demo/main.go`) to Ard, exercising the direct Go FFI end to end:

- `use go:` imports of a third-party Go library (widgets, themes, events)
- Go struct literals, including generic ones (`ui::Radio<Str>`, `ui::Provider<ui::Theme>`)
- generic Go function calls (`ffi::StateRef<DemoState>`, `ui::MustDepend<ui::Theme>`)
- Ard closures as Go callbacks, named Go func/map types, named Go interfaces
- dynamic foreign type matching over `ui::Event` (`term::EventNotify(e) => ...`)
- optional Go variadic tails (`ui::Run(app)`)
- a small Go shim package (`ffi/`) for the few shapes Ard intentionally does not
  express: Go struct embedding (`ui.StateBase`), methods on string newtypes,
  `exec.Command` variadics, and functional options

## Requirements

The demo depends on the vaxis `ui` framework. The `go.mod` uses a `replace`
directive pointing at a sibling checkout:

```text
replace go.rockorager.dev/vaxis => ../../../vaxis
```

Adjust that path (or remove the replace once the `ui` package is published) to
match your checkout.

## Run

```sh
ard run main.ard
```

Keys: `n`/`p` switch pages, `Tab` moves focus, `Alt+k` opens the command
palette, `q` quits.
