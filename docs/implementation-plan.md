# Implementation Plan: User File Imports

This document tracks the implementation of importing from other files in the Ard language.

## Overview
Enable importing from user-defined `.ard` files using the existing `use` keyword syntax, following the module resolution strategy defined in [imports.md](./imports.md).

## Phase 1: Refactoring & Project Discovery ✅ COMPLETE

### 1.1 Rename Package to Module
- [x] Rename `Package` interface to `Module` in `checker/checker.go`
- [x] Update `StdPackage` to `StdModule` 
- [x] Update all references throughout codebase (`Imports` field, function names, etc.)
- [x] Update tests to use new terminology

### 1.2 Add project root discovery
- [x] Add function to find `ard.toml` file by walking up directory tree
- [x] Parse `ard.toml` to extract project name (simple TOML parsing or regex)
- [x] Fallback to directory name if no `ard.toml` found
- [x] Cache project root and name for performance
- [x] Add tests for project discovery

### 1.3 Add file path resolution
- [x] Convert import path like `my_project/utils` to filesystem path `utils.ard`
- [x] Convert nested paths like `my_project/math/operations` to `math/operations.ard`
- [x] Validate that resolved files exist and are readable
- [x] Add tests for path resolution

## Phase 2: Module Loading & Caching ✅ COMPLETE

### 2.1 Create module loading system
- [x] Add function to load and parse `.ard` files on demand
- [x] Cache parsed AST to avoid re-parsing the same file
- [x] Handle circular dependency detection
- [x] Track import chain for better error messages
- [x] Add tests for module loading

## Phase 3: Symbol Extraction & Checker Integration ✅ MOSTLY COMPLETE

### 3.1 Extract public symbols from imported modules ✅ COMPLETE
- [x] Modify `Check()` function to return a `Module` representing the checked program
- [x] Create `UserModule` struct implementing `Module` interface with `get()` method for symbol access
- [x] Add `Public` fields to `FunctionDef` and `StructDef` types for visibility control
- [x] Extract only public symbols in `UserModule.get()` - return nil for private/missing symbols
- [x] Use global scope symbols instead of program statements for symbol extraction
- [x] Add tests for symbol extraction and caching

### 3.2 Update symbol resolution ✅ COMPLETE
- [x] Modify identifier lookup to check imported modules via `::` syntax
- [x] Implement user module loading and caching in `Check()` function
- [x] Ensure private symbols are not accessible from other modules
- [x] Add tests for symbol resolution, private access control, and caching
- [ ] Handle namespace conflicts and ambiguous imports (optional enhancement)

### 3.3 Add import validation
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
- [x] Circular dependency error tests
- [x] `ard.toml` parsing tests
- [x] Nested module structure tests

### 5.2 Integration testing
- [ ] End-to-end tests with sample projects
- [ ] Error handling validation
- [ ] Performance testing with large import graphs

## Implementation Notes

### Technical Decisions
- **Caching strategy**: Cache parsed AST and checked modules by file path (avoid recursive type checking)
- **Module interface**: `Check()` returns `Module` objects cached in `moduleCache` 
- **Symbol visibility**: `Module.get()` method handles public/private access automatically
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
- ✅ Project discovery and file path resolution
- ✅ Module loading, caching, and circular dependency detection
- ✅ Symbol extraction from checked modules (Phase 3.1 complete)
- ✅ Symbol resolution with `::` syntax for user modules (Phase 3.2 complete)
- ✅ Private symbol access control and comprehensive testing
- 🚧 Import validation and VM integration (next steps)

## Related Files
- `docs/imports.md` - Import specification
- `TODO.md` - Project backlog
- `ast/parser.go` - Import parsing
- `checker/checker.go` - Import processing
- `vm/vm.go` - Runtime execution
