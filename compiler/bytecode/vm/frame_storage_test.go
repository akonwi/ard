package vm

import (
	"testing"

	"github.com/akonwi/ard/runtime"
)

func TestFrameAnyStoragePushPopAndLookup(t *testing.T) {
	vm := &VM{}
	frame := &Frame{Stack: make([]any, 2), Locals: make([]any, 1)}
	value := runtime.MakeStr("hello")
	frame.Locals[0] = value

	vm.push(frame, frame.Locals[0])
	if frame.StackTop != 1 {
		t.Fatalf("expected stack top 1, got %d", frame.StackTop)
	}

	peek, err := vm.stackObjectAt(frame, 0)
	if err != nil {
		t.Fatalf("stackObjectAt failed: %v", err)
	}
	if peek.AsString() != "hello" {
		t.Fatalf("expected peeked string hello, got %q", peek.AsString())
	}

	popped, err := vm.pop(frame)
	if err != nil {
		t.Fatalf("pop failed: %v", err)
	}
	if popped.AsString() != "hello" {
		t.Fatalf("expected popped string hello, got %q", popped.AsString())
	}
	if frame.StackTop != 0 {
		t.Fatalf("expected stack top 0 after pop, got %d", frame.StackTop)
	}
}
