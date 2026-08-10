# 0059: Add Raw Interpolated String Literals

## Status

Accepted

## Context

Ard has one string literal form. Double-quoted strings process character escapes,
support `{expression}` interpolation, and currently accept physical newlines.
Physical multiline source is not stable under formatting: the formatter
canonicalizes newline characters as `\n` escapes.

This makes several common values unnecessarily noisy:

- multiline SQL, HTML, shell snippets, and generated source;
- Windows paths and regular expressions containing backslashes;
- JSON and other text containing both quotes and braces.

Backtick-delimited strings provide a familiar visual convention for raw and
multiline text. Pure Go-style raw strings would have the smallest lexical model,
but would force concatenation whenever multiline text needs dynamic values.
Supporting interpolation is more useful for Ard's existing template-heavy code.

Raw interpolation originally risked creating a second way to escape literal
braces. Ard now uses doubled braces in ordinary strings and retains legacy
backslash-brace escapes only for source compatibility:

```ard
"Hello, {name}"     // interpolation
"Hello, {{name}}"   // literal braces
```

The same doubled-brace convention can therefore be used in backtick strings
without introducing another interpolation dialect.

## Decision

Add backtick-delimited raw interpolated string literals:

```ard
fn load(owner: Str) {
  let query = `
    SELECT id, name
    FROM users
    WHERE owner = '{owner}'
    `
}
```

The closing delimiter defines the source indentation margin, so the runtime
value begins with `SELECT`, not spaces.

"Raw" describes the literal text chunks. Interpolation expressions continue to
use ordinary Ard syntax and semantics.

### Raw text

Backslashes and double quotes have no special meaning in raw text:

```ard
let path = `C:\Users\{username}\files`
let pattern = `^\w+\s+\d+$`
```

The first value interpolates `username`; every backslash is preserved. The
second value contains the displayed backslashes literally.

Raw strings do not process `\n`, `\t`, `\\`, hexadecimal, Unicode, quote, or
other character escapes. Source characters are preserved as written after the
multiline margin and interpolation rules below are applied.

Backticks may be used for single-line or multiline values. A single-line raw
string preserves every content character exactly. Multiline mode begins when
the opening backtick is immediately followed by a physical newline. In
multiline mode:

1. the newline immediately after the opening backtick is not part of the value;
2. the closing backtick must appear on its own line, preceded only by
   indentation;
3. the newline immediately before that closing line is not part of the value;
4. the whitespace prefix before the closing backtick is the margin;
5. exactly that margin is removed from every nonblank raw-text line;
6. every nonblank raw-text line must begin with that margin, or the compiler
   emits a targeted indentation diagnostic;
7. indentation beyond the margin remains part of the value; and
8. whitespace-only lines are treated as blank and do not trigger the
   under-indentation diagnostic.

The margin is compared as a source-character prefix rather than as visual tab
columns. The formatter's canonical code indentation uses spaces. Physical
source line endings in multiline strings are normalized to LF, consistent with
Ard's canonical formatting contract.

A deliberate leading or trailing newline is represented by an additional blank
content line after the opener or before the closing line.

### Interpolation and literal braces

Both quoted and raw strings use the same brace grammar:

```ard
`Hello, {name}`       // interpolate name
`Hello, {{name}}`     // literal {name}
`{{{name}}}`          // a value surrounded by literal braces
```

The rules are:

- `{expression}` evaluates an ordinary Ard interpolation expression;
- `{{` contributes one literal `{`;
- `}}` contributes one literal `}`;
- a lone `}` remains literal text, matching existing quoted strings;
- an unclosed interpolation is a syntax error with a source range beginning at
  its opening brace.

Interpolation expressions are not raw. Quoted strings, escapes, nested calls,
and nested brace-delimited expressions inside an interpolation retain their
ordinary syntax.

Legacy `\{` and `\}` brace escapes remain accepted only in double-quoted
strings. In a backtick string those sequences contain a literal backslash before
the brace; they do not escape interpolation.

### Backtick delimiter

A backtick terminates the literal and cannot occur directly in a raw string.
Because backslashes are literal, `\`` is not a delimiter escape. Values requiring
a backtick use concatenation with an ordinary quoted string:

```ard
let markdown = `Run ` + "`ard test`" + ` to execute the tests.`
```

Ard does not initially add variable-length backtick delimiters, hash-delimited
raw strings, or a doubled-backtick escape.

