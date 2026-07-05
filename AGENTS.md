# AGENTS.md

## Project Summary
This is the source code for the Ard programming language.

### Repo structure
This is a monorepo with the following top-level directories:
- /compiler: The Ard language compiler, parser, type checker, and Go target
  - /compiler/parse: parser implementation
  - /compiler/checker: type checker and semantic analysis (including direct Go FFI resolution)
  - /compiler/air: the target-neutral intermediate representation (frontend/backend boundary)
  - /compiler/frontend: module loading pipeline shared by CLI commands
  - /compiler/go: Go target implementation (lowering and code generation)
  - /compiler/lsp: the language server (snapshot-based analysis engine, see ADR 0043)
  - /compiler/formatter: canonical Ard formatting
  - /compiler/runtime: the minimal shared runtime (`Maybe`, `Result`) generated programs depend on
  - /compiler/std_lib: standard library definitions (intentionally minimal during the FFI reset, see ADR 0034)
  - /compiler/samples: runnable Ard program samples
  - /compiler/main.go: compiler CLI entry point
  - /compiler/go.mod & go.sum: Go module dependencies for the compiler
- /docs: project-wide documentation; /docs/adrs holds the Architecture Decision Records
- /examples: full example projects (vaxis-demo exercises the direct Go FFI against a real dependency)
- /scripts: repo tooling (commit message validation, the manual LSP harness)
- /tree-sitter-ard: tree-sitter grammar for Ard (used for the zed plugin and syntax highlighting)
- /website: the documentation website built with Astro and Starlight as a static site
- /zed-plugin: plugin for the Zed editor

## Philosophy

- Code quality is important. Do not take shortcuts or create hard to maintain problems.
- Be pragmatic and solution oriented
- Test Driven Development: use tests to guide implmeentation
  - when working on bugs, start with test reproduction. tests also add documentation of the case for future reference

## Commands

All commands should be run from the `/compiler` directory:

- Build: `cd compiler && go build`
  > Important: do not stage and commit the built binary
- Run Ard program: `cd compiler && go run main.go run samples/[file].ard`
- Format Ard files: `cd compiler && go run main.go format [path]`
- Run all tests: `cd compiler && go test ./...`
- Run package tests: `cd compiler && go test ./go` or `go test ./checker` or `go test ./formatter`
- Run single test: `cd compiler && go test -run TestName ./[package]`
- Verbose testing: `cd compiler && go test -v ./...`
- Validate the LSP end-to-end: `cd compiler && go build -o /tmp/ard-lsp-test . && cd .. && python3 scripts/lsp-harness.py` (manual stdio harness against `examples/vaxis-demo`; run after changes to the LSP transport, analysis engine, or checker semantics — see ADR 0043)

## Instructions
- use the gopls tool (Go language server) to get diagnostics and compilation info

## Code Style Guidelines
- **Naming**: PascalCase for exported, camelCase for unexported items
- **Imports**: Group standard library, third-party, then local imports
- **Error Handling**: Use diagnostics for compiler errors with source locations
- **Types**: Follow the type system in README.md for the Ard language spec
- **Testing**: Table-driven tests with descriptive names
  - use TDD
  - when adding new features or fixing bugs, recreate them with a test where possible instead of creating sample programs
  - prefer Go-target tests for executable behavior. use checker tests to document expected compiler errors
- **Formatting**:
  - Go: standard Go formatting (`gofmt`)
  - Ard: use `cd compiler && go run main.go format [path]`
  - When changing `.ard` files, include a formatter verification pass as part of validation. At minimum, run `ard format` on the touched file(s) or containing directory and make sure formatting does not introduce unintended semantic changes.
  - When changing formatter behavior or stdlib `.ard` files, also run the relevant formatter tests (at least `cd compiler && go test ./formatter`) and, for stdlib changes, prefer `cd compiler && go run main.go format std_lib` as a regression check.
- **Project Structure**: Compiler follows parse → checker → AIR → target lowering
- **Development Tracking**: Use GitHub issues for top-level work items
- **Sample Programs**: Reference samples directory for example Ard programs
- **FFI System**: Go interop is direct — there is no extern binding layer (see ADRs 0031/0034/0035)
  - Ard code imports Go packages with `use go:` and calls exported symbols with namespace syntax (`fmt::Println`)
  - The checker validates Go boundaries through `go/packages`; the Go backend owns the lowering
  - Project Go shim code lives in `ffi/*.go` as `package ffi` and is imported like any other Go package (`use go:<project>/ffi`)
  - `extern` is no longer valid syntax; the old std_lib/ffi generation step no longer exists
