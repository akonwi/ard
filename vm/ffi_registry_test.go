package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
)

// panic_test_ffi deliberately panics to test panic recovery (test-only function)
func panic_test_ffi(vm *VM, args []*object) (*object, any) {
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

// error_type_test returns different error types to test type flexibility
func error_type_test(vm *VM, args []*object) (*object, any) {
	if len(args) != 1 {
		return nil, fmt.Errorf("error_type_test expects 1 argument, got %d", len(args))
	}

	errorType, ok := args[0].raw.(string)
	if !ok {
		return nil, fmt.Errorf("error_type_test expects string argument")
	}

	switch errorType {
	case "string":
		return nil, "this is a string error"
	case "int":
		return nil, 42
	case "bool":
		return nil, true
	default:
		return &object{raw: "success", _type: checker.Str}, nil
	}
}

func TestFFIPanicRecovery(t *testing.T) {
	// Create FFI registry
	registry := NewRuntimeFFIRegistry()
	err := registry.RegisterBuiltinFFIFunctions()
	if err != nil {
		t.Fatalf("Failed to register FFI functions: %v", err)
	}

	// Register test functions
	err = registry.Register("test.panic_test_ffi", panic_test_ffi)
	if err != nil {
		t.Fatalf("Failed to register panic test function: %v", err)
	}

	err = registry.Register("test.error_type_test", error_type_test)
	if err != nil {
		t.Fatalf("Failed to register error type test function: %v", err)
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

	t.Run("flexible error types", func(t *testing.T) {
		// Test that FFI functions can return different error types

		// Test string error
		stringResultType := checker.MakeResult(checker.Str, checker.Str)
		stringArgs := []*object{{raw: "string", _type: checker.Str}}
		result, err := registry.Call(vm, "test.error_type_test", stringArgs, stringResultType)

		if err != nil {
			t.Errorf("Expected no Go error, got: %v", err)
		}

		resultObj := result.raw.(_result)
		if resultObj.ok {
			t.Error("Expected Error result, got Ok result")
		}

		if resultObj.raw.raw.(string) != "this is a string error" {
			t.Errorf("Expected string error, got: %v", resultObj.raw.raw)
		}

		// Test int error
		intResultType := checker.MakeResult(checker.Str, checker.Int)
		intArgs := []*object{{raw: "int", _type: checker.Str}}
		result, err = registry.Call(vm, "test.error_type_test", intArgs, intResultType)

		if err != nil {
			t.Errorf("Expected no Go error, got: %v", err)
		}

		resultObj = result.raw.(_result)
		if resultObj.ok {
			t.Error("Expected Error result, got Ok result")
		}

		if resultObj.raw.raw.(int) != 42 {
			t.Errorf("Expected int error 42, got: %v", resultObj.raw.raw)
		}
	})
}
