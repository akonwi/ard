---
title: Strings
---

Ard's `Str` type has quoted and raw interpolated literals. Both forms use
`{expression}` for interpolation and doubled braces for literal braces.

## Quoted strings

Double-quoted strings process character escapes:

```ard
let greeting = "Hello, {name}!\n"
let literal_braces = "{{name}}"
```

Use quoted strings when escapes such as `\n`, `\t`, and `\\` are convenient.

## Raw strings

Backtick-delimited strings preserve backslashes, double quotes, and other text
without processing character escapes:

```ard
let path = `C:\Users\{username}\files`
let pattern = `^\w+\s+\d+$`
```

The path interpolates `username`. Its backslashes remain literal; a backslash
does not disable interpolation.

Raw strings use the same brace rules as quoted strings:

```ard
let greeting = `Hello, {name}!`
let literal_braces = `{{name}}`
let wrapped = `{{{name}}}`
```

These evaluate to `Hello, Ada!`, `{name}`, and `{Ada}` when `name` is `Ada`.

## Multiline raw strings

A raw string enters multiline mode when its opening backtick is immediately
followed by a newline:

```ard
let query = `
  SELECT id, name
  FROM users
  WHERE owner = '{owner}'
  `
```

The indentation before the closing backtick defines the margin removed from
each nonblank content line. The value above begins with `SELECT` and has no
trailing newline.

Indentation beyond the closing margin is preserved:

```ard
let text = `
  first
    nested
  `
```

Its value is `first\n  nested`. A nonblank line indented less than the closing
margin is a compile error.

The newline after the opening backtick and the newline before the closing line
are delimiters, not content. Add an extra blank line when the value needs a
leading or trailing newline.

## Limitations

A raw string cannot contain a backtick directly because backslashes are
literal. Concatenate a quoted backtick when needed:

```ard
let markdown = `Run ` + "`ard test`" + ` to execute the tests.`
```
