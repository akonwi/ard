---
title: Formatting
description: The Ard code style enforced by the formatter.
---

Ard uses a canonical formatter style. Run the formatter instead of manually styling code.

## Command

```bash
ard format <file-or-dir>
ard format --check <file-or-dir>
```

- `format` rewrites files in place
- `--check` reports files that are not formatted

## Core Style Rules

- line width target: `100`
- indentation: `2` spaces
- braces: K&R (`if cond { ... }`)
- binary operators use spaces: `a + b`, `x == y`
- range operator has no spaces: `0..10`
- colon spacing: `name: Type`, `[Str: Int]`

## Lists, Maps, Calls, and Params

- multiline collections and calls include trailing commas
- wrapped function parameters are one per line
- empty map literal is `[:]`

## Struct Literals

- `0-2` properties may stay on one line if they fit
- `3+` properties are formatted as multiline

```ard
// one line
Point{x: 1, y: 2}

// multiline
User{
  name: "A",
  age: 1,
  role: "admin",
}
```

## Match and Try Blocks

- match arms are comma-separated
- if a match arm body is a single expression and fits, it can stay inline

```ard
match ok {
  true => { total =+ 1 },
  false => { total =- 1 },
}
```

- `try ... -> var { ... }` catch blocks with a single expression stay inline when they fit

```ard
let raw = try self.raw -> _ { "" }
```

## Imports

`use` statements are grouped and sorted:

1. `ard/*`
2. absolute package paths
3. relative paths (`./`, `../`)

## Comments and Spacing

- formatter preserves blank-line gaps using source locations (capped to one blank line)
- comments are kept conservatively and aligned to nearby nodes
