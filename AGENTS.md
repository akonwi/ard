# AGENTS.md

## Project Summary
This is the source code for the Ard programming language.

### Repo structure
This is a monorepo with the following top-level directories:
- /compiler: The Ard language compiler, parser, type checker, and native/JS targets
  - /compiler/docs: documentation about language syntax, feature design, and implementation decisions
  - /compiler/samples: runnable Ard program samples
  - /compiler/parse: parser implementation
  - /compiler/checker: type checker and semantic analysis
  - /compiler/go: Go target implementation
  - /compiler/javascript: JavaScript target implementation
  - /compiler/std_lib: standard library definitions
  - /compiler/main.go: compiler CLI entry point
  - /compiler/go.mod & go.sum: Go module dependencies for the compiler
- /tree-sitter-ard: tree-sitter grammar for Ard (used for the zed plugin and syntax highlighting)
- /website: the documentation website built with Astro and Starlight as a static site
- /zed-plugin: plugin for the Zed editor

## Philosophy

- Code quality is important. Do not take shortcuts or create hard to maintain problems.
- Be pragmatic and solution oriented
- Test Driven Development: use tests to guide implmeentation
  - when working on bugs, start with test reproduction. tests also add documentation of the case for future reference

## Commands
The project uses the `jsonv2` experiment for the [new json tools](https://antonz.org/go-json-v2/) and has build tag directives for it.

All commands should be run from the `/compiler` directory:

- Build: `cd compiler && go build`
  > Important: do not stage and commit the built binary
- Run Ard program: `cd compiler && go run main.go run samples/[file].ard`
- Format Ard files: `cd compiler && go run main.go format [path]`
- Run all tests: `cd compiler && go generate ./std_lib/ffi && go test ./...`
- Run package tests: `cd compiler && go test ./go` or `go test ./checker` or `go test ./formatter`
- Run single test: `cd compiler && go test -run TestName ./[package]`
- Verbose testing: `cd compiler && go test -v ./...`
- Generate stdlib FFI/Go-target metadata: `cd compiler && go generate ./std_lib/ffi` (run before tests and when changing stdlib extern declarations)

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
- **FFI System**: Standard library modules use Foreign Function Interface (FFI)
  - Go target stdlib FFI metadata is generated from std_lib extern declarations and std_lib/ffi Go implementations
  - Project Go FFI companions live in root `ffi.go` or `ffi/*.go` and use `package ffi`
  - Standard library definitions in std_lib/*.ard use `extern fn` declarations
