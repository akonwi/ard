---
title: ard/embed
description: Compile files and immutable filesystems into Ard programs.
---

The compiler-provided `ard/embed` module captures package files while checking
and includes their contents in the generated program. It does not read the host
filesystem at runtime.

```ard
use ard/embed

let license = embed::text("LICENSE")
let logo = embed::bytes("assets/logo.png")
let public = embed::fs(["public"])
```

Constructor arguments are static, non-interpolated string literals. Paths are
slash-separated and resolve from the root of the Ard package containing the
constructor—not from the source file's directory or the process working
directory.

## Constructors

### `text(path: Str) Str`

Embed one UTF-8 text file. The returned string preserves the file contents
exactly, including line endings and a trailing newline. Checking fails if the
file is not valid UTF-8.

```ard
use ard/embed

let template = embed::text("templates/page.html")
```

### `bytes(path: Str) [Byte]`

Embed one file as bytes. Each evaluation returns a fresh list, so mutating one
result does not modify a later result.

```ard
use ard/embed

fn icon() [Byte] {
  embed::bytes("assets/icon.png")
}
```

### `fs(patterns: [Str]) embed::FS`

Embed files selected by a non-empty static list of patterns and return an
immutable filesystem.

```ard
use ard/embed

let assets = embed::fs([
  "public",
  "templates/*.html",
  "all:public/.well-known",
])
```

Patterns use slash-separated Go-style path matching:

- `*` matches within one path segment;
- `?` matches one non-separator character;
- character classes such as `[a-z]` are supported;
- a directory match includes its complete subtree;
- recursively selected names beginning with `.` or `_` are excluded by default;
- the `all:` prefix includes those hidden names; and
- recursive `**` patterns are not supported.

Every pattern must select at least one regular file. Overlapping matches are
deduplicated. Paths cannot escape the package, cross a nested Ard package or Go
module boundary, or select symbolic links and reserved directories.

## `embed::FS`

`embed::FS` is an immutable, opaque filesystem. Runtime paths are unrooted and
slash-separated; `"."` denotes the filesystem root. Invalid or missing paths
return `Error` values.

### `read_file(path: Str) [Byte]!Error`

Read a file as a fresh byte list.

### `read_text(path: Str) Str!Error`

Read a file as text. Returns an error when its contents are not valid UTF-8.

### `read_dir(path: Str) [embed::DirEntry]!Error`

Read the immediate children of a directory, sorted by name.

### `stat(path: Str) embed::FileInfo!Error`

Return metadata for a file or directory.

### `sub(path: Str) embed::FS!Error`

Return a filesystem rooted at an embedded directory. The path must identify a
directory.

```ard
use ard/embed

let public = embed::fs(["public"]).sub("public").expect("embedded public files")
let index = public.read_text("index.html").expect("embedded index")
```

On the Go target, `embed::FS` implements `io/fs.FS` and can be passed directly
to compatible Go APIs:

```ard
use ard/embed
use go:net/http

let public = embed::fs(["public"]).sub("public").expect("embedded public files")
let handler = http::FileServer(http::FS(public))
```

## Supporting types

### `embed::DirEntry`

| Field | Type | Description |
| --- | --- | --- |
| `name` | `Str` | Entry name relative to the directory being read. |
| `is_dir` | `Bool` | Whether the entry is a directory. |

### `embed::FileInfo`

| Field | Type | Description |
| --- | --- | --- |
| `name` | `Str` | Base name, or `"."` for the filesystem root. |
| `is_dir` | `Bool` | Whether the path identifies a directory. |
| `size` | `Int?` | File size in bytes; `none` for directories. |

## Resource limits

- 16 MiB per file
- 64 MiB per embedded filesystem
- 128 MiB and 10,000 selected files per program

Embedded contents are recoverable from the executable. Do not embed passwords,
tokens, private keys, or other secrets.

For patterns, generated artifacts, and a complete example, see
[Embedded files](/guide/embedding/).
