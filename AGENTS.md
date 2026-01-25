# AGENTS.md

This file provides guidance to a coding agent.

## Project Summary
This is the source code for the parser, compiler, interpreter, and documentation website of a programming language called Ard.

### Repo structure
This is a monorepo with the following top-level directories:
- /compiler: The Ard language compiler, parser, type checker, and runtime VM
  - /compiler/docs: documentation about language syntax, feature design, and implementation decisions
  - /compiler/samples: runnable Ard program samples
  - /compiler/parse: parser implementation
  - /compiler/checker: type checker and semantic analysis
  - /compiler/vm: virtual machine runtime and FFI system
  - /compiler/std_lib: standard library definitions
  - /compiler/main.go: compiler CLI entry point
  - /compiler/go.mod & go.sum: Go module dependencies for the compiler
- /website: the documentation website built with Astro and Starlight as a static site
- TODO.md: The running backlog of bugs, refactorings, and new feature ideas

## Commands
The project uses the `jsonv2` experiment for the [new json tools](https://antonz.org/go-json-v2/) and has build tag directives for it.

All commands should be run from the `/compiler` directory:

- Build: `cd compiler && go build`
  > Important: do not stage and commit the built binary
- Run Ard program: `cd compiler && go run main.go run samples/[file].ard`
- Run all tests: `cd compiler && go test ./...`
- Run package tests: `cd compiler && go test ./ast` or `go test ./checker` or `go test ./vm`
- Run single test: `cd compiler && go test -run TestName ./[package]`
- Verbose testing: `cd compiler && go test -v ./...`
- Generate FFI registry: `cd compiler && go generate ./vm` (run when adding new FFI functions)

## Instructions
- When searching the codebase, use the codanna tool first and only grep or use search CLI commands if codanna doesn't yield results
- use the gopls tool (Go language server) to get diagnostics and compilation info

## Code Style Guidelines
- **Naming**: PascalCase for exported, camelCase for unexported items
- **Imports**: Group standard library, third-party, then local imports
- **Error Handling**: Use diagnostics for compiler errors with source locations
- **Types**: Follow the type system in README.md for the Ard language spec
- **Testing**: Table-driven tests with descriptive names
  - use TDD
  - when adding new features or fixing bugs, recreate them with a test where possible instead of creating sample programs
  - prefer vm tests over checker tests. use vm tests to validate that written code functions as expected. use checker tests to document expected compiler errors
- **Formatting**: Standard Go formatting (`gofmt`)
- **Project Structure**: Compiler follows ast → checker → vm pipeline
- **Development Tracking**: Use TODO.md for feature development progress
- **Sample Programs**: Reference samples directory for example Ard programs
- **FFI System**: Standard library modules use Foreign Function Interface (FFI)
  - FFI functions in vm/ffi_*.go files with signature: `func(vm *VM, args []*object) (*object, any)`
  - Automatic code generation discovers and registers FFI functions
  - VM automatically handles Result and Maybe type wrapping
  - Standard library definitions in std_lib/*.ard using `extern fn` declarations
