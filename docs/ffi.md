# Foreign Function Interface (FFI)

## Overview

The Ard FFI system enables the standard library to be written in Ard rather than Go by providing a clean, type-safe interface to call compiled Go functions from interpreted Ard code. This design keeps Ard as an interpreted language while allowing performance-critical operations to be implemented in Go.

## Architecture

### Core Design Principles

- **Standard library focus**: FFI is designed specifically for standard library development, not general user extensions
- **Type safety**: Full type checking and validation between Ard and Go boundaries
- **Zero reflection**: Direct function calls using uniform signatures for maximum performance
- **Automatic discovery**: Code generation eliminates manual registration overhead
- **Clean separation**: Go implementations in `./ffi/`, Ard interfaces in `./std_lib/`

### Directory Structure

```
ard/
├── ffi/                    # FFI implementations (compiled into VM)
│   ├── runtime.go         # Print, ReadLine, PanicWithMessage, EnvGet
│   └── json.go            # JsonEncode
├── std_lib/               # Standard library (interpreted Ard)
│   ├── io.ard             # Uses "Print", "ReadLine"
│   ├── env.ard            # Uses "EnvGet"
│   └── json.ard           # Uses "JsonEncode"
└── vm/
    ├── ffi_generate.go    # Auto-discovery and registry generation
    ├── ffi_registry.go    # Runtime FFI registry
    └── registry.gen.go    # Generated function bindings
```

## FFI Function Implementation

### Function Signature

All FFI functions follow a uniform signature for consistency and performance:

```go
func FunctionName(vm runtime.VM, args []*runtime.Object) *runtime.Object
```

**Parameters:**
- `vm runtime.VM`: Interface providing access to VM methods for object evaluation
- `args []*runtime.Object`: Array of Ard values converted to VM's internal object representation

**Return:**
- `*runtime.Object`: Single return value (use Result types for error handling)

### Example Implementation

```go
// ffi/runtime.go
func Print(vm runtime.VM, args []*runtime.Object) *runtime.Object {
    if len(args) != 1 {
        panic(fmt.Errorf("print expects 1 argument, got %d", len(args)))
    }

    arg := args[0]
    switch raw := arg.Raw().(type) {
    case string, bool, int, float64:
        fmt.Printf("%v\n", raw)
        return runtime.Void()
    default:
        // Handle complex types by calling Ard's to_str method
        if _, ok := arg.Type().(*checker.StructDef); ok {
            call := &checker.FunctionCall{Name: "to_str", Args: []checker.Expression{}}
            str := vm.EvalStructMethod(arg, call).Raw().(string)
            fmt.Println(str)
            return runtime.Void()
        }
        panic(fmt.Errorf("unprintable type: %T", raw))
    }
}
```

## Standard Library Integration

### Extern Function Declaration

Standard library modules use `extern fn` declarations to bind Ard functions to Go implementations:

```ard
// std_lib/io.ard
extern fn print(value: $T) Void = "Print"
extern fn read_line() Str!Str = "ReadLine"
```

**Syntax:**
- `extern fn` keyword introduces external function
- Full Ard type signature for type safety
- String binding directly references Go function name (no module prefix needed)

### Usage in User Code

```ard
use ard/io

fn main() {
    io::print("Hello from FFI!")

    match io::read_line() {
        Ok(input) -> io::print("You entered: " + input)
        Err(error) -> io::print("Error: " + error)
    }
}
```

## Automatic Code Generation

### Discovery Process

The FFI system uses Go's `go generate` to automatically discover and register FFI functions:

1. **Parse**: Scan all `./ffi/*.go` files using Go's AST parser
2. **Validate**: Verify functions match required signature `func(vm runtime.VM, args []*runtime.Object) *runtime.Object`
3. **Generate**: Create `vm/registry.gen.go` with registration code
4. **Register**: VM automatically loads generated bindings at startup

### Generated Registry

```go
// vm/registry.gen.go (generated)
func (r *RuntimeFFIRegistry) RegisterGeneratedFFIFunctions() error {
    if err := r.Register("Print", ffi.Print); err != nil {
        return fmt.Errorf("failed to register Print: %w", err)
    }
    if err := r.Register("JsonEncode", ffi.JsonEncode); err != nil {
        return fmt.Errorf("failed to register JsonEncode: %w", err)
    }
    // ... more registrations
    return nil
}
```

### Build Integration

```bash
# Regenerate FFI registry after adding new functions
go generate ./vm

# Build normally - generated code is included automatically
go build
```

## Type Marshalling

### VM Interface

The `runtime.VM` interface provides access to VM operations needed by FFI functions:

