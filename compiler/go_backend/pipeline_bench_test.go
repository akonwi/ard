package go_backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
)

func benchmarkGoBackendModule(b *testing.B) checker.Module {
	b.Helper()
	dir := b.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"bench\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		b.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	source := `
struct Person {
  name: Str,
  age: Int,
}

fn score(person: Person) Int {
  if person.age > 40 {
    10
  } else {
    20
  }
}

fn greet(person: Person) Str {
  "hi ${person.name}"
}

fn main() Int {
  let person = Person{name: "Ada", age: 42}
  let result = score(person)
  let greeting = greet(person)
  if greeting == "" {
    0
  } else {
    result
  }
}
`
	if err := os.WriteFile(mainPath, []byte(source), 0o644); err != nil {
		b.Fatalf("failed to write main.ard: %v", err)
	}
	module, _, err := loadModule(mainPath)
	if err != nil {
		b.Fatalf("failed to load module: %v", err)
	}
	return module
}

func BenchmarkLowerModuleToBackendIR(b *testing.B) {
	module := benchmarkGoBackendModule(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := lowerModuleToBackendIR(module, "main", true); err != nil {
			b.Fatalf("did not expect error: %v", err)
		}
	}
}

func BenchmarkEmitGoFileFromBackendIR(b *testing.B) {
	module := benchmarkGoBackendModule(b)
	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		b.Fatalf("did not expect error: %v", err)
	}
	imports := collectModuleImports(module.Program().Statements, "bench")
	imports[helperImportPath] = helperImportAlias
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := emitGoFileFromBackendIR(irModule, module, imports, true, "bench"); err != nil {
			b.Fatalf("did not expect error: %v", err)
		}
	}
}

func BenchmarkOptimizeGoFileIR(b *testing.B) {
	module := benchmarkGoBackendModule(b)
	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		b.Fatalf("did not expect error: %v", err)
	}
	imports := collectModuleImports(module.Program().Statements, "bench")
	imports[helperImportPath] = helperImportAlias
	fileIR, err := emitGoFileFromBackendIR(irModule, module, imports, true, "bench")
	if err != nil {
		b.Fatalf("did not expect error: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = optimizeGoFileIR(fileIR)
	}
}

func BenchmarkRenderGoFile(b *testing.B) {
	module := benchmarkGoBackendModule(b)
	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		b.Fatalf("did not expect error: %v", err)
	}
	imports := collectModuleImports(module.Program().Statements, "bench")
	imports[helperImportPath] = helperImportAlias
	fileIR, err := emitGoFileFromBackendIR(irModule, module, imports, true, "bench")
	if err != nil {
		b.Fatalf("did not expect error: %v", err)
	}
	optimized := optimizeGoFileIR(fileIR)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := renderGoFile(optimized); err != nil {
			b.Fatalf("did not expect error: %v", err)
		}
	}
}

func BenchmarkCompileModuleSource(b *testing.B) {
	module := benchmarkGoBackendModule(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := compileModuleSource(module, "main", true, "bench"); err != nil {
			b.Fatalf("did not expect error: %v", err)
		}
	}
}
