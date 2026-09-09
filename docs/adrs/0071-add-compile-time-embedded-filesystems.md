# 0071: Add Compile-Time Embedded Filesystems

## Status

Accepted

## Context

Applications commonly ship templates, static web assets, schemas, migrations,
certificates, and other files inside the executable. Ard can currently do this
through a project-local Go FFI package containing `//go:embed`, but that forces a
pure Ard application to add a Go shim solely for build-time packaging.

Go's embedding model has two useful parts:

- a build-time selection language for exact files, patterns, and directory
  trees; and
- an immutable `embed.FS` value implementing `io/fs.FS` for runtime lookup.

The Go directive itself is not an appropriate Ard source feature. Ard is its own
language, AIR is its target-neutral backend boundary, and future targets must be
able to implement the same behavior without parsing or emulating Go source.
Still, the Go backend should use native `//go:embed` output because it stores
large resources efficiently and interoperates with the Go filesystem ecosystem.

Embedding also introduces compiler inputs that are not `.ard` files. Checking,
AIR lowering, generated artifacts, dependency ownership, and LSP invalidation
must all use the bytes captured for a resource during that analysis. A backend
must not reread the original source file after checking, because the resulting
binary could then contain bytes different from those that were validated.

ADR 0049 reserves exact compiler-owned paths under `ard/*`. ADR 0069 establishes
a related precedent: build-time data should be explicit, typed, deterministic,
and carried through the checker-to-AIR pipeline rather than discovered from
ambient process state.

## Decision

Reserve `ard/embed` as a compiler-owned intrinsic module. It provides exact-file
embedding and a statically constructed immutable filesystem:

```ard
use ard/embed

let license: Str = embed::text("LICENSE")
let logo: [Byte] = embed::bytes("assets/logo.png")
let assets: embed::FS = embed::fs([
  "public",
  "templates/*.html",
  "all:public/.well-known",
])
```

These constructors execute during checking, not at runtime. They require static
arguments, capture the selected contents in AIR, and never return `Result`.
Runtime filesystem operations on `embed::FS` do return `Result`.

### Exact-file constructors

`ard/embed` exposes:

```ard
embed::text(path: Str) Str
embed::bytes(path: Str) [Byte]
```

The argument must be exactly one positional, non-interpolated string literal.
Computed strings, interpolation, named arguments, and references to the
constructor as a function value are rejected. Exact-file paths do not interpret
pattern metacharacters or the `all:` prefix. They must name one regular file;
files beginning with `.` or `_` may be selected explicitly when the rest of the
portable path rules permit them.

`text` requires the selected file to contain valid UTF-8 and preserves its bytes
exactly. It does not normalize newlines, strip a byte-order mark, or otherwise
rewrite the contents. Invalid UTF-8 is a compile-time diagnostic; `bytes` is the
alternative for arbitrary data.

Each evaluation of `bytes` produces fresh mutable list storage. Mutating one
result cannot modify another result or the compiler-owned embedded bytes. A
module-level binding is initialized once and thereafter follows ordinary Ard
list sharing and mutability rules. `Str` remains immutable.

### Embedded filesystem constructor

`ard/embed` also exposes:

```ard
embed::fs(patterns: [Str]) embed::FS
```

The argument must be a non-empty list literal containing only
non-interpolated string literals. Computed lists, spreads, and references to the
constructor as a function value are rejected. The selected filesystem is the
union of all pattern matches. Overlapping and repeated patterns are permitted;
files are stored once by their logical path. Every pattern must independently
match at least one selectable file.

The compiler expands every pattern and reads every selected file while checking.
The resulting file set and bytes are part of the checked program and AIR. The
constructor performs no runtime I/O and cannot fail at runtime.

### Resource ownership and path base

Constructor paths and patterns are relative to the root of the Ard package that
owns the module containing the constructor call. They are not relative to the
`.ard` file, compiler process working directory, generated output directory, or
consuming application.

For example:

```text
widgets/
├── ard.toml
├── assets/
│   └── icon.svg
└── ui/
    └── button.ard
```

A call in `widgets/ui/button.ard` uses:

```ard
embed::text("assets/icon.svg")
```

When `widgets` is consumed as a dependency, that path still resolves against the
selected `widgets` package root. A dependency cannot read or be overridden by a
similarly named file in the consuming project. Git dependencies read resources
from their locked checkout; path dependencies read them from their declared
source root.

Using package-root-relative paths follows Ard's absolute, package-oriented
module model from ADR 0013 and ensures that moving an `.ard` module does not
silently change the resource it selects.

