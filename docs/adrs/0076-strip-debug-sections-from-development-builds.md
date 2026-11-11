# 0076: Strip Debug Sections From Development Builds

## Status

Accepted. Partially supersedes ADR 0069: `--release` now also selects the
linker's debug-section policy.

## Context

The Go target finishes every `ard build`, `ard run`, and `ard test` by invoking
`go build` on the generated workspace. That invocation has two phases: the Go
compiler, which runs per package and is cached by content, and the Go linker,
which runs once per binary and is never cached. On a large program the linker
dominates incremental builds, and most of its time goes to merging and
compressing DWARF debug records from every package in the dependency graph.

Measured on the Kit CLI (116 Ard modules, 145 Go modules) after the
compile-time work in #555–#560 reduced compiler-owned stages to under two
seconds: the link step took 4.7s with debug sections and 1.7s without, and the
binary shrank from 86MB to 61MB.

Ard previously took no position on linker flags. ADR 0069 states that
`--release` "does not alter optimization, backend settings, or defaults", and
the build-values guide (#505) directs users to strip symbols themselves through
`GOFLAGS`. That leaves the slow default in place for the workflow where speed
matters most, and it asks every Ard user to learn Go linker flags to fix it.

Two properties of the Go toolchain shape how Ard can set a default:

- A Go binary carries three kinds of self-description. `pclntab` maps
  addresses to functions and file:line positions; the runtime reads it for
  panic traces, `runtime.Caller`, and profiling, and the linker never omits it.
  The symbol table (`-s` removes it) serves external tools such as `nm` and
  crash symbolisers. DWARF (`-w` removes it; `-s` also implies it on current
  toolchains) serves source-level debuggers such as delve.
- An explicit `-ldflags` on the `go build` command line replaces any `-ldflags`
  supplied through `GOFLAGS`, and repeated `-ldflags` do not merge; the last
  one wins. A compiler that passes its own `-ldflags` therefore silently
  discards the user's unless it merges them itself.

For a compiled Ard program the generated Go is an intermediate artifact. Nobody
attaches a debugger to it, so DWARF describing it has no consumer in normal
use. Panic traces and profiles, which Ard users do rely on, come from `pclntab`
and are unaffected by either flag.

## Decision

The Go target owns a default linker policy, selected by the build mode:

- `ard build` without `--release`, `ard run`, and `ard test` link with
  `-ldflags=-s -w`, omitting the symbol table and DWARF.
- `ard build --release` passes no linker flags of its own, keeping both
  sections so shipped binaries remain inspectable by external tools.

`--release` thus means "a build someone else may need to inspect later", in
addition to the build-value requirement from ADR 0069. It still does not change
optimization, target, or build-value defaults.

When Ard supplies `-ldflags`, it preserves the user's. The backend reads the
effective `GOFLAGS` through `go env GOFLAGS`, which covers both the environment
and values stored with `go env -w`, splits it with the go tool's quoting rules,
takes the last `-ldflags` entry if any, and emits a single
`-ldflags=<user flags> -s -w`. User-supplied flags come first so they are
applied in the order the user wrote them. In release mode Ard sets no
`-ldflags`, and `GOFLAGS` reaches the go tool untouched.

Other `GOFLAGS` entries, and Go environment variables such as `GOOS`,
`GOARCH`, and `CGO_ENABLED`, continue to pass through unchanged. Ard does not
add `-trimpath` or any other packaging flag by default; this decision covers
debug sections only.

## Consequences

- Development builds link roughly twice as fast on large programs and produce
  binaries about 30% smaller. Panic traces, `runtime.Caller`, and pprof output
  are unchanged.
- Source-level debugging of the generated Go with delve or lldb requires
  `--release`. This is a compiler-developer workflow, not a user one.
- Release scripts that already set `-ldflags` in `GOFLAGS` keep working in both
  modes. Scripts that strip through `GOFLAGS` are now redundant for development
  builds but harmless.
- `ard run` and `ard test` always strip; there is no release mode for them.
- The build-values guide's "Go toolchain flags" section must be revised: Ard
  now sets a default for debug sections, and `GOFLAGS` extends rather than
  solely provides it.
- Any future default linker or packaging flag must go through the same
  `GOFLAGS` merge so user flags are never discarded. The argument builder is a
  pure function with table tests for this reason.

## Related

- `docs/adrs/0031-go-backend-lowering-contract.md`
- `docs/adrs/0035-use-go-packages-for-ffi-resolution.md` — build tags reach
  the same `go build` invocation
- `docs/adrs/0069-expose-manifest-build-values-through-ard-build.md` —
  partially superseded
- `website/src/content/docs/guide/build-values.md`
- Pull request #563; prior `GOFLAGS` passthrough documented in #505
