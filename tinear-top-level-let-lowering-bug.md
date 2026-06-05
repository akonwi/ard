# Ard lowering bug: module-level `let` visible to checker but not lowerer

## Summary

A module-level `let` binding can pass `ard check`, but `ard build` fails during
lowering when a function references that binding, especially when the value is
captured for use in a closure.

This was encountered in `tinear` while trying to replace a zero-argument helper
function with a module-level string binding for a static custom event name.

## Expected

A module-level binding should be usable from functions in the same module and
capturable into closures.

## Actual

`ard check main.ard` succeeds, but `ard build main.ard --out ...` fails with:

```text
lower function main: lower function run: lower function init: lower method init: lower function init: lower function register_refresh_timer: unknown local refresh_event
```

## Repro

```ard
use ard/async
use ard/duration

let refresh_event = "inbox.refresh"

struct Context {}

impl Context {
  fn post(event: Str) {}
}

fn register_refresh_timer(ctx: Context) {
  let event = refresh_event
  async::start(fn() {
    while {
      async::sleep(duration::from_minutes(5))
      ctx.post(event)
    }
  })
}

fn main() {
  register_refresh_timer(Context{})
}
```

A typed binding behaves the same:

```ard
let refresh_event: Str = "inbox.refresh"
```

## Workaround

Use a zero-argument function instead of a module-level binding:

```ard
fn refresh_event() Str { "inbox.refresh" }

fn register_refresh_timer(ctx: Context) {
  let event = refresh_event()
  async::start(fn() {
    while {
      async::sleep(duration::from_minutes(5))
      ctx.post(event)
    }
  })
}
```

## Related note

Trying `const refresh_event = "inbox.refresh"` instead caused the checker to
panic with:

```text
panic: Cannot look up symbols in unrefined $unknown
```

So `const` was not a viable workaround here.
