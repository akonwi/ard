package checker_test

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// TestNilTypeGuard pins the checker's defense-in-depth for the parser's type
// contract (issue #258 class): a nil DeclaredType in the tree is an internal
// parser bug when the parse was clean, but an expected recovery artifact when
// the tree carries parse errors (the LSP checks such trees).
func TestNilTypeGuard(t *testing.T) {
	// A struct field with a nil type, as parser recovery would leave behind.
	makeProgram := func() *parse.Program {
		return &parse.Program{
			Statements: []parse.Statement{
				&parse.StructDefinition{
					Name: parse.Identifier{Name: "S"},
					Fields: []parse.StructField{
						{Name: parse.Identifier{Name: "f"}, Type: nil},
					},
				},
			},
		}
	}

	t.Run("clean parse reports an internal error", func(t *testing.T) {
		c := checker.New("test.ard", makeProgram(), nil, checker.CheckOptions{})
		c.Check()
		found := false
		for _, diagnostic := range c.Diagnostics() {
			if strings.Contains(diagnostic.Message, "internal error") {
				found = true
			}
		}
		if !found {
			t.Fatalf("Expected an internal error diagnostic, got: %v", c.Diagnostics())
		}
	})

	t.Run("error-carrying tree degrades silently", func(t *testing.T) {
		c := checker.New("test.ard", makeProgram(), nil, checker.CheckOptions{HasParseErrors: true})
		c.Check()
		for _, diagnostic := range c.Diagnostics() {
			if strings.Contains(diagnostic.Message, "internal error") {
				t.Fatalf("Expected no internal error diagnostic on an error-carrying tree, got: %v", c.Diagnostics())
			}
		}
	})
}
