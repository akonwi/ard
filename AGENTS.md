# AGENTS.md

Guidelines for agentic coding assistants working with the Ard language compiler.

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
- **Testing**: Table-driven tests with descriptive names using `[]test` struct pattern
- **Formatting**: Standard Go formatting (`gofmt`)
- **Types**: Follow the Ard language type system specification in README.md
- **Project Structure**: Compiler follows ast → checker → vm pipeline architecture
- **Dependencies**: Uses `github.com/google/go-cmp` for test comparisons with `cmpopts.IgnoreUnexported`
- **Sample Programs**: Reference samples/ directory for example Ard language programs
- **Development Tracking**: Use TODO.md for feature development progress
- **Commit Messages**: Do not add marketing content or Claude/Anthropic attribution
- do not add marketing content to commit messages. do not mention claude or anthropic
- never add "Co-Authored-By: Claude <noreply@anthropic.com>" to commits