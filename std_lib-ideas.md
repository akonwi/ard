# Stdlib ideas discovered while building blog SSG

These are string/list methods that don't exist in Ard's stdlib but came up as
useful while writing the static site generator in Ard.

## String methods

| Method | Purpose | Example |
|---|---|---|
| `str.ends_with(suffix)` | ✅ Added in v0.18.0 |
| `str.to_int()` | Parse string to Int | `"42".to_int() → 42` |
| `str.at(index) -> Str?` | ✅ Added in v0.18.0 (returns Maybe for bounds safety) |

## List methods

| Method | Purpose | Example |
|---|---|---|
| `list.join(sep)` | Join list elements into a string | `["a", "b"].join(", ") → "a, b"` |

## Control flow

| Feature | Purpose | Example |
|---|---|---|
| `if` as expression | Assign result of if/else | `let x = if cond { a } else { b }` |
| `match` on strings | ✅ Added in v0.18.0 |
| `continue` in loops | Skip to next iteration | `for x in list { if bad { continue } ... }` |
| `return` keyword | Early return from functions | `if bad { return err }` |

## Parser bugs

| Issue | Details |
|---|---|
| Function call with string literal arg inside `{interpolation}` hangs parser | ✅ Fixed in v0.18.0. `"{fn(\"arg\")}"` now works. |