### Ordinary strings

Double-quoted string semantics remain unchanged:

- character escapes continue to be processed;
- `{expression}` continues to interpolate;
- doubled braces remain the canonical literal-brace syntax;
- legacy backslash-brace escapes remain accepted;
- physical newlines remain accepted and formatter-canonicalized as `\n`.

This decision does not introduce `f`, `r`, or combined string prefixes and does
not change interpolation to Swift-style `\(expression)`.

### Representation and formatting

The parser must retain whether a string was quoted or raw. Raw literal chunks
lower to the same `Str` values as quoted chunks after brace processing, while
interpolated raw strings use the existing interpolated-string checking and
lowering semantics.

The formatter preserves raw syntax rather than converting it to a quoted
literal. It may shift the multiline margin and every corresponding raw-text
line together so the literal follows its enclosing code indentation without
changing the runtime value. Relative indentation beyond the margin, blank lines,
and trailing content whitespace remain semantic and must be preserved.

The formatter may format interpolation expressions, but margin normalization is
computed lexically from static raw-text lines before interpolated values are
inserted. A multiline interpolated value is inserted verbatim; its continuation
lines are not automatically indented and do not participate in margin
calculation. The formatter must not add or remove semantic leading or trailing
newlines. Doubled braces remain doubled when rendering literal braces in raw
strings.

### Diagnostics and tooling

The compiler should provide targeted diagnostics for:

- an unterminated raw string;
- an unterminated interpolation inside a raw string;
- malformed interpolation expressions.

Source locations must refer to source spelling rather than decoded value
positions. Tree-sitter, syntax highlighting, Zed support, formatter tests, and
LSP/parser location tests must be updated with the compiler grammar. The
Tree-sitter grammar should expose static raw-text chunks separately from Ard
interpolations so editor tooling can target literal regions without interpreting
embedded Ard expressions.

## Alternatives Considered

### Zig-style line prefixes

Zig prefixes each raw content line with `\\`, making indentation before the
marker nonsemantic and eliminating a closing delimiter. This gives exact,
formatter-safe control and permits delimiter characters in content. Ard does
not adopt it because every pasted line must be prefixed and interpolation across
line-oriented tokens would add a second structural model for strings.

### Embedded-language tags (deferred)

An explicit language tag could identify raw text for editor integrations:

```ard
let query = sql`
  SELECT id, name
  FROM users
  `
```

This would give Tree-sitter and editors such as Zed a reliable language-injection
signal without guessing from variable or function names. It is not part of this
decision because it adds syntax and semantics beyond the raw-string value model.
Untagged raw strings are sufficient for the initial feature; language tags may
be designed separately without changing their behavior.

### Exact whitespace preservation

Dart, Rust, Gleam, Kotlin, and Python preserve multiline source indentation by
default, sometimes paired with runtime dedent helpers. Ard rejects exact
preservation because normal code indentation would leak into values and prevent
the formatter from safely reindenting literals. Runtime dedent also occurs after
interpolation, allowing multiline inserted values to affect indentation
calculation.

### Common-minimum indentation

Automatically removing the smallest indentation shared by all lines is concise,
but nonlocal: adding one less-indented line changes every other line's value.
The closing-delimiter margin is explicit and stable under local edits.

## Consequences

- Ard gains an explicit, formatter-stable multiline string form.
- Paths, regular expressions, JSON, SQL, and generated text require fewer
  backslash and quote escapes.
- Raw and quoted strings share one interpolation and literal-brace grammar.
- Backtick strings are not completely literal because interpolation and doubled
  braces remain recognized; documentation must call them raw interpolated
  strings rather than imply Go-identical semantics.
- Closing-delimiter margins let raw strings follow surrounding indentation
  without adding runtime whitespace.
- Under-indented nonblank lines become compile-time errors rather than silently
  changing the value.
- Multiline interpolated values are inserted verbatim and may require explicit
  indentation by the caller.
- Backticks inside raw content require concatenation.
- The lexer, AST, parser locations, formatter, Tree-sitter grammar, highlighting,
  and documentation require coordinated changes; checker, AIR, and backend
  changes should remain small.

## Related

- GitHub issue #312: Design explicit raw and multiline string literals
- Commit `4860699c`: use doubled braces for string escapes
- ADR 0026: Add Byte and Rune Primitives
