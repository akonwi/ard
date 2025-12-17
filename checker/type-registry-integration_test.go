package checker_test

import (
	"testing"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
)

func TestTypeRegistry_AvailableAfterCheck(t *testing.T) {
	// Phase 1: Just verify the registry infrastructure is in place
	// Phase 2 will wire up actual registration during expression checking
	input := `
fn main() {
	let x: Int = 42
	let y: Str = "hello"
	let z: Bool = true
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Checker errors: %v", c.Diagnostics())
	}

	registry := c.Module().TypeRegistry()
	if registry == nil {
		t.Fatal("expected non-nil TypeRegistry after Check()")
	}

	// Phase 1: Just verify the registry exists and is functional
	// The actual registration of types happens in Phase 2
	testType := checker.Int
	id := registry.Next()
	err := registry.Register(id, testType)
	if err != nil {
		t.Fatalf("failed to register test type: %v", err)
	}

	lookedUp := registry.Lookup(id)
	if lookedUp != testType {
		t.Error("registry lookup failed")
	}
}



func TestTypeRegistry_IsAccessibleFromModule(t *testing.T) {
	// Phase 1: Just verify the registry is accessible from the Module
	input := `
fn main() {
	let x: Int = 5
}
`
	result := ast.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	c.Check()

	module := c.Module()
	registry := module.TypeRegistry()

	if registry == nil {
		t.Fatal("expected TypeRegistry to be accessible from Module")
	}

	// Phase 1: Just verify the registry is functional
	// Phase 2 will actually populate it during expression checking
	testID := registry.Next()
	err := registry.Register(testID, checker.Str)
	if err != nil {
		t.Fatalf("failed to register test type: %v", err)
	}

	if registry.Lookup(testID) != checker.Str {
		t.Error("registry is not accessible or not functional from Module")
	}
}
