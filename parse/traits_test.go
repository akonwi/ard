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
			wantErrs: []string{"Expected new line after '{'", "Expected function declaration in trait block"},
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
			name:     "Empty trait works",
			input:    "trait MyTrait {\n}",
			wantErrs: []string{},
		},
	})
}
