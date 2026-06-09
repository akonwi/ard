package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestStructMethodsAreStoredInProgramSideTable(t *testing.T) {
	result := parse.Parse([]byte(`
		struct Frame {}

		impl Frame {
			fn sub() Frame {
				self
			}
		}
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}

	sym, ok := c.scope.get("Frame")
	if !ok {
		t.Fatal("Frame missing from scope")
	}
	frame, ok := sym.Type.(*StructDef)
	if !ok {
		t.Fatalf("Frame symbol type = %T, want *StructDef", sym.Type)
	}
	if got := frame.get("sub"); got != nil {
		t.Fatalf("StructDef.get returned method %T; methods should not be in value-shape lookup", got)
	}
	if _, ok := c.program.StructMethod(StructMethodOwner(frame), "sub"); !ok {
		t.Fatal("method sub missing from Program.StructMethods side table")
	}
}

func TestStructSideTableMethodUsesGenericBindingsFromNestedFields(t *testing.T) {
	result := parse.Parse([]byte(`
		extern type Chan<$T>

		struct Channel {
			chan: Chan<$T>
		}

		impl Channel {
			fn send(value: $T) Bool {
				true
			}
		}

		extern fn new() Channel<Int> = "NewChannel"

		let ch = new()
		ch.send(42)
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}

func TestStructEqualityIndependentOfMethodSideTable(t *testing.T) {
	left := &StructDef{Name: "Frame", Fields: map[string]Type{}}
	right := &StructDef{Name: "Frame", Fields: map[string]Type{}}
	program := &Program{}
	program.AddStructMethod(StructMethodOwner(right), "sub", &FunctionDef{Name: "sub", ReturnType: right})

	if !left.equal(right) {
		t.Fatal("struct equality should ignore side-table method signatures")
	}
}
