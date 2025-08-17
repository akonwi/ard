package ast

import "testing"

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

func TestParserErrorRecovery(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedErrors []string // Expected error messages, empty array means no errors
	}{
		{
			name: "import missing path",
			input: `use
use ard/fs
use valid/module`,
			expectedErrors: []string{"Expected module path after 'use'"},
		},
		{
			name: "import missing alias after as",
			input: `use ard/fs as
use another/module`,
			expectedErrors: []string{"Expected alias name after 'as'"},
		},
		{
			name: "multiple import errors with recovery",
			input: `use
use ard/fs as
use valid/module
use another/bad/path as`,
			expectedErrors: []string{
				"Expected module path after 'use'",
				"Expected alias name after 'as'",
				"Expected alias name after 'as'",
			},
		},
		{
			name: "valid imports with newlines",
			input: `use ard/fs

use another/module

use third/module`,
			expectedErrors: []string{}, // No errors expected
		},
		{
			name: "import errors followed by valid statements",
			input: `use
use ard/fs
let x = 5`,
			expectedErrors: []string{"Expected module path after 'use'"},
		},
		{
			name:           "break statement missing newline",
			input:          `break let x = 5`,
			expectedErrors: []string{"Expected new line"},
		},
		{
			name:           "variable declaration missing equals",
			input:          `let x 5`,
			expectedErrors: []string{"Expected '=' after variable name"},
		},
		{
			name: "multiple variable declaration errors",
			input: `let x 5
let z = 10`,
			expectedErrors: []string{
				"Expected '=' after variable name",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseWithRecovery([]byte(tt.input), "test.ard")

			// Parser should always return a program (even with errors)
			if result.Program == nil {
				t.Error("Expected program to be returned, got nil")
				return
			}

			// Check error count
			if len(result.Errors) != len(tt.expectedErrors) {
				t.Errorf("Expected %d errors, got %d", len(tt.expectedErrors), len(result.Errors))
				for i, err := range result.Errors {
					t.Logf("Error %d: %s", i, err.Message)
				}
				return
			}

			// Check error messages
			for i, expectedMsg := range tt.expectedErrors {
				if i < len(result.Errors) {
					if result.Errors[i].Message != expectedMsg {
						t.Errorf("Error %d: expected %q, got %q", i, expectedMsg, result.Errors[i].Message)
					}
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
