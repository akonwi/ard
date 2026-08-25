# 0067: Emit Opaque Go Struct Field Tags

## Status

Accepted

## Context

ADR 0064 added generic struct-field attribute syntax and the semantic `#json`
attribute. It intentionally deferred arbitrary Go struct tags because JSON
needed stable Ard semantics while raw backend metadata would be an escape hatch.

Direct Go interop also exposes reflection-based libraries whose schemas are
configured by struct tags. YAML, TOML, mapstructure, validators, database
mappers, and similar packages cannot consume an Ard-owned struct directly when
the generated Go field lacks their tag. Defining a Go-owned DTO and converting
it to Ard values adds boilerplate even when the field types are otherwise fully
representable.

The motivating case needs `yaml:"global_context"` on an Ard field. Adding a
YAML-specific semantic attribute would couple the compiler to one ecosystem
library without defining YAML behavior for Ard types such as `Maybe`, enums, or
unions. The requirement is instead an explicit Go-target escape hatch.

## Decision

A namespaced field attribute may attach an opaque Go struct tag to an Ard-owned
struct field:

```ard
struct QMDConfig {
  #go:yaml("global_context")
  global_context: Str,

  #go:validate("required")
  name: Str,
}
```

`#go:<key>` has these rules:

- `<key>` uses Ard identifier syntax and becomes the Go `reflect.StructTag` key.
- The attribute requires exactly one positional, non-interpolated static string.
- The string is the complete tag value. Ard does not interpret commas, options,
  or other library-specific syntax.
- Different keys may be attached to one field, but a key may appear at most
  once.
- `#go:json` is reserved. JSON names and behavior remain expressed through the
  compiler-owned automatic tag and semantic `#json` attribute.
- Other namespaces remain unknown attributes until separately specified.

The checker normalizes valid tags onto the canonical struct declaration.
Generic applications inherit their declaration's tags. AIR carries key/value
pairs explicitly as Go-targeted metadata rather than preserving parser nodes or
raw Go source.

The Go backend composes one struct tag whose entries are ordered as:

1. the compiler-owned `json` entry; then
2. custom entries sorted lexicographically by key.

For the example above, lowering emits:

```go
type QMDConfig struct {
    GlobalContext string `json:"global_context" yaml:"global_context"`
    Name          string `json:"name" validate:"required"`
}
```

Tag values are quoted as Go strings, and the complete struct-tag literal uses a
safe raw or interpreted Go string as appropriate. Attribute contents cannot
inject additional fields, tags, or Go syntax.

## Consequences

- Ard-owned structs can participate directly in reflection-based Go APIs
  without project-local DTOs used only for tags.
- The `go:` namespace makes backend coupling visible at each use site.
- Third-party tag semantics and errors remain owned by the consuming library.
  The Ard checker does not validate options such as YAML `omitempty` or
  validator `required`.
- Custom tags compose with renamed, omitted, and skipped JSON fields; they do
  not change whether a field is JSON-representable.
- Generic declarations emit the same metadata for every application.
- AIR gains narrowly scoped target metadata while preserving the frontend's
  normalized semantic boundary.
- This decision does not add YAML encoding or decoding, `Maybe` YAML behavior,
  custom tags on imported Go fields, attributes outside Ard-owned struct
  fields, or arbitrary Go declaration/source injection.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0054-represent-generic-structs-as-nominal-applications.md`
- `docs/adrs/0064-add-json-struct-field-attributes.md`
