package bytecode_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/bytecode"
	bytecodevm "github.com/akonwi/ard/bytecode/vm"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

const benchSource = `
fn fib(n: Int) Int {
  match (n <= 1) {
    true => n,
    false => fib(n - 1) + fib(n - 2)
  }
}

fn main() Int {
  fib(20)
}
`

func BenchmarkBytecodeRun(b *testing.B) {
	module := loadBenchModule(b)
	program, err := bytecode.NewEmitter().EmitProgram(module)
	if err != nil {
		b.Fatalf("Emit error: %v", err)
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		b.Fatalf("Verify error: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := bytecodevm.New(program).Run("main"); err != nil {
			b.Fatalf("Bytecode run error: %v", err)
		}
	}
}

func loadBenchModule(b *testing.B) checker.Module {
	result := parse.Parse([]byte(benchSource), "bench.ard")
	if len(result.Errors) > 0 {
		b.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		b.Fatalf("Failed to get working directory: %v", err)
	}
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		b.Fatalf("Failed to init module resolver: %v", err)
	}
	relPath, err := filepath.Rel(workingDir, "bench.ard")
	if err != nil {
		relPath = "bench.ard"
	}
	c := checker.New(relPath, result.Program, moduleResolver)
	c.Check()
	if c.HasErrors() {
		b.Fatalf("Diagnostics found: %v", c.Diagnostics())
	}
	return c.Module()
}
