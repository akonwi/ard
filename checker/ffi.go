package checker

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FFIRegistry manages the mapping of external bindings to Go files
type FFIRegistry struct {
	projectRoot string
}

// NewFFIRegistry creates a new FFI registry for the given project root
func NewFFIRegistry(projectRoot string) *FFIRegistry {
	return &FFIRegistry{
		projectRoot: projectRoot,
	}
}

// ResolveBinding takes an external binding like "runtime.go_print"
// and returns the file path and function name
func (r *FFIRegistry) ResolveBinding(binding string) (filePath string, functionName string, err error) {
	// Split binding into module.function format
	parts := strings.Split(binding, ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid external binding format: %q, expected \"module.function\"", binding)
	}

	module := parts[0]
	function := parts[1]

	// Check if we're processing standard library modules
	if filepath.Base(r.projectRoot) == "std_lib" {
		// We're in standard library - FFI functions are in ../vm/ffi_functions.go
		filePath = filepath.Join(filepath.Dir(r.projectRoot), "vm", "ffi_functions.go")
	} else {
		// We're in user project - FFI functions are in ffi/*.go structure
		ffiDir := filepath.Join(r.projectRoot, "ffi")
		filePath = filepath.Join(ffiDir, module+".go")
	}

	return filePath, function, nil
}

// ValidateBinding checks if the external binding can be resolved to an existing file
// todo: is this still necessary?
func (r *FFIRegistry) ValidateBinding(binding string) error {
	// filePath, functionName, err := r.ResolveBinding(binding)
	// if err != nil {
	// 	return err
	// }

	// // Check if the FFI file exists
	// if _, err := os.Stat(filePath); os.IsNotExist(err) {
	// 	return fmt.Errorf("FFI file not found for binding %q: %s", binding, filePath)
	// }

	// // For now, we assume the function exists if the file exists
	// // Future enhancement: parse Go file and validate function exists
	// _ = functionName

	return nil
}

// GetFFIDirectory returns the path to the FFI directory
func (r *FFIRegistry) GetFFIDirectory() string {
	return filepath.Join(r.projectRoot, "ffi")
}
