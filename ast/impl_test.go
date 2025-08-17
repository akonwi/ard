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
	})
}
