package parse

import (
	"strings"
	"testing"
)

// parseErrors asserts the input produces at least one parse error (and no
// panic) and returns the error messages.
func parseErrors(t *testing.T, input string) []string {
	t.Helper()
	result := Parse([]byte(input), "test.ard")
	if len(result.Errors) == 0 {
		t.Fatalf("Expected parse errors, got none")
	}
	messages := make([]string, len(result.Errors))
	for i, err := range result.Errors {
		messages[i] = err.Message
	}
	return messages
}

// TestTypeDeclarationInvalidRightHandSide pins that unsupported forms after
// `type Name =` produce parse errors instead of panicking (issue #258).
func TestTypeDeclarationInvalidRightHandSide(t *testing.T) {
	assertHasError := func(t *testing.T, messages []string, want string) {
		t.Helper()
		for _, message := range messages {
			if strings.Contains(message, want) {
				return
			}
		}
		t.Fatalf("Expected an error containing %q, got: %v", want, messages)
	}

	t.Run("struct-shape alias reports a parse error", func(t *testing.T) {
		messages := parseErrors(t, "type Success = { value: Str }\n")
		assertHasError(t, messages, "Expected a type after '='")
	})

	t.Run("struct-shape union arm reports a parse error", func(t *testing.T) {
		messages := parseErrors(t, "type Outcome = Int | { message: Str }\n")
		assertHasError(t, messages, "Expected a type after '|'")
	})

	t.Run("mut with no inner type reports a parse error", func(t *testing.T) {
		messages := parseErrors(t, "type Bad = mut\nfn main() {}\n")
		assertHasError(t, messages, "Expected a type after '='")
	})

	t.Run("mut struct-shape alias reports a parse error", func(t *testing.T) {
		messages := parseErrors(t, "type Bad = mut { x: Int }\n")
		assertHasError(t, messages, "Expected a type after '='")
	})

	t.Run("recovery continues to later declarations", func(t *testing.T) {
		result := Parse([]byte("type Bad = { value: Str }\n\nfn ok() Int {\n  1\n}\n"), "test.ard")
		if len(result.Errors) == 0 {
			t.Fatal("Expected parse errors, got none")
		}
		if result.Program == nil {
			t.Fatal("Expected a program despite errors")
		}
		for _, stmt := range result.Program.Statements {
			if fn, ok := stmt.(*FunctionDeclaration); ok && fn.Name == "ok" {
				return
			}
		}
		t.Fatal("Expected the parser to recover and parse fn ok")
	})
}
