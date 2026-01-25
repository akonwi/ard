package vm_test

import (
	"os"
	"testing"
)

func TestEnvGet(t *testing.T) {
	tests := []test{
		{
			name: "env::get returns Some string for set variables",
			input: `use ard/env
env::get("HOME")`,
			want: os.Getenv("HOME"),
		},
		{
			name: "env::get returns None for non-existent variable",
			input: `use ard/env
		env::get("NON_EXISTENT_VAR").is_none()`,
			want: true,
		},
	}

	runTests(t, tests)
}
