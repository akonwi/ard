# Implementation Plan: User File Imports

This document tracks the implementation of importing from other files in the Ard language.

## Overview
Enable importing from user-defined `.ard` files using the existing `use` keyword syntax, following the module resolution strategy defined in [imports.md](./imports.md).

## Phase 1: Refactoring & Project Discovery ✅ TODO

### 1.1 Rename Package to Module
- [x] Rename `Package` interface to `Module` in `checker/checker.go`
- [x] Update `StdPackage` to `StdModule` 
- [x] Update all references throughout codebase (`Imports` field, function names, etc.)
- [x] Update tests to use new terminology

### 1.2 Add project root discovery
- [ ] Add function to find `ard.toml` file by walking up directory tree
- [ ] Parse `ard.toml` to extract project name (simple TOML parsing or regex)
- [ ] Fallback to directory name if no `ard.toml` found
- [ ] Cache project root and name for performance
- [ ] Add tests for project discovery

### 1.3 Add file path resolution
- [ ] Convert import path like `my_project/utils` to filesystem path `utils.ard`
- [ ] Convert nested paths like `my_project/math/operations` to `math/operations.ard`
- [ ] Validate that resolved files exist and are readable
- [ ] Add tests for path resolution

## Phase 2: Module Loading & Caching ✅ TODO

### 2.1 Create module loading system
- [ ] Add function to load and parse `.ard` files on demand
- [ ] Cache parsed AST to avoid re-parsing the same file
- [ ] Handle circular dependency detection
- [ ] Track import chain for better error messages
- [ ] Add tests for module loading

### 2.2 Extend checker to process imported modules
- [ ] Run type checking on imported modules first (dependency order)
- [ ] Extract public symbols (functions, types, structs, etc.) from imported modules
- [ ] Create `UserModule` objects containing only public symbols
- [ ] Add to `c.program.Imports` map like standard library modules
- [ ] Add tests for symbol extraction

## Phase 3: Symbol Resolution & Scoping ✅ TODO

### 3.1 Update symbol resolution
- [ ] Modify identifier lookup to check imported modules via `::` syntax
- [ ] Handle namespace conflicts and ambiguous imports
- [ ] Ensure private symbols are not accessible from other modules
- [ ] Add tests for symbol resolution

### 3.2 Add import validation
- [ ] Verify imported symbols actually exist and are public
- [ ] Better error messages for import failures
- [ ] Detect unused imports (warnings)
- [ ] Add tests for import validation

## Phase 4: VM Integration ✅ TODO

### 4.1 Module execution system
- [ ] Load and execute imported modules in dependency order
- [ ] Share global environment between modules for imported symbols
- [ ] Handle module initialization (top-level statements)
- [ ] Add tests for module execution

### 4.2 Runtime symbol resolution
- [ ] Update VM's identifier resolution to work with imported modules
- [ ] Ensure proper scoping between modules
- [ ] Add tests for runtime resolution

## Phase 5: Testing & Integration ✅ TODO

### 5.1 Create comprehensive test files
- [ ] Multi-file test scenarios
- [ ] Public/private visibility tests
- [ ] Circular dependency error tests
- [ ] `ard.toml` parsing tests
- [ ] Nested module structure tests

### 5.2 Integration testing
- [ ] End-to-end tests with sample projects
- [ ] Error handling validation
- [ ] Performance testing with large import graphs

## Implementation Notes

### Technical Decisions
- **Caching strategy**: Cache parsed AST and checked modules by file path
- **Error handling**: Collect all import errors before failing
- **Performance**: Lazy loading of modules (only when imported)
- **Threading**: Single-threaded for now (can parallelize later)

### Implementation Order
1. **Phase 1**: Refactoring and foundation
2. **Phase 2**: Core module loading in checker
3. **Phase 3**: Symbol resolution and validation
4. **Phase 4**: VM runtime support
5. **Phase 5**: Comprehensive testing

### Current Status
- ✅ AST and parser support for `pub` keyword and `use` statements
- ✅ Standard library import system (`ard/*` modules)
- ✅ Duplicate import detection
- 🚧 User file import support (this implementation)

## Related Files
- `docs/imports.md` - Import specification
- `TODO.md` - Project backlog
- `ast/parser.go` - Import parsing
- `checker/checker.go` - Import processing
- `vm/vm.go` - Runtime execution
