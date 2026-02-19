package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

func panicTestFFI(args []*runtime.Object, _ checker.Type) *runtime.Object {
	if len(args) != 1 {
		panic(fmt.Errorf("panicTestFFI expects 1 arg, got %d", len(args)))
	}
	panic("test panic: " + args[0].AsString())
}

func TestBytecodeFFIPanicRecovery(t *testing.T) {
	registry := NewRuntimeFFIRegistry()
	if err := registry.RegisterBuiltinFFIFunctions(); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	if err := registry.Register("test.panic_test_ffi", panicTestFFI); err != nil {
		t.Fatalf("register panic test ffi: %v", err)
	}

	resultType := checker.MakeResult(checker.Str, checker.Str)
	result, err := registry.Call("test.panic_test_ffi", []*runtime.Object{runtime.MakeStr("test message")}, resultType)
	if err != nil {
		t.Fatalf("expected no Go error, got: %v", err)
	}
	if !result.IsErr() {
		t.Fatalf("expected Result::err, got: %v", result)
	}
	msg := result.AsString()
	if !strings.Contains(msg, "panic in FFI function 'test.panic_test_ffi'") || !strings.Contains(msg, "test panic: test message") {
		t.Fatalf("unexpected panic recovery message: %s", msg)
	}
}
