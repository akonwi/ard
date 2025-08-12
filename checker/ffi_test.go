package checker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFFIRegistry_ResolveBinding(t *testing.T) {
	registry := NewFFIRegistry("/project/root")

	tests := []struct {
		name         string
		binding      string
		expectedFile string
		expectedFunc string
		expectError  bool
	}{
		{
			name:         "Valid binding",
			binding:      "runtime.go_print",
			expectedFile: "/project/root/ffi/runtime.go",
			expectedFunc: "go_print",
			expectError:  false,
		},
		{
			name:         "Math module binding",
			binding:      "math.go_add",
			expectedFile: "/project/root/ffi/math.go",
			expectedFunc: "go_add",
			expectError:  false,
		},
		{
			name:        "Invalid format - no dot",
			binding:     "invalid",
			expectError: true,
		},
		{
			name:        "Invalid format - multiple dots",
			binding:     "module.sub.function",
			expectError: true,
		},
		{
			name:        "Empty binding",
			binding:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath, functionName, err := registry.ResolveBinding(tt.binding)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if filePath != tt.expectedFile {
				t.Errorf("Expected file path %q, got %q", tt.expectedFile, filePath)
			}

			if functionName != tt.expectedFunc {
				t.Errorf("Expected function name %q, got %q", tt.expectedFunc, functionName)
			}
		})
	}
}

func TestFFIRegistry_ValidateBinding(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "ffi_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create ffi directory and a test file
	ffiDir := filepath.Join(tempDir, "ffi")
	if err := os.MkdirAll(ffiDir, 0755); err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(ffiDir, "runtime.go")
	if err := os.WriteFile(testFile, []byte("package main\nfunc go_print() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewFFIRegistry(tempDir)

	tests := []struct {
		name        string
		binding     string
		expectError bool
	}{
		{
			name:        "Valid binding with existing file",
			binding:     "runtime.go_print",
			expectError: false,
		},
		{
			name:        "Invalid binding format",
			binding:     "invalid",
			expectError: true,
		},
		{
			name:        "Missing FFI file",
			binding:     "missing.go_func",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.ValidateBinding(tt.binding)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
