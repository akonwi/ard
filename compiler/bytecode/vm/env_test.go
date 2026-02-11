package vm

import (
	"os"
	"testing"
)

func TestBytecodeEnvGet(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "env::get returns Some string for set variables",
			input: `
				use ard/env
				env::get("HOME")
			`,
			want: os.Getenv("HOME"),
		},
		{
			name: "env::get returns None for non-existent variable",
			input: `
				use ard/env
				env::get("NON_EXISTENT_VAR").is_none()
			`,
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}
