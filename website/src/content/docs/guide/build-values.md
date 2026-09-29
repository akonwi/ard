---
title: Build values
description: Embed typed application metadata at build time.
---

Build values provide application metadata such as versions, release channels,
and build numbers without relying on generated Go names or linker flags.

## Declare values

Declare values in the root project's `ard.toml`:

```toml
[build.values]
version = { type = "Str", default = "dev", release = true }
release_channel = { type = "Str", default = "local" }
build_number = { type = "Int", default = 0 }
experimental = { type = "Bool", default = false }
```

Build value names must be valid Ard identifiers. Supported types are `Str`,
`Int`, and `Bool`. Every value requires a default, which is used by ordinary
builds, `ard check`, `ard run`, `ard test`, and editor tooling.

Import the compiler-provided `ard/build` module to read the values:

```ard
use ard/build

fn version() Str {
  build::version
}
```

Build values are immutable. They belong to the root application; dependencies
cannot read the application's values.

## Override values

Pass an override for each value that release tooling supplies:

```sh
ard build main.ard \
  --define version=v1.2.3 \
  --define release_channel=stable \
  --define build_number=42 \
  --out example
```

`Str` values use all text after the first `=` and may be empty or contain more
`=` characters. `Bool` accepts exactly `true` or `false`. `Int` accepts signed
decimal values.

Unknown values, duplicate overrides, and values with the wrong type are errors.

## Release requirements

A declaration with `release = true` must have an explicit override when building
with `--release`:

```sh
ard build main.ard --release --define version=v1.2.3
```

The default does not satisfy this requirement, even when the value is unused.
An explicit override equal to the default does satisfy it. `--release` only
enforces this policy; it does not change optimization or target settings.

Ard does not inspect environment variables, Git state, timestamps, or hostnames.
Release tooling should collect that metadata and pass it explicitly.

## Go toolchain flags

Build values replace linker-stamped metadata such as `-ldflags -X`, which would
otherwise depend on generated Go names. Packaging options that do not change
program behavior, such as stripping symbols or removing local paths, belong to
the Go toolchain. The Go target runs `go build` with the current environment,
so pass them through `GOFLAGS`:

```sh
GOFLAGS="-trimpath -ldflags=-s" ard build main.ard \
  --release \
  --define version=v1.2.3 \
  --out example
```

Since Go 1.22, `-s` also omits DWARF debug information, as `-w` does. Quote
flags whose values contain spaces, for example
`GOFLAGS="'-ldflags=-s -w'"`. Setting `GOFLAGS` replaces any value already in
the environment, so include existing flags when release scripts extend it.
`GOFLAGS` also applies to `ard run`, `ard test`, and Go package resolution
during checking. Other Go environment variables such as `CGO_ENABLED`,
`GOOS`, and `GOARCH` pass through the same way.

Avoid `-ldflags -X` with Ard packages. Generated Go package paths and symbol
names are compiler implementation details; use build values instead.

:::caution
Build values are embedded in the resulting artifact. Do not use them for
passwords, tokens, or other secrets.
:::
