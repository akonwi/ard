//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// FFIFunction represents a discovered FFI function
type FFIFunction struct {
	Name   string
	Module string
	File   string
}

func main() {
	// Find all Go files in ffi directory
	ffiDir := "../ffi"
	functions, err := discoverFFIFunctions(ffiDir)
	if err != nil {
		log.Fatalf("Failed to discover FFI functions: %v", err)
	}

	// Generate the registry file
	if err := generateRegistry(functions); err != nil {
		log.Fatalf("Failed to generate registry: %v", err)
	}

	fmt.Printf("Generated registry with %d FFI functions\n", len(functions))
}

// discoverFFIFunctions scans Go files for FFI function signatures
func discoverFFIFunctions(dir string) ([]FFIFunction, error) {
	var functions []FFIFunction

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-Go files
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip generate.go and generated files
		if strings.HasSuffix(path, "ffi_generate.go") || strings.HasSuffix(path, ".gen.go") {
			return nil
		}

		fileFunctions, err := parseFFIFunctions(path)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}

		functions = append(functions, fileFunctions...)
		return nil
	})

	return functions, err
}

// parseFFIFunctions parses a Go file and extracts FFI functions
func parseFFIFunctions(filename string) ([]FFIFunction, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var functions []FFIFunction

	// Extract module name from filename (e.g., runtime.go -> runtime, json.go -> json)
	base := filepath.Base(filename)
	if !strings.HasSuffix(base, ".go") {
		return functions, nil // Skip non-Go files
	}
	module := strings.TrimSuffix(base, ".go")

	// Walk the AST to find FFI functions
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			if isFFIFunction(fn) {
				functions = append(functions, FFIFunction{
					Name:   fn.Name.Name,
					Module: module,
					File:   filename,
				})
			}
		}
		return true
	})

	return functions, nil
}

// isFFIFunction checks if a function matches the FFI signature
func isFFIFunction(fn *ast.FuncDecl) bool {
	// Skip functions that start with underscore (internal/private functions)
	if strings.HasPrefix(fn.Name.Name, "_") {
		return false
	}

	// Check function signature: func(vm *VM, args []*runtime.Object, ret checker.Type) *runtime.Object
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 3 {
		return false
	}

	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		return false
	}

	// Validate first parameter: vm VM (interface)
	firstParam := fn.Type.Params.List[0]
	if len(firstParam.Names) != 1 || firstParam.Names[0].Name != "vm" {
		return false
	}
	if !isVMInterface(firstParam.Type) {
		return false
	}

	// Validate second parameter: args []*runtime.Object
	secondParam := fn.Type.Params.List[1]
	if !isSliceOfPointerToRuntimeObject(secondParam.Type) {
		return false
	}

	// Validate third parameter: ret checker.Type
	thirdParam := fn.Type.Params.List[2]
	if sel, isSelector := thirdParam.Type.(*ast.SelectorExpr); isSelector {
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == "checker" && sel.Sel.Name == "Type"
	}

	// Validate return type: *runtime.Object
	firstReturn := fn.Type.Results.List[0]
	if !isPointerToRuntimeObject(firstReturn.Type) {
		return false
	}

	return true
}

// Helper functions for AST type checking
func isPointerToType(expr ast.Expr, typeName string) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == typeName
}

func isVMInterface(expr ast.Expr) bool {
	// Check for ffi.VM (selector expression)
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == "runtime" && sel.Sel.Name == "VM"
	}

	return false
}

func isPointerToRuntimeObject(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}

	// Check for runtime.Object (selector expression)
	sel, ok := star.X.(*ast.SelectorExpr)
	if ok {
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == "runtime" && sel.Sel.Name == "Object"
	}

	// Also check for just Object (in case it's imported differently)
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "Object"
}

func isSliceOfPointerToRuntimeObject(expr ast.Expr) bool {
	array, ok := expr.(*ast.ArrayType)
	if !ok {
		return false
	}
	return isPointerToRuntimeObject(array.Elt)
}

func isSliceOfPointerToType(expr ast.Expr, typeName string) bool {
	array, ok := expr.(*ast.ArrayType)
	if !ok {
		return false
	}
	return isPointerToType(array.Elt, typeName)
}

func isAnyType(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "any"
}

// generateRegistry creates the registry.gen.go file
func generateRegistry(functions []FFIFunction) error {
	var sb strings.Builder

	sb.WriteString(`// Code generated by generate.go; DO NOT EDIT.

package vm

import (
	"fmt"

	"github.com/akonwi/ard/ffi"
)

// RegisterGeneratedFFIFunctions registers all discovered FFI functions
func (r *RuntimeFFIRegistry) RegisterGeneratedFFIFunctions() error {
`)

	for _, fn := range functions {
		// Use function name directly as binding name
		binding := fn.Name
		functionRef := fmt.Sprintf("ffi.%s", fn.Name)
		sb.WriteString(fmt.Sprintf("\tif err := r.Register(%q, %s); err != nil {\n", binding, functionRef))
		sb.WriteString(fmt.Sprintf("\t\treturn fmt.Errorf(\"failed to register %s: %%w\", err)\n", binding))
		sb.WriteString("\t}\n")
	}

	sb.WriteString(`
	return nil
}
`)

	return os.WriteFile("registry.gen.go", []byte(sb.String()), 0644)
}
