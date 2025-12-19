package checker

import (
	"github.com/akonwi/ard/parser"
	"github.com/akonwi/ard/std_lib"
)

// EmbeddedModule represents a standard library module loaded from embedded .ard files
type EmbeddedModule struct {
	path          string
	program       *Program
	publicSymbols map[string]Symbol
}

func (m EmbeddedModule) Path() string {
	return m.path
}

func (m EmbeddedModule) Program() *Program {
	return m.program
}

func (m EmbeddedModule) Get(name string) Symbol {
	return m.publicSymbols[name]
}

// findEmbeddedModule loads a .ard standard library module from embedded files
func FindEmbeddedModule(path string) (Module, bool) {
	// Read the embedded file using std_lib.Find
	content, err := std_lib.Find(path)
	if err != nil {
		return nil, false
	}

	// Parse the .ard file
	result := parser.Parse(content, path)
	if len(result.Errors) > 0 {
		return nil, false
	}
	program := result.Program

	// Type check the program to create a Program with symbols
	// Use the check function which returns a Module and diagnostics
	module, diagnostics := check(program, nil, path)
	if len(diagnostics) > 0 {
		// For now, we'll continue even with diagnostics
		// In a production system, you might want to handle this differently

	}

	return module, true
}