### Pattern language

Ard's pattern language is based on Go embed patterns, with explicit Ard-defined
differences: paths are package-root-relative rather than Go-package-relative,
backslashes are rejected instead of acting as escapes, and nested Ard package
boundaries are enforced in addition to Go module boundaries. The compiler
implements selection itself; it does not ask the active Go toolchain to decide
which source files belong to an AIR resource set.

Patterns have these rules:

- `/` is the separator on every platform.
- Pattern matching otherwise follows Go `path.Match`; `*`, `?`, and character
  classes do not cross `/` separators. Malformed character classes are errors.
- A path naming a directory selects its complete subtree recursively.
- Recursive directory selection excludes files and directories whose names
  begin with `.` or `_`. This exclusion does not apply to an explicit wildcard
  match: for example, `images/*` may select `images/.thumb`, while `images`
  does not.
- The `all:` prefix includes otherwise excluded names during recursive directory
  selection.
- Patterns cannot be empty or absolute and cannot contain empty, `.`, or `..`
  path segments.
- Backslashes are rejected in constructor patterns rather than acting as host
  separators or pattern escapes.
- Symlinks and non-regular files are not selectable.
- Selection cannot escape the owning Ard package or cross into a nested Ard
  package boundary, a nested directory containing `go.mod`, or a `vendor`
  directory. A `go.mod` at the owning package root does not block selection, but
  a selected logical file named `go.mod` at any depth is rejected because its
  generated copy would create a nested Go module boundary.
- Version-control directories `.bzr`, `.git`, `.hg`, and `.svn` are not
  selectable.
- Every selected path element must be valid UTF-8. Allowed characters are
  Unicode letters, ASCII digits, ASCII space, and the ASCII punctuation
  `!#$%&()+,-.=@[]^_{}~`. An element cannot consist only of dots or end in a
  dot. Its case-insensitive prefix before the first dot cannot be a Windows
  reserved device name: `CON`, `PRN`, `AUX`, `NUL`, `COM1` through `COM9`, or
  `LPT1` through `LPT9`. These fixed portable-name rules intentionally match
  Go's `module.CheckFilePath` at the time of this decision; the implementation
  may use that helper only alongside regression tests preserving Ard's fixed
  contract.
- Empty directories are not represented. The root and every ancestor directory
  of a selected file are synthesized in the embedded filesystem.

These rules are fixed Ard semantics. Other targets must reproduce them even if
their host language has a different resource or glob implementation.

### Runtime filesystem API

`embed::FS` is an immutable compiler-owned nominal type. Conceptually, the
intrinsic module declares the following Ard API; the qualified names shown at
use sites come from importing the module, not from qualified declaration syntax:

```ard
// FS is a compiler-owned opaque type with no literal syntax.

struct DirEntry {
  name: Str,
  is_dir: Bool,
}

struct FileInfo {
  name: Str,
  is_dir: Bool,
  size: Int?,
}

impl FS {
  fn read_file(path: Str) [Byte]!Error
  fn read_text(path: Str) Str!Error
  fn read_dir(path: Str) [DirEntry]!Error
  fn stat(path: Str) FileInfo!Error
  fn sub(path: Str) FS!Error
}
```

`FS` has no public struct literal or empty constructor. Only `embed::fs` and
`FS.sub` construct values. It may be stored in structs, `Maybe`, `Any`, and
generic values, but it has no equality or ordering operations, is not a valid
map key, and has no automatic JSON representation.

Runtime lookup paths follow `io/fs.ValidPath` semantics:

- `"."` denotes the filesystem root;
- other paths are non-empty, unrooted, and slash-separated;
- empty, `.` and `..` elements are invalid;
- leading or trailing slashes are invalid; and
- backslash is an ordinary character, never a path separator.

`read_file` requires a file and returns a fresh mutable byte list. The caller may
modify it without affecting the filesystem or later reads. `read_text` requires
a file and validates UTF-8 at runtime because its path may select any file in a
mixed binary/text filesystem. It otherwise preserves contents exactly.

`read_dir` requires a directory, returns only its immediate children, and sorts
them lexicographically by filename. Its returned list is fresh and mutable.
`DirEntry.name` is the basename, not a full path.

`stat` accepts files, synthesized directories, and `"."`. A regular file
reports `some(byte_count)` for `size`; a directory reports `none`.
`stat(".").name` is `"."`, including on a filesystem returned by `sub`.
Sizes are guaranteed to fit Ard `Int` by the compile-time resource limits.
Embedded filesystems do not expose permissions, ownership, timestamps, devices,
or symlinks because those values are absent or intentionally not part of
reproducible embedding.

