# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build/Test Commands
- Build: `go build`
- Run Ard program: `go run main.go run samples/[file].ard`
- Run all tests: `go test ./...`
- Run package tests: `go test ./ast` or `go test ./checker_v2` or `go test ./vm`
- Run single test: `go test -run TestName ./[package]`
- Verbose testing: `go test -v ./...`

## Code Style Guidelines
- **Naming**: PascalCase for exported, camelCase for unexported items
- **Imports**: Group standard library, third-party, then local imports
- **Error Handling**: Use diagnostics for compiler errors with source locations
- **Types**: Follow the type system in README.md for the Ard language spec
- **Testing**: Table-driven tests with descriptive names
- **Formatting**: Standard Go formatting (`gofmt`)
- **Project Structure**: Compiler follows ast → checker_v2 → vm pipeline
- **Development Tracking**: Use TODO.md for feature development progress
- **Sample Programs**: Reference samples directory for example Ard programs