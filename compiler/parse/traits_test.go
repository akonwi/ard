package parse

import (
	"testing"
)

func TestTraitDefinitions(t *testing.T) {
	runTests(t, []test{
		// Error cases
		{
			name:     "Missing trait name",
			input:    "trait { fn test(); }",
			wantErrs: []string{"Expected trait name after 'trait'"},
		},
		{
			name:     "Missing opening brace",
			input:    "trait MyTrait fn test(); }",
			wantErrs: []string{"Expected '{'"},
		},
		{
			name:     "Missing newline after brace",
			input:    "trait MyTrait {fn test(); }",
			wantErrs: []string{"Expected new line after '{'", "Expected a type"},
		},
		{
			name:     "Missing fn keyword",
			input:    "trait MyTrait {\n    test();\n}",
			wantErrs: []string{"Expected function declaration in trait block"},
		},
		{
			name:     "Missing function name",
			input:    "trait MyTrait {\n    fn ();\n}",
			wantErrs: []string{"Expected function name"},
		},
		{
			name:     "Missing opening paren",
			input:    "trait MyTrait {\n    fn test;\n}",
			wantErrs: []string{"Expected '(' after function name"},
		},
		{
			name:     "Missing closing paren",
			input:    "trait MyTrait {\n    fn test(name: string;\n}",
			wantErrs: []string{"Expected ',' between parameters", "Expected ')' after parameters"},
		},
		{
			name:     "Valid simple trait",
			input:    "trait MyTrait {\n    fn test()\n}",
			wantErrs: []string{},
		},
		{
			name:     "Valid trait method with mutable parameter",
			input:    "trait MyTrait {\n    fn test(value: mut Counter)\n}",
			wantErrs: []string{},
		},
		{
			name:     "Empty trait works",
			input:    "trait MyTrait {\n}",
			wantErrs: []string{},
		},
	})
}

func TestTraitMethodReceiverMutability(t *testing.T) {
	result := Parse([]byte(`trait Counter {
  fn mut set(value: Int)
  fn mut()
  fn mut mut()
}`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	trait := result.Program.Statements[0].(*TraitDefinition)
	if got := len(trait.Methods); got != 3 {
		t.Fatalf("method count = %d, want 3", got)
	}
	want := []struct {
		name    string
		mutates bool
	}{
		{name: "set", mutates: true},
		{name: "mut", mutates: false},
		{name: "mut", mutates: true},
	}
	for i, method := range trait.Methods {
		if method.Name != want[i].name || method.Mutates != want[i].mutates {
			t.Fatalf("method %d = %q mutates=%t, want %q mutates=%t", i, method.Name, method.Mutates, want[i].name, want[i].mutates)
		}
	}
}
