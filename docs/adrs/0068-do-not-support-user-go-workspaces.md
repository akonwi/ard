# 0068: Do Not Support User Go Workspaces

## Status

Accepted

## Context

ADR 0035 originally included Go workspaces in the goal of resolving direct Go
interop the same way `go build` would. Ard's actual project and generation model
is module-scoped: a project has its own `go.mod`, generated output copies or
synthesizes one module file, and locked/path Ard dependencies are represented as
module replacements.

A user-provided `go.work` expands resolution beyond that project boundary and
introduces another source of module selection and replacement policy. It also
conflicts with compiler-owned module operations such as `-modfile`. Accommodating
workspace mode would therefore require a separate resolution, generation,
invalidation, and testing contract rather than being a transparent extension of
module support.

## Decision

Ard does not support user-provided Go workspaces. This includes workspaces
selected implicitly from a parent directory and explicitly through `GOWORK`.
The workspace portion of ADR 0035's original goal is superseded by this
decision.

The compiler is not required to detect, disable, preserve, emulate, or provide a
workspace-specific fallback for an ambient user workspace. Behavior when Ard is
run in user workspace mode is unspecified and may include diagnostics from the
Go toolchain. Users of direct Go interop must describe the required Go graph in
the Ard project's standalone `go.mod`, `go.sum`, and `replace` directives. When
their environment would otherwise select a workspace, they can invoke Ard with
`GOWORK=off`.

Ard does not create, select, or configure `go.work` files. Locked/path
dependency FFI is resolved through module files only. When the project has a
`go.mod`, the resolver derives a private alternate modfile without mutating the
source module. When the project has no `go.mod`, the resolver creates an
isolated temporary standalone module outside the source tree. Its requirements
and replacements mirror the generated backend module so checking and builds
select the same dependency roots.

Adding support for user Go workspaces requires a separate architecture decision
that defines checker/backend parity, generated-output behavior, invalidation,
diagnostics, and project-boundary semantics.

## Consequences

- Ard's supported Go interop boundary remains one standalone Go module per Ard
  project.
- Parent-directory `go.work` files and user `GOWORK` settings are not Ard project
  inputs and receive no compatibility guarantee.
- Resolver paths use only standalone module mechanisms, including private
  `-modfile` overlays and isolated temporary modules; Ard never synthesizes a
  Go workspace.
- The compiler does not add workspace-specific branches or tests merely to make
  ambient user workspaces succeed.
- Users who also maintain a Go workspace must run Ard outside workspace mode,
  typically with `GOWORK=off`.

## Related

- `docs/adrs/0035-use-go-packages-for-ffi-resolution.md`
- `docs/adrs/0044-use-a-shared-go-type-universe.md`
- `compiler/checker/go_packages_resolver.go`