```go
// vm/runtime/vm.go
type VM interface {
    EvalStructMethod(obj *Object, call *checker.FunctionCall) *Object
    EvalEnumMethod(obj *Object, call *checker.FunctionCall, enum *checker.Enum) *Object
}
```

This interface enables FFI functions to:
- Call Ard methods on objects (like `to_str()` for custom string conversion)
- Evaluate enum methods for proper formatting
- Maintain type safety across the FFI boundary

### Runtime Objects

All values are marshalled through the VM's `*runtime.Object` type:

```go
// Creating Ard values from Go
runtime.MakeStr("hello")           // String
runtime.MakeInt(42)                // Integer
runtime.MakeBool(true)             // Boolean
runtime.Void()                     // Void return

// Result types for error handling
runtime.MakeOk(value)              // Success case
runtime.MakeErr(error)             // Error case
runtime.MakeMaybe(value, type)     // Maybe types
```

## Error Handling

### Panic Recovery

The FFI registry automatically recovers from panics and converts them appropriately:

- **Result return types**: Panics become `Err(message)`
- **Other return types**: Panics propagate with enhanced context

### Example Error Patterns

```go
func SafeOperation(vm runtime.VM, args []*runtime.Object) *runtime.Object {
    // Input validation - panic will be caught by registry
    if len(args) != 1 {
        panic(fmt.Errorf("expected 1 argument, got %d", len(args)))
    }

    // Operation that might fail
    if result, err := riskyOperation(args[0]); err != nil {
        return runtime.MakeErr(runtime.MakeStr(err.Error()))
    } else {
        return runtime.MakeOk(result)
    }
}
```

## Development Workflow

### Adding New FFI Functions

1. **Implement Go function** in appropriate `./ffi/*.go` file:
   ```go
   func NewFunction(vm runtime.VM, args []*runtime.Object) *runtime.Object {
       // Implementation
   }
   ```

2. **Regenerate registry**:
   ```bash
   go generate ./vm
   ```

3. **Add Ard binding** in `./std_lib/*.ard`:
   ```ard
   extern fn new_function(param: Type) ReturnType = "NewFunction"
   ```

4. **Build and test**:
   ```bash
   go build && go run main.go run test_program.ard
   ```

### Function Organization

- **`ffi/runtime.go`**: Core runtime functions (print, input, panic, environment)
- **`ffi/json.go`**: JSON encoding/decoding operations
- **Future modules**: File system, networking, cryptography, etc.

## Current Implementation Status

### ✅ Completed Features

- **Complete FFI syntax**: `extern fn` declarations with direct Go function bindings
- **Type-safe integration**: Full type checking between Ard and Go
- **Automatic discovery**: Code generation eliminates manual registration
- **Runtime execution**: Direct function calls with uniform signatures
- **VM interface**: Clean access to VM operations for complex type handling
- **Error handling**: Panic recovery and Result type integration
- **Standard library migration**: Proof-of-concept with `io`, `env`, and `json` modules

### 🎯 Architecture Achievements

- **Zero reflection**: All function calls are direct for maximum performance
- **Clean separation**: Go implementations completely separate from Ard interfaces
- **Scalable design**: Adding new functions requires no manual registration
- **Type safety**: Full marshalling between Ard and Go type systems
- **Backward compatibility**: All existing sample programs work unchanged

### 📊 Testing Coverage

- **Unit tests**: Comprehensive FFI registry and marshalling tests
- **Integration tests**: All VM tests pass with FFI-based standard library
- **Sample programs**: All existing samples work with new FFI system
- **Performance**: Equivalent or better than previous hardcoded implementations

## Future Expansions

### Standard Library Migration Candidates

- **File system**: `ard/fs` for file operations
- **HTTP client**: `ard/http` for web requests
- **Cryptography**: `ard/crypto` for hashing and encryption
- **Date/time**: `ard/time` for temporal operations
- **Regular expressions**: `ard/regex` for pattern matching

### Enhancement Opportunities

- **Cross-compilation**: JavaScript target support with same syntax
- **Documentation generation**: Auto-generate docs from Go function comments
- **IDE integration**: Navigate from Ard declarations to Go implementations
- **Validation**: Check Ard bindings match Go function signatures

## Conclusion

The FFI system successfully achieves its primary goal: enabling the standard library to be written in Ard rather than Go while maintaining the performance and type safety of a compiled implementation. The automatic discovery and registration system provides an excellent developer experience that scales naturally as new functionality is added.

This architecture positions Ard well for future growth, with a clean separation between the interpreted language layer and high-performance system operations implemented in Go.
