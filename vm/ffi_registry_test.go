package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
)

// panic_test_ffi deliberately panics to test panic recovery (test-only function)
func panic_test_ffi(vm *VM, args []*object) (*object, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("panic_test_ffi expects 1 argument, got %d", len(args))
	}

	message, ok := args[0].raw.(string)
	if !ok {
		return nil, fmt.Errorf("panic_test_ffi expects string argument, got %T", args[0].raw)
	}

	// Deliberately panic to test recovery
	panic("test panic: " + message)
}

func TestFFIPanicRecovery(t *testing.T) {
	// Create FFI registry
	registry := NewRuntimeFFIRegistry()
	err := registry.RegisterBuiltinFFIFunctions()
	if err != nil {
		t.Fatalf("Failed to register FFI functions: %v", err)
	}

	// Register test panic function
	err = registry.Register("test.panic_test_ffi", panic_test_ffi)
	if err != nil {
		t.Fatalf("Failed to register test function: %v", err)
	}

	// Create a mock VM
	vm := &VM{}

	t.Run("panic recovery for Result type", func(t *testing.T) {
		// Test panic recovery when return type is Result
		resultType := checker.MakeResult(checker.Str, checker.Str)
		args := []*object{{raw: "test message", _type: checker.Str}}

		// This should recover the panic and return an Ard Error result
		result, err := registry.Call(vm, "test.panic_test_ffi", args, resultType)

		// Should not return Go error
		if err != nil {
			t.Errorf("Expected no Go error, got: %v", err)
		}

		// Should return Ard Error result
		if result == nil {
			t.Fatal("Expected result, got nil")
		}

		// Check it's an Error result
		resultObj, ok := result.raw.(_result)
		if !ok {
			t.Errorf("Expected _result type, got: %T", result.raw)
		}

		if resultObj.ok {
			t.Error("Expected Error result, got Ok result")
		}

		// Check error message contains context
		errorMsg, ok := resultObj.raw.raw.(string)
		if !ok {
			t.Errorf("Expected string error message, got: %T", resultObj.raw.raw)
		}

		if !strings.Contains(errorMsg, "panic in FFI function 'test.panic_test_ffi'") {
			t.Errorf("Expected panic context in error message, got: %s", errorMsg)
		}

		if !strings.Contains(errorMsg, "test panic: test message") {
			t.Errorf("Expected original panic message, got: %s", errorMsg)
		}
	})

	t.Run("panic recovery for non-Result type", func(t *testing.T) {
		// Test panic recovery when return type is not Result - should re-panic with context
		args := []*object{{raw: "test message", _type: checker.Str}}

		defer func() {
			if r := recover(); r != nil {
				panicMsg, ok := r.(string)
				if !ok {
					t.Errorf("Expected string panic, got: %T", r)
					return
				}

				if !strings.Contains(panicMsg, "panic in FFI function 'test.panic_test_ffi'") {
					t.Errorf("Expected panic context, got: %s", panicMsg)
				}

				if !strings.Contains(panicMsg, "test panic: test message") {
					t.Errorf("Expected original panic message, got: %s", panicMsg)
				}
			} else {
				t.Error("Expected panic, but none occurred")
			}
		}()

		// This should re-panic with enhanced context
		_, _ = registry.Call(vm, "test.panic_test_ffi", args, checker.Str)
	})
}
