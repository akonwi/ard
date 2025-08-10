package checker

import (
	"fmt"
	"os"
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
	
	// Map to file path: "runtime" -> "./ffi/runtime.go"
	ffiDir := filepath.Join(r.projectRoot, "ffi")
	filePath = filepath.Join(ffiDir, module+".go")
	
	return filePath, function, nil
}

// ValidateBinding checks if the external binding can be resolved to an existing file
func (r *FFIRegistry) ValidateBinding(binding string) error {
	filePath, functionName, err := r.ResolveBinding(binding)
	if err != nil {
		return err
	}
	
	// Check if the FFI file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("FFI file not found for binding %q: %s", binding, filePath)
	}
	
	// For now, we assume the function exists if the file exists
	// Future enhancement: parse Go file and validate function exists
	_ = functionName
	
	return nil
}

// GetFFIDirectory returns the path to the FFI directory
func (r *FFIRegistry) GetFFIDirectory() string {
	return filepath.Join(r.projectRoot, "ffi")
}
