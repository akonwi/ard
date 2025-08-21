package ast

import (
	"testing"
)

func TestImplBlocks(t *testing.T) {
	runTests(t, []test{
		// Error cases
		{
			name:     "Missing impl type name",
			input:    "impl { fn test() {} }",
			wantErrs: []string{"Expected type name after 'impl'"},
		},
		{
			name:     "Missing opening brace",
			input:    "impl Person fn test() {} }",
			wantErrs: []string{"Expected '{'"},
		},
		{
			name:     "Missing newline after brace",
			input:    "impl Person {fn test() {} }",
			wantErrs: []string{"Expected new line after '{'"},
		},
		{
			name:     "Valid empty impl block",
			input:    "impl Person {\n}",
			wantErrs: []string{},
		},
		// Trait implementation error cases
		{
			name:     "Missing trait name after impl",
			input:    "impl for Person {\n}",
			wantErrs: []string{"Expected trait name after 'impl'"},
		},
		{
			name:     "Missing 'for' keyword",
			input:    "impl Display Person {\n}",
			wantErrs: []string{"Expected 'for' after trait name"},
		},
		{
			name:     "Missing type name after 'for'",
			input:    "impl Display for {\n}",
			wantErrs: []string{"Expected type name after 'for'"},
		},
		{
			name:     "Missing opening brace in trait impl",
			input:    "impl Display for Person fn test() {}",
			wantErrs: []string{"Expected '{' after type name"},
		},
		{
			name:     "Missing newline after brace in trait impl",
			input:    "impl Display for Person {fn test() {}}",
			wantErrs: []string{"Expected new line"},
		},
		{
			name:     "Valid trait implementation",
			input:    "impl Display for Person {\n}",
			wantErrs: []string{},
		},
	})
}
