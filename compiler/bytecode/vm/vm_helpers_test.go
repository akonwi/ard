package vm

import (
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func runBytecodeInDir(t *testing.T, dir, filename, input string) any {
	t.Helper()

	result := parse.Parse([]byte(input), filename)
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	resolver, err := checker.NewModuleResolver(dir)
	if err != nil {
		t.Fatalf("Failed to init module resolver: %v", err)
	}

	c := checker.New(filename, result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Diagnostics found: %v", c.Diagnostics())
	}

	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		t.Fatalf("Verify error: %v", err)
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
