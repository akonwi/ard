# FFI (Foreign Function Interface) Design

## Context & Goals

**Primary Goal**: Enable the standard library to be written in Ard rather than Go by providing FFI to call compiled Go functions from interpreted Ard code.

**Inspiration**: Gleam's FFI system - clean, type-safe interface to external functions.

**Key Requirements**:
- Type safety between Ard and Go
- Clean syntax for declaring external functions
- Allow standard library to be written in Ard instead of Go
- Keep Ard as an interpreted language (no user compilation required)
- Target-agnostic design (works for future JavaScript target)

**Scope**: FFI is designed **for standard library development only**. User extensions remain through the existing module system.

## Research: Gleam's FFI System

Gleam uses `@external` attributes for FFI:

```gleam
@external(erlang, "erlang", "halt")
@external(javascript, "./my_module.mjs", "halt")
pub fn halt(code: Int) -> Nil
```

Key insights:
- Multiple target support in single declaration
- Clean separation of declaration and implementation
- Type-safe boundaries between languages

## ✅ Decided Design Elements

### 1. Architecture: FFI for Standard Library Only ✅

**Decision**: FFI enables standard library to be written in Ard, while keeping Ard as an interpreted language.

**Current module system stays** for built-in/core functionality:
- Core VM functionality remains in Go (`vm/*_module.go` files)
- FFI functions are **compiled into the VM binary** (part of Ard distribution)

**FFI enables standard library migration to Ard**:
```
ard-vm-binary/
├── vm/               # VM implementation  
├── ffi/              # FFI implementations (compiled into VM)
│   ├── runtime.go    # go_print, go_read_line
│   ├── fs.go         # go_read_file, go_write_file  
│   └── net.go        # go_http_send
└── std_lib/          # Standard library (interpreted Ard!)
    ├── io.ard        # external fn print(...) = "runtime.go_print"
    ├── fs.ard        # external fn read_file(...) = "fs.go_read_file"  
    └── http.ard      # external fn send(...) = "net.go_http_send"
```

**Execution Flow**:
1. User runs: `ard run main.ard`  
2. User imports: `use ard/io`
3. VM loads: `std_lib/io.ard` (interpreted)
4. User calls: `io::print("hello")`
5. VM executes: External function `"runtime.go_print"` (compiled into VM)

**Benefits**:
- **Standard library in Ard** - main goal achieved
- **Keep Ard interpreted** - no user compilation complexity
- **Performance** - FFI functions compiled into VM
- **User simplicity** - just `ard run`, no build steps
- **Future-compatible** - works for JavaScript target too

**User Extensions**: Continue using existing module system (no FFI needed)

### 2. Syntax: External Keyword ✅

**Decision**: Use `external fn` with string binding

```ard
external fn print(value: $T) Void = "runtime.go_print"
external fn calculate(data: [Float]) Float = "math.complex_calculation"  
external fn read_file(path: Str) Str!Str = "fs.read_file_sync"
```

**Resolution strategy**:
- **Format**: `"module.function_name"`
- **Go target**: `"runtime.go_print"` → `./ffi/runtime.go::go_print()`
- **JS target**: `"runtime.go_print"` → `./ffi/runtime.js::go_print()`
- **Build system** resolves file extension based on compile target

**Benefits**:
- Clean and concise syntax
- Target-agnostic (same syntax for Go/JS)
- Clear mapping between Ard function and external implementation
- Type-safe (full Ard function signature)

### 3. FFI Organization: VM Binary Structure ✅

**Decision**: FFI functions are organized within the VM binary structure

**Standard library FFI pattern** (compiled into VM):
```
ard-vm-binary/ffi/
  runtime.go    # go_print, go_read_line, go_panic - core runtime
  fs.go         # go_read_file, go_write_file, go_mkdir - file system  
  net.go        # go_http_send, go_tcp_connect - networking
  system.go     # go_get_env, go_current_time - system info
```

**Standard library Ard code** (interpreted):
```
ard-vm-binary/std_lib/
  io.ard        # Uses "runtime.*" external functions
  fs.ard        # Uses "fs.*" external functions  
  http.ard      # Uses "net.*" external functions
  env.ard       # Uses "system.*" external functions
```

**Resolution**: `"module.function"` → `ffi/module.go::function()` (within VM binary)

**Benefits**:
- Logical grouping of related FFI functions
- Clear separation between Go implementations and Ard interfaces
- No user compilation required
- Clean mapping from external declaration to implementation

### 4. Type Mapping: Balance of Safety and Flexibility ✅

**Decision**: Direct mapping for simple types, typed collections, Dynamic escape hatch for complex data

**Simple Types**:
- `Int` → `int` (matches current Ard implementation)
- `Float` → `float64`
- `Str` → `string`  
- `Bool` → `bool`

**Generics**:
- `$T` → `any` (modern Go syntax, equivalent to `interface{}`)

**Collections (typed where possible)**:
- `[T]` → `[]T` (e.g., `[Int]` → `[]int`, `[Str]` → `[]string`)
- `{K: V}` → `map[K]V` (e.g., `{Str: Int}` → `map[string]int`)

**Error Handling**:
- `Result<T, E>` → `(T, error)` multiple return values
- VM handles conversion between Result and Go's error pattern

**Complex Types**:
- **Custom structs** → `decode::Dynamic` (opaque type)
- Users handle marshalling/unmarshalling on Go side as needed

