package vm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func runBytecode(t *testing.T, input string) any {
	t.Helper()
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}
	tree := result.Program
	c := checker.New("test.ard", tree, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Diagnostics found: %v", c.Diagnostics())
	}

	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}

	vm := New(program)
	res, err := vm.Run("main")
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	if res == nil {
		return nil
	}
	return res.GoValue()
}

func TestBytecodeEmptyProgram(t *testing.T) {
	res := runBytecode(t, "")
	if res != nil {
		t.Fatalf("Expected nil, got %v", res)
	}
}

func TestBytecodeBindingVariables(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{input: `"Alice"`, want: "Alice"},
		{input: `40`, want: 40},
		{input: `true`, want: true},
	}
	for _, test := range tests {
		res := runBytecode(t, strings.Join([]string{
			fmt.Sprintf("let val = %s", test.input),
			"val",
		}, "\n"))
		if res != test.want {
			t.Fatalf("Expected %v, got %v", test.want, res)
		}
	}
}

func TestBytecodeArithmetic(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`let a = 2`,
		`let b = 3`,
		`a * b + 4`,
	}, "\n"))
	if res != 10 {
		t.Fatalf("Expected 10, got %v", res)
	}
}

func TestBytecodeStringConcat(t *testing.T) {
	res := runBytecode(t, `"hello" + " world"`)
	if res != "hello world" {
		t.Fatalf("Expected hello world, got %v", res)
	}
}

func TestBytecodeEquality(t *testing.T) {
	res := runBytecode(t, `1 == 1`)
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}
}

func TestBytecodeIfExpression(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`let val = 3`,
		`if val > 2 { 10 } else { 20 }`,
	}, "\n"))
	if res != 10 {
		t.Fatalf("Expected 10, got %v", res)
	}
}

func TestBytecodeLogicalOps(t *testing.T) {
	res := runBytecode(t, `true and false`)
	if res != false {
		t.Fatalf("Expected false, got %v", res)
	}
	res = runBytecode(t, `true or false`)
	if res != true {
		t.Fatalf("Expected true, got %v", res)
	}
}

func TestBytecodeFunctionCall(t *testing.T) {
	res := runBytecode(t, strings.Join([]string{
		`fn add(a: Int, b: Int) Int { a + b }`,
		`add(2, 3)`,
	}, "\n"))
	if res != 5 {
		t.Fatalf("Expected 5, got %v", res)
	}
}