`sub` eagerly verifies that its path names an existing synthesized directory,
then returns an immutable view rooted there. The Go backend therefore performs
`fs.Stat` and an `IsDir` check before `fs.Sub`, whose own fallback can otherwise
validate lazily. `"."` in the returned filesystem denotes the subtree root. The
view shares immutable embedded storage with its parent.

Copies of `embed::FS` are small handles sharing immutable content. Its methods do
not require a mutable receiver and are safe for concurrent use. Taking an Ard
mutable reference to the handle does not grant interior mutation operations.

### Errors

Constructor failures are compile-time diagnostics located at the invalid path or
pattern. Runtime operations return the builtin `Error` type.

On the Go target, filesystem errors preserve Go error identity as required by
ADR 0063. In particular, callers using direct Go interop can test errors against
`io/fs.ErrInvalid`, `io/fs.ErrNotExist`, and related sentinels through
`errors.Is`. `read_text` wraps invalid UTF-8 as a path error whose cause is
`io/fs.ErrInvalid`. The operation and logical path remain available through a
Go `*fs.PathError` where applicable, but exact human-readable error messages are
not a stable language contract.

### Go filesystem interoperability

On the Go target, `embed::FS` lowers directly to an `io/fs.FS` interface value.
Its initial dynamic value is derived from Go `embed.FS`; `sub` may produce the
Go filesystem implementation returned by `fs.Sub`. Ard's nominal identity is a
checker and AIR guarantee and does not require a generated Go wrapper type.
Interface copies provide the specified small shared handle, and there is no nil
or zero `embed::FS` value constructible from safe Ard.

This representation intentionally satisfies imported `io/fs.FS`.
Compiler-owned intrinsic types may declare this built-in foreign-interface
bridge without a source `impl`:

```ard
use ard/embed
use go:net/http

let assets = embed::fs(["public"])
let web_filesystem = http::FS(assets)
```

This also permits use with APIs such as `template.ParseFS`. The language
contract promises `io/fs.FS` compatibility on the Go target. It does not promise
that direct type assertions to optional Go interfaces such as `io/fs.ReadFileFS`
or `io/fs.ReadDirFS` succeed, even when a backend value happens to implement
them. Pure Ard code uses the methods declared by `embed::FS`.

On non-Go targets, `embed::FS` retains the same Ard API and semantics using a
target-specific immutable representation. Go interface compatibility is an
explicit target interop property, not the definition of the Ard type.

### Resource limits

The compiler must protect CLI and language-server processes from accidental or
malicious resource expansion. Initial limits are:

- 16 MiB per file;
- 64 MiB per embedded filesystem;
- 128 MiB across one checked program; and
- 10,000 files across one checked program.

The per-file limit applies to every selected logical file. The per-filesystem
limit sums each unique logical path in that set, even when different paths have
identical bytes. Identical filesystem sets are interned by owner plus their
sorted path/blob mapping. Program totals are defined entirely over checked
resource identities: each unique set contributes its entry count and the sum of
its entries' blob sizes, while exact constructors contribute each distinct
referenced blob once. A blob referenced both directly and from a set counts in
both categories, as do entries in distinct non-identical sets. This conservative
accounting is independent of backend deduplication and prevents overlapping sets
from hiding artifact growth through content hashes.

Checking applies these ceilings to all source expressions it validates so a
test-only declaration cannot exhaust the compiler. Production AIR and artifacts
omit resources referenced only by declarations excluded under the existing
`IncludeTests` policy; test AIR retains them. Diagnostics report the observed
and allowed values. These ceilings may be raised compatibly in future releases;
configurable limits and application packaging of larger resource trees are
deferred.

Embedded data is recoverable from the resulting artifact. Documentation must
warn users not to embed passwords, tokens, private keys, or other secrets and
must note that redistribution and license obligations apply to embedded files.

## Checker and project loading

The checker recognizes `ard/embed` by exact intrinsic identity, not merely by a
local import alias or function name. It constructs checked embedded-resource
expressions after validating static arguments and delegates package ownership,
path containment, and dependency-root resolution to project/module loading.
The parser does not read files.

Resource collection is invocation-local. Process-wide embedded standard-library
caches must not retain application resources. Repeated references may share
captured immutable content internally, but their source expressions and runtime
value semantics remain distinct.

Diagnostics cover at least:

