# 0064: Add JSON struct-field attributes

## Status

Accepted

## Context

Ard-owned structs already lower to exported Go fields with automatic `json` tags that preserve each original Ard field name. This makes ordinary Ard structs serializable through Go's JSON APIs, but source code cannot rename a field, omit an absent nullable field, or exclude an implementation-only field.

A raw Go struct-tag escape hatch would solve more cases, but it would make arbitrary backend metadata part of Ard's source model without static semantics. Go build constraints are also not merely emitted comments: faithful support requires source and module selection before checking. Neither belongs in this change.

The syntax should leave room for future compiler metadata without allowing attributes everywhere before a concrete use requires those targets.

## Decision

### Syntax and placement

A struct field may have prefix attributes using `#name(...)` syntax. Attributes are written one per line immediately before their field:

```ard
struct User {
  #json(name: "displayName", omit: none)
  display_name: Str?,
}
```

The parser represents attribute names, arguments, and static values generically, but attribute syntax is initially accepted only on Ard-owned struct fields. Attributes on declarations, parameters, enum variants, statements, expressions, struct literals, and imported Go types are syntax errors.

Attribute arguments may be positional or named, but the two forms cannot be mixed. Values are compile-time metadata: non-interpolated strings, signed integers, booleans, symbols, and lists of metadata values. The formatter puts each attribute on its own line and omits parentheses for an argument-free marker attribute.

Only registered attributes are accepted by the checker. Unknown attributes are errors rather than silently preserved metadata. `#json` is the only registered attribute in this decision.

### JSON field metadata

`#json` is nonrepeatable and accepts named arguments only:

- `name: Str` overrides the JSON object-member name for both marshaling and unmarshaling.
- `omit: none` omits an absent nullable field while retaining every present value, including present empty values.
- `skip: true` excludes the field from both marshaling and unmarshaling.

`name` and `omit` may be combined. `skip` is exclusive with both. `skip: false`, an argument-free `#json`, duplicate or unknown arguments, and `omit: none` on a non-nullable field are errors. Final non-skipped JSON names must be unique within a struct. A non-empty struct may not skip every field because `encoding/json/v2` rejects a non-empty Go struct with no JSON-representable fields.

Without `#json`, existing behavior remains unchanged: the generated JSON name is the original Ard field name and a `none` value is encoded as `null`.

The checker normalizes valid metadata onto the canonical struct declaration. Generic applications inherit it. AIR carries normalized JSON field options instead of parser-level attribute syntax.

### Go lowering

The Go backend lowers the options to validated `json` struct tags:

```ard
struct User {
  #json(name: "displayName", omit: none)
  display_name: Str?,

  #json(skip: true)
  password_hash: Str,
}
```

```go
type User struct {
    DisplayName ard.Maybe[string] `json:"displayName,omitzero"`
    PasswordHash string            `json:"-"`
}
```

`omit: none` uses `omitzero`. Ard's pointer-backed `Maybe` is zero only when it is `none`; a `some` value remains nonzero even when its payload is empty. This gives the same absence behavior through `encoding/json` and `encoding/json/v2`.

JSON names may be arbitrary valid strings, including `""`, `"-"`, commas, quotes, and Unicode. The backend uses Go's quoted JSON tag-name form when necessary and constructs a valid Go string literal rather than inserting source text directly.

### Deferred functionality

This decision does not add:

- arbitrary Go struct tags;
- `#go::build` or other Go target attributes;
- user-defined or silently ignored attributes;
- attributes on targets other than struct fields;
- general empty or zero omission;
- JSON inline fields, unknown-member capture, number stringification, case matching, formatting, required fields, defaults, or struct-wide naming strategies.

General empty omission is deferred because `encoding/json` and `encoding/json/v2` assign different meanings to `omitempty`. A future feature must define stable Ard semantics rather than accidentally depending on which Go JSON API consumes the generated type.

## Consequences

- Ard structs can describe their intended JSON wire names and absence behavior without raw backend metadata.
- Invalid metadata and conflicting wire names fail during checking instead of at JSON runtime.
- The parser and formatter gain a reusable attribute representation, while the accepted source surface remains intentionally field-only.
- AIR remains the semantic frontend/backend boundary by carrying normalized JSON options.
- Supporting Go build constraints later requires a separate compiler-aware module-selection design.
