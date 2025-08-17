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

func TestEnumDefinitionErrorRecovery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErrs []string
	}{
		{
			name:     "missing enum name",
			input:    "enum { A, B }\nlet x = 5",
			wantErrs: []string{"Expected name after 'enum'"},
		},
		{
			name:     "missing opening brace",
			input:    "enum Color A, B }\nlet x = 5",
			wantErrs: []string{"Expected '{'"},
		},
		{
			name:     "empty first variant",
			input:    "enum Color { , B }\nlet x = 5",
			wantErrs: []string{},
		},
		{
			name:     "empty middle variant",
			input:    "enum Color { A, , C }\nlet x = 5",
			wantErrs: []string{},
		},
		{
			name:     "empty enum",
			input:    "enum Color { }\nlet x = 5",
			wantErrs: []string{},
		},
		{
			name:     "trailing comma works",
			input:    "enum Color { A, B, }\nlet x = 5",
			wantErrs: []string{},
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
					t.Errorf("Expected error %d to contain \"%s\", got \"%s\"", i, wantErr, result.Errors[i].Message)
				}
			}

			// Should still parse subsequent statements successfully
			if result.Program != nil && len(result.Program.Statements) > 0 {
				t.Logf("Successfully recovered and parsed %d statements", len(result.Program.Statements))
			}
		})
	}
}

func TestStructDefinitionErrorRecovery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErrs []string
	}{
		{
			name:     "missing struct name",
			input:    "struct { name: string }\nlet x = 5",
			wantErrs: []string{"Expected name after 'struct'"},
		},
		{
			name:     "missing opening brace",
			input:    "struct Person name: string }\nlet x = 5",
			wantErrs: []string{"Expected '{'"},
		},
		{
			name:     "missing colon after field name",
			input:    "struct Person { name string }\nlet x = 5",
			wantErrs: []string{"Expected ':' after field name", "Expected '}'"},
		},
		{
			name:     "missing comma between fields",
			input:    "struct Person { name: string age: int }\nlet x = 5",
			wantErrs: []string{"Expected ',' or '}' after field type", "Expected '}'"},
		},
		{
			name:     "missing closing brace",
			input:    "struct Person { name: string\nlet x = 5",
			wantErrs: []string{"Expected ':' after field name", "Expected '}'"},
		},
		{
			name:     "empty struct works",
			input:    "struct Person { }\nlet x = 5",
			wantErrs: []string{},
		},
		{
			name:     "trailing comma works",
			input:    "struct Person { name: string, }\nlet x = 5",
			wantErrs: []string{},
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

			// Should still parse subsequent statements successfully
			if result.Program != nil && len(result.Program.Statements) > 0 {
				t.Logf("Successfully recovered and parsed %d statements", len(result.Program.Statements))
			}
		})
	}
}