- non-static constructor arguments;
- an empty filesystem pattern list;
- malformed, absolute, or traversing paths;
- a pattern that matches nothing;
- unreadable files;
- symlinks and non-regular files;
- package-boundary escapes;
- invalid path-name UTF-8 or unsupported names;
- invalid file UTF-8 for `text`;
- resource limits; and
- intrinsic constructors used as function values.

Runtime invalid paths and missing entries are values in `Result`, not compiler
diagnostics.

## AIR representation

AIR carries resources independently from their source filesystem and separates
content identity from logical filesystem membership:

```text
Program.EmbeddedBlobs []EmbeddedBlob
Program.EmbeddedSets  []EmbeddedSet

EmbeddedBlob {
  ID
  Data []byte
  Digest
}

EmbeddedSet {
  ID
  OwnerPackageIdentity
  Entries []EmbeddedEntry
  Digest
}

EmbeddedEntry {
  Path
  BlobID
}
```

`OwnerPackageIdentity` is the canonical root/dependency package identity
supplied by project loading and used for ownership and interning; it is not a
filesystem root or process-local checker pointer. Pattern spelling, source
module identity, and call-site attribution remain on checked expressions and AIR
source locations rather than on interned sets, so different constructors that
select identical contents can share one set deterministically. Project loading
and checking are solely responsible for proving filesystem containment before
AIR is produced. A set contains only files; its root and ancestor directories
are derived deterministically from entry paths.

It adds a target-neutral embedded-filesystem type and explicit operations:

```text
TypeEmbeddedFS
ExprEmbeddedText
ExprEmbeddedBytes
ExprMakeEmbeddedFS
ExprEmbeddedReadFile
ExprEmbeddedReadText
ExprEmbeddedReadDir
ExprEmbeddedStat
ExprEmbeddedSub
```

Exact-file expressions refer to blob IDs and filesystem expressions refer to
set IDs rather than expanding bytes into thousands of integer literal nodes.
Entries are sorted by logical path and deduplicated; sets may share immutable
blobs while retaining distinct paths. Digests make resource identity and
generated names deterministic; contents remain authoritative.

AIR validation checks blob and set IDs, sorted unique logical paths, content
digests, derived file/directory conflicts, normalized stable package identity,
resource limits, and each operation's result type. It does not attempt to
re-prove historical filesystem containment. Serialized AIR includes captured
contents so backend generation never depends on the original source tree.

## Go lowering and generated artifacts

The Go backend uses native `//go:embed` over compiler-staged copies of the AIR
resources. It does not place user patterns directly in generated directives.

Generated output contains a compiler-owned package such as
`internal/ardembed`. For an embedded filesystem it emits a shape equivalent to:

```go
package ardembed

import (
    "embed"
    "io/fs"
)

//go:embed all:data/a1b2c3
var rawA1B2C3 embed.FS

var SetA1B2C3 fs.FS = mustSub(rawA1B2C3, "data/a1b2c3")
```

The backend materializes the exact captured entries beneath the generated set
path, then uses `fs.Sub` so user-visible runtime paths do not contain the
compiler-owned prefix. A failed generated `fs.Sub` is an internal compiler
layout invariant and may panic during generated package initialization.

Exact text and byte resources use unexported generated string variables
initialized by safe compiler-owned directives. Because generated Ard modules
are separate Go packages, the resource package exposes compiler-generated Go
accessors such as `TextA1B2C3() string` and `BytesA1B2C3() []byte`; these names
are not visible as Ard declarations. The byte accessor performs a conversion or
copy at each call to guarantee fresh mutable storage.

Filesystem operations lower through `io/fs` helpers such as `fs.ReadFile`,
`fs.ReadDir`, `fs.Stat`, and `fs.Sub`, with explicit conversion into Ard's
`Result`, `DirEntry`, and `FileInfo` representations. `read_file` must preserve
the fresh-copy contract even if the underlying target helper does not.

Generated resources should be centralized rather than duplicated into every Go
package corresponding to an Ard module. Generated Ard packages import the
compiler-owned resource package, which has no dependency back to user modules
and therefore cannot create import cycles.

The backend artifact model expands from Go source files to a bundle containing
both generated sources and ancillary resource files. Build, run, and test
workflows write the complete bundle before invoking Go. APIs or tests that need
only rendered Go may continue to expose a source-only view, but such a view is
not a complete buildable artifact for programs using embedding.

## Language server and incremental analysis

Embedded files and pattern match sets are semantic compiler inputs. LSP analysis
signatures include:

- constructor path and pattern spelling;
- sorted matched logical paths;
- each selected file's content digest; and
- missing or invalid match state.

