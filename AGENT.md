# AGENT.md

This file provides guidance to a coding agent.

## Project Summary
This is the source code for the parser, compiler, and interpreter of a programming language called Ard
The language spec is in the README.md
The backlog of work is a simple todo list in TODO.md
Runnable sample code can be found in ./samples/*.ard

## Build/Test Commands
- Build: `go build`
- Run Ard program: `go run main.go run samples/[file].ard`
- Run all tests: `go test ./...`
- Run package tests: `go test ./ast` or `go test ./checker` or `go test ./vm`
- Run single test: `go test -run TestName ./[package]`
- Verbose testing: `go test -v ./...`

## Code Style Guidelines
- **Naming**: PascalCase for exported, camelCase for unexported items
- **Imports**: Group standard library, third-party, then local imports
- **Error Handling**: Use diagnostics for compiler errors with source locations
- **Types**: Follow the type system in README.md for the Ard language spec
- **Testing**: Table-driven tests with descriptive names
- **Formatting**: Standard Go formatting (`gofmt`)
- **Project Structure**: Compiler follows ast → checker → vm pipeline
- **Development Tracking**: Use TODO.md for feature development progress
- **Sample Programs**: Reference samples directory for example Ard programs
