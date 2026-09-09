---
title: Embedded files
description: Package files and read-only filesystems directly into Ard programs.
---

Import the compiler-provided `ard/embed` module to package files into an
application. Embedded files are read while the program is checked and are
available at runtime without accessing the host filesystem.

## Embed one file

Use `text` for UTF-8 text and `bytes` for arbitrary data:

```ard
use ard/embed

let license: Str = embed::text("LICENSE")
let logo: [Byte] = embed::bytes("assets/logo.png")
```

Paths are relative to the Ard package root, including when the source expression
is in a nested module. They must be non-interpolated string literals.

`text` preserves the file exactly and reports a compile-time error when its
contents are not valid UTF-8. Each evaluation of `bytes` returns a fresh mutable
list.

## Embed a filesystem

Use `fs` with a static list of patterns to create an immutable embedded
filesystem:

```ard
use ard/embed

let assets = embed::fs([
  "public",
  "templates/*.html",
])
```

A directory pattern includes its complete subtree. Recursive directory
selection excludes names beginning with `.` or `_`; prefix a pattern with
`all:` to include them:

```ard
let all_assets = embed::fs(["all:public"])
```

Every pattern must match at least one file. Patterns and files cannot escape the
owning package, cross nested package boundaries, or select symlinks.

## Read embedded files

```ard
fn homepage() Str!Error {
  assets.read_text("public/index.html")
}

fn logo() [Byte]!Error {
  assets.read_file("public/logo.png")
}
```

An embedded filesystem provides:

- `read_file(path)` — returns a fresh `[Byte]`;
- `read_text(path)` — reads and validates UTF-8;
- `read_dir(path)` — returns immediate entries sorted by name;
- `stat(path)` — returns the name, file kind, and optional byte size;
- `sub(path)` — returns a filesystem rooted at an embedded directory.

Runtime paths are unrooted and slash-separated. `"."` denotes the filesystem
root. Missing and invalid runtime paths return `Error` values.

On the Go target, `embed::FS` implements `io/fs.FS`, so it can be passed directly
to compatible Go APIs such as `net/http.FS` and `template.ParseFS`.

:::caution
Embedded contents can be recovered from the resulting executable. Do not embed
passwords, tokens, private keys, or other secrets.
:::
