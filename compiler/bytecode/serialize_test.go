package bytecode_test

import (
	"os"
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/bytecode/vm"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestSerializeProgram(t *testing.T) {
	result := parse.Parse([]byte("let val = 2\nval + 3"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	resolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		t.Fatalf("Failed to init module resolver: %v", err)
	}
	c := checker.New("test.ard", result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Diagnostics found: %v", c.Diagnostics())
	}

	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}

	data, err := bytecode.SerializeProgram(program)
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}
	decoded, err := bytecode.DeserializeProgram(data)
	if err != nil {
		t.Fatalf("Deserialize error: %v", err)
	}

	res, err := vm.New(decoded).Run("main")
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	if got := res.GoValue(); got != 5 {
		t.Fatalf("Expected 5, got %v", got)
	}
}
