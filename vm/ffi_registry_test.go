package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// panic_test_ffi deliberately panics to test panic recovery (test-only function)
func panic_test_ffi(vm *VM, args []*runtime.Object) (*runtime.Object, any) {
	if len(args) != 1 {
		return nil, fmt.Errorf("panic_test_ffi expects 1 argument, got %d", len(args))
	}

	message, ok := args[0].Raw().(string)
	if !ok {
		return nil, fmt.Errorf("panic_test_ffi expects string argument, got %T", args[0].Raw())
	}

	// Deliberately panic to test recovery
	panic("test panic: " + message)
}

// error_type_test returns different error types to test type flexibility
func error_type_test(vm *VM, args []*runtime.Object) (*runtime.Object, any) {
	if len(args) != 1 {
		return nil, fmt.Errorf("error_type_test expects 1 argument, got %d", len(args))
	}

	errorType, ok := args[0].Raw().(string)
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
		return runtime.MakeStr("success"), nil
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
		args := []*runtime.Object{runtime.MakeStr("test message")}

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
		if !result.IsErr() {
			t.Errorf("Expected a Result::err, got: %v", result)
		}

		// Check error message contains context
		errorMsg := result.AsString()
		if !strings.Contains(errorMsg, "panic in FFI function 'test.panic_test_ffi'") {
			t.Errorf("Expected panic context in error message, got: %s", errorMsg)
		}

		if !strings.Contains(errorMsg, "test panic: test message") {
			t.Errorf("Expected original panic message, got: %s", errorMsg)
		}
	})

	t.Run("panic recovery for non-Result type", func(t *testing.T) {
		// Test panic recovery when return type is not Result - should re-panic with context
		args := []*runtime.Object{runtime.MakeStr("test message")}

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
		stringArgs := []*runtime.Object{runtime.MakeStr("string")}
		result, err := registry.Call(vm, "test.error_type_test", stringArgs, stringResultType)

		if err != nil {
			t.Errorf("Expected no Go error, got: %v", err)
		}
		if !result.IsErr() {
			t.Errorf("Expected Error result, got %v", result)
		}

		if result.AsString() != "this is a string error" {
			t.Errorf("Expected string error, got: %v", result.Raw())
		}

		// Test int error
		intResultType := checker.MakeResult(checker.Str, checker.Int)
		intArgs := []*runtime.Object{runtime.MakeStr("int")}
		result, err = registry.Call(vm, "test.error_type_test", intArgs, intResultType)

		if err != nil {
			t.Errorf("Expected no Go error, got: %v", err)
		}
	})

	t.Run("automatic Maybe handling", func(t *testing.T) {
		// Create a test function whose Ard signature returns a Str?
		maybeTestFunc := func(vm *VM, args []*runtime.Object) (*runtime.Object, any) {
			if len(args) != 1 {
				return nil, fmt.Errorf("maybe_test expects 1 argument")
			}

			returnType, ok := args[0].Raw().(string)
			if !ok {
				return nil, fmt.Errorf("maybe_test expects string argument")
			}

			switch returnType {
			case "some":
				return runtime.MakeStr("test_value"), nil
			case "none":
				return nil, nil // VM should convert to None
			default:
				return nil, fmt.Errorf("unknown type: %s", returnType)
			}
		}

		// Register test function
		err := registry.Register("test.maybe_test", maybeTestFunc)
		if err != nil {
			t.Fatalf("Failed to register maybe test function: %v", err)
		}

		// Test Some case
		maybeStrType := checker.MakeMaybe(checker.Str)
		someArgs := []*runtime.Object{runtime.MakeStr("some")}
		result, err := registry.Call(vm, "test.maybe_test", someArgs, maybeStrType)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if result.Raw() != "test_value" {
			t.Errorf("Expected 'test_value', got: %v", result.Raw())
		}

		// Test None case
		noneArgs := []*runtime.Object{runtime.MakeStr("none")}
		result, err = registry.Call(vm, "test.maybe_test", noneArgs, maybeStrType)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if result.Raw() != nil {
			t.Errorf("Expected nil for None, got: %v", result.Raw())
		}
	})
}
