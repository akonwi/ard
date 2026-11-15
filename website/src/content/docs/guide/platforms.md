---
title: Platform-specific code
description: Select declarations by operating system and architecture.
---

Some Go APIs exist only on some platforms. `syscall.SIGWINCH`, for example, is
defined on unix systems but not on Windows. Every Ard declaration is
type-checked against the Go packages for the platform being built, so code
that names such a symbol needs to say which platforms it is for.

## `#platform`

Write `#platform(...)` on the line before a top-level declaration or a `use`
line. The declaration is compiled only for the listed platforms:

```ard
use go:os
use go:syscall

fn watched() [os::Signal] {
  [os::Interrupt]
}

#platform(unix)
fn watched() [os::Signal] {
  let signals = &mut List::new<os::Signal>()
  signals.push(os::Interrupt)
  signals.push(syscall::SIGWINCH)
  signals.*
}
```

On unix platforms `watched` returns both signals. Everywhere else, including
Windows, the declaration without `#platform` is used. Declarations for other
platforms are not type-checked or compiled for the current platform, so
`syscall::SIGWINCH` causes no error when building for Windows.

The attribute takes one or more platform names and matches when any of them
matches:

| Name | Matches |
| --- | --- |
| `linux`, `darwin`, `windows`, ... | That operating system (any Go `GOOS`) |
| `linux_amd64`, `darwin_arm64`, ... | That operating system and architecture |
| `unix` | Go's unix systems: aix, android, darwin, dragonfly, freebsd, hurd, illumos, ios, linux, netbsd, openbsd, solaris |

As with Go build constraints, `linux` also matches Android, `darwin` also
matches iOS, and `solaris` also matches illumos. Unknown names are errors.

`#platform` may be used on functions, `let` and `mut` bindings, type
declarations, structs, enums, traits, `impl` blocks, tests, and `use` lines.
A platform-specific `use` loads its package only on those platforms, which
matters for Go packages that do not build everywhere:

```ard
#platform(windows)
use go:golang.org/x/sys/windows
```

## Platform-specific versions of a declaration

Declarations with the same name form one group:

- The declaration without `#platform` is the fallback. It is used on every
  platform that none of its siblings list. There can be only one.
- The annotated declarations must not overlap: no platform may match two of
  them.
- Without a fallback, the name does not exist on unlisted platforms.

Every version must have the same signature, so code that calls it checks the
same way on every platform. This is checked on every build, whatever the
platform:

- functions must have identical headers: parameter names and types, and the
  return type;
- `let` and `mut` bindings need an explicit, identical type;
- `impl` blocks must declare the same method signatures;
- `type` declarations must have the same name and visibility, while the type
  they name may differ;
- structs, enums, and traits can be limited to some platforms, but cannot
  have per-platform versions.

## Choosing and checking platforms

By default Ard builds for the platform the Go toolchain targets, which follows
`GOOS` and `GOARCH`. Pass `--platform` to `ard check` or `ard build` to choose
another:

```sh
ard build main.ard --platform windows/amd64
```

A Windows build without `--out` is named with an `.exe` suffix.

Code for other platforms is not checked by an ordinary build. To check it,
run:

```sh
ard check main.ard --all-platforms
```

This checks one platform for each distinct combination of `#platform`
declarations in the program, such as `linux/amd64` and `windows/amd64` for the
example above. Each check resolves Go packages for its platform; the first
check for a platform compiles Go's export data for it, which the Go build cache
keeps.

The language server checks declarations for other platforms too. Diagnostics
inside them are labeled with the platform they were found on, and hover and
go-to-definition work inside them.