**Examples**:
```ard
// Simple types - direct mapping
external fn add(a: Int, b: Int) Int = "math.add"

// Collections - typed 
external fn sum_list(numbers: [Int]) Int = "math.sum"
external fn lookup(data: {Str: Int}, key: Str) Int = "maps.get"

// Results - multiple returns
external fn read_file(path: Str) Str!Str = "fs.read_file"

// Complex data - Dynamic escape hatch
external fn process_data(data: decode::Dynamic) decode::Dynamic = "handlers.process"
```

```go
// Corresponding Go implementations
func add(a, b int) int { return a + b }
func sum(numbers []int) int { /* implementation */ }
func read_file(path string) (string, error) { return os.ReadFile(path) }
func process(data interface{}) interface{} { /* handle complex types */ }
```

**Benefits**:
- Type safety for common cases
- Performance with direct type mapping
- Flexibility with Dynamic for complex cases
- Simplicity - avoids complex struct marshalling

## Implementation Plan

### Phase 1: Syntax and Parsing 🎯
**Goal**: Parse `external fn` declarations

**Tasks**:
1. **AST Extension**:
   - Add `ExternalFunction` AST node
   - Fields: `name`, `parameters`, `return_type`, `external_binding`
   
2. **Parser Updates** (`ast/parser.go`):
   - Recognize `external fn` syntax
   - Parse function signature and external binding string
   - Add to statement parsing

3. **Checker Integration** (`checker/`):
   - Add `ExternalFunction` to type checking
   - Validate function signatures
   - Store external bindings for later resolution

**Deliverable**: Parse and type-check external function declarations

### Phase 2: External Function Resolution 🔍
**Goal**: Resolve external bindings to actual Go functions

**Tasks**:
1. **FFI Registry** (`checker/ffi.go`):
   - Map external bindings to file paths
   - `"module.function"` → `./ffi/module.go::function`
   - Validate that external files/functions exist at compile time

2. **Build Integration**:
   - Ensure `./ffi/*.go` files are compiled with main program
   - Handle Go module dependencies

3. **Error Handling**:
   - Clear errors when external functions don't exist
   - Helpful messages for missing FFI files

**Deliverable**: Compile-time validation of external function bindings

### Phase 3: VM Integration and Type Marshalling 🚀
**Goal**: Execute external functions at runtime with proper type conversion

**Tasks**:
1. **Type Marshalling** (`vm/ffi_marshal.go`):
   - Convert Ard values to Go values (based on our type mapping)
   - Convert Go return values back to Ard values
   - Handle `Result<T, E>` ↔ `(T, error)` conversion

2. **VM Execution** (`vm/interpret.go`):
   - Detect external function calls
   - Marshal arguments, call Go function, marshal results
   - Handle panics and errors gracefully

3. **Function Registry** (`vm/ffi_registry.go`):
   - Runtime registry of external Go functions
   - Use Go's reflection to call functions dynamically
   - Cache function lookups for performance

**Deliverable**: Full runtime execution of external functions

### Phase 4: Standard Library Migration 📚
**Goal**: Prove FFI works by migrating some built-in modules

**Tasks**:
1. **Create FFI Runtime** (`ffi/runtime.go`):
   - Implement `go_print`, `go_read_line`, etc.
   - Move some functionality from `vm/io_module.go`

2. **Ard Standard Library** (`std_lib/`):
   - Create `io.ard` with external function declarations
   - Test that external functions work correctly
   - Ensure API compatibility with current modules

3. **Integration Testing**:
   - Extensive tests for type marshalling
   - Performance benchmarks vs current implementation
   - Error handling edge cases

**Deliverable**: Working FFI-based standard library modules **compiled into VM**

## Development Strategy

### Start Small
**Phase 1 MVP**: Simple standard library functions only
```ard
// std_lib/math.ard  
external fn add(a: Int, b: Int) Int = "math.add"
```

### Build Up Complexity
**Phase 2**: Add collections and Results to standard library
```ard
// std_lib/list_utils.ard
external fn sum_list(numbers: [Int]) Int = "math.sum"

// std_lib/fs.ard
external fn read_file(path: Str) Str!Str = "fs.read_file"
```

### Prove with Real Use Cases
**Phase 3**: Migrate actual standard library modules
```ard
// std_lib/io.ard
external fn print(value: $T) Void = "runtime.go_print"
```

## Integration Points

- **Parser** (`ast/parser.go`): New `external fn` syntax
- **Checker** (`checker/`): Type validation and external binding resolution  
- **VM** (`vm/`): Runtime execution and type marshalling
- **FFI Functions** (`ffi/`): Go implementations compiled into VM binary
- **Standard Library** (`std_lib/`): Ard code using external functions

## Examples & Use Cases

### Standard Library Migration
```ard
// std_lib/io.ard
external fn print(value: $T) Void = "runtime.go_print"  
external fn read_line() Str!Str = "runtime.go_read_line"

// std_lib/fs.ard  
external fn read_file(path: Str) Str!Str = "fs.read_file"
external fn write_file(path: Str, content: Str) Void!Str = "fs.write_file"
```

### User Code (unchanged - just imports standard library)
```ard
use ard/io
use ard/fs

fn main() {
    // Uses FFI-based standard library transparently
    let content = fs::read_file("data.txt")
    match content {
        ok(data) => io::print("File contents: " + data)
        error(err) => io::print("Error: " + err)
    }
}
```