The language server dynamically registers `workspace/didChangeWatchedFiles` for
exact files and every directory traversed by directory or pattern selection.
Directory registrations include create/delete events so a newly matching file
invalidates analysis. Clients that do not support dynamic watched-file
registration still receive correct results when another event triggers analysis,
but cannot receive immediate diagnostics for an otherwise invisible external
resource edit. Changes to unrelated files should retain cached analysis where
practical; conservative invalidation of a traversed directory is acceptable
initially.

Each selected file is read once into the checked resource snapshot, and backend
generation uses only those captured bytes. Ordinary filesystem APIs cannot
provide an atomic snapshot of a changing directory tree, so the compiler does
not promise one; cancelled or superseded LSP analyses must not publish their
results.

## Implementation sequence

1. Register `ard/embed`, its static constructors, `embed::FS`, and supporting
   value types in the checker.
2. Add package-owned path and pattern resolution with containment, symlink,
   UTF-8, and resource-limit diagnostics.
3. Add checked resource nodes, AIR tables/types/operations, serialization, and
   strict validation.
4. Introduce a generated artifact bundle and stage captured resources.
5. Generate the compiler-owned Go resource package and native `//go:embed`
   directives.
6. Lower exact-file values and filesystem methods, including fresh byte copies
   and preserved errors.
7. Add the built-in `io/fs.FS` interop bridge.
8. Include resource dependencies in CLI and LSP invalidation.
9. Document patterns, package ownership, limits, generated-artifact visibility,
   and the embedded-secret warning.

## Test plan

Checker tests cover exact files, binary and UTF-8 content, pattern syntax,
directories, `all:`, overlapping matches, package-root resolution, locked and
path dependencies, traversal, symlinks, nested package boundaries, unreadable
or unsupported files, static-argument restrictions, limits, method signatures,
and the Go filesystem bridge.

AIR tests cover deterministic resource tables, content capture, serialization,
deduplication, embedded filesystem typing, operation payloads, malformed IDs,
bad hashes, conflicting paths, and resource limits.

Go backend tests cover generated directives and ancillary files, hidden-file
selection, prefix removal through `fs.Sub`, exact runtime contents, fresh byte
lists, directory ordering, stat and sub behavior, invalid and missing path
errors, `errors.Is` identity, runtime UTF-8 validation, direct use with an API
accepting `io/fs.FS`, deterministic output, and run/build/test parity. Backend
generation must still succeed from captured AIR after the original resource is
changed or removed.

LSP tests cover edits, deletion, recreation, newly matching pattern entries,
dependency resource changes, cancellation, excluded and unrelated files, and
cache reuse when resource inputs are unchanged.

## Deferred functionality

This decision does not add:

- runtime filesystem access outside the captured set;
- environment-, Git-, timestamp-, or build-tag-dependent selection;
- compression, MIME inference, or content transformation;
- writable embedded filesystems;
- preservation of permissions, ownership, or timestamps;
- empty-directory entries;
- custom pattern dialects such as recursive `**`;
- manifest-named resource bundles;
- configurable resource limits; or
- guaranteed Go optional-interface conformance beyond `io/fs.FS`.

## Consequences

- Pure Ard applications can package individual files and directory trees without
  project-local Go FFI shims.
- Embedded resources are explicit, typed, deterministic build inputs.
- Dependencies retain ownership of their resources and cannot inspect consumer
  files.
- The Go target interoperates naturally with `io/fs` consumers while Ard
  semantics remain target-neutral.
- AIR and generated artifacts become larger because they contain captured file
  contents.
- The backend must support ancillary files in addition to rendered Go source.
- LSP invalidation must track non-Ard files and directory membership.
- Reserving `ard/embed` permanently removes that path from the separately
  versioned standard-library namespace.
- Embedded contents increase executable size and remain extractable from built
  artifacts.

## Related

- [Go `embed` package](https://pkg.go.dev/embed)
- [Go `io/fs` package](https://pkg.go.dev/io/fs)
- `docs/language-philosophy.md`
- `docs/adrs/0002-use-air-as-backend-boundary.md`
- `docs/adrs/0013-use-file-based-modules-and-absolute-imports.md`
- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0049-overlay-ard-intrinsics-on-an-explicit-stdlib-package.md`
- `docs/adrs/0052-adopt-structured-labeled-diagnostics.md`
- `docs/adrs/0063-preserve-imported-go-error-identity.md`
- `docs/adrs/0069-expose-manifest-build-values-through-ard-build.md`
