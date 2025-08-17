package ast

import (
	"strings"
	"testing"
)

func TestParseWithRecoveryInfrastructure(t *testing.T) {
	t.Run("valid code returns no errors", func(t *testing.T) {
		source := `let x = 5`
		result := ParseWithRecovery([]byte(source), "test.ard")

		if len(result.Errors) != 0 {
			t.Errorf("Expected no errors for valid code, got %d", len(result.Errors))
		}
		if result.Program == nil {
			t.Error("Expected program to be parsed")
		}
		if len(result.Program.Statements) != 1 {
			t.Errorf("Expected 1 statement, got %d", len(result.Program.Statements))
		}
	})

	t.Run("empty program returns no errors", func(t *testing.T) {
		source := ``
		result := ParseWithRecovery([]byte(source), "test.ard")

		if len(result.Errors) != 0 {
			t.Errorf("Expected no errors for empty program, got %d", len(result.Errors))
		}
		if result.Program == nil {
			t.Error("Expected program to be parsed")
		}
	})
}

func TestParserErrorRecoveryCore(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErrs []string
	}{
		{
			name:     "import missing path",
			input:    "use\nlet x = 5",
			wantErrs: []string{"Expected module path after 'use'"},
		},
		{
			name:     "import missing alias after as",
			input:    "use ard/fs as\nlet x = 5",
			wantErrs: []string{"Expected alias name after 'as'"},
		},
		{
			name:     "break statement missing newline",
			input:    "break let x = 5",
			wantErrs: []string{"Expected new line"},
		},
		{
			name:     "variable declaration missing equals",
			input:    "let x 5\nlet y = 10",
			wantErrs: []string{"Expected '='"},
		},
		{
			name:     "while loop missing opening brace",
			input:    "while true\nlet x = 5",
			wantErrs: []string{"Expected '{'"},
		},
		{
			name:     "multiple variable declaration errors",
			input:    "let x\nlet y 10\nlet z = 5",
			wantErrs: []string{"Expected '='", "Expected '='"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseWithRecovery([]byte(tt.input), "test.ard")

			if len(result.Errors) != len(tt.wantErrs) {
				t.Errorf("Expected %d errors, got %d: %v", len(tt.wantErrs), len(result.Errors), result.Errors)
				return
			}

			for i, wantErr := range tt.wantErrs {
				if !strings.Contains(result.Errors[i].Message, wantErr) {
					t.Errorf("Expected error %d to contain '%s', got '%s'", i, wantErr, result.Errors[i].Message)
				}
			}

			// For debugging: log successful parsing results
			if len(result.Errors) == 0 && result.Program != nil {
				t.Logf("Successfully parsed %d imports, %d statements",
					len(result.Program.Imports), len(result.Program.Statements))
			}
		})
	}
}
