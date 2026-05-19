package air

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestValidateEntrypointSignature(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name: "allows argless Void main",
			source: `fn main() Void {
			}`,
		},
		{
			name: "rejects main with args",
			source: `fn main(name: Str) Void {
			}`,
			wantErr: "main entrypoint cannot have parameters",
		},
		{
			name: "rejects main returning non-Void",
			source: `fn main() Int {
			  1
			}`,
			wantErr: "main entrypoint must return Void, got Int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := lowerEntrypointValidationSource(t, tt.source)
			err := ValidateEntrypointSignature(program)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func lowerEntrypointValidationSource(t *testing.T, input string) *Program {
	t.Helper()
	result := parse.Parse([]byte(input), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}
