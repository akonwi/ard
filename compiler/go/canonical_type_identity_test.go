package gotarget

import (
	"path/filepath"
	"testing"
)

func TestBuildProgramTypesNonReturningOperandsFromContext(t *testing.T) {
	program := lowerSource(t, `
		fn equality() Bool { panic("stop") == 1 }
		fn not_value() Bool { not panic("stop") }
		fn result_ok() Bool { Result::ok(panic("stop")).is_ok() }
		fn result_err() Bool { Result::err(panic("stop")).is_err() }
		fn result_or() Int { Result::err("failed").or(1) }
		fn maybe_or() Int { Maybe::new().or(1) }
		fn result_expect() Int { { Result::err("failed") }.expect("failed") }
		fn maybe_expect() Int { Maybe::new().expect("failed") }
		fn main() {}
	`)
	if _, err := BuildProgram(program, filepath.Join(t.TempDir(), "app")); err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
}

func TestRunProgramWrapsResultsContainingVoidInMaybe(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "Void success",
			source: `
				fn main() {
					let value: Void!Error = Result::ok(())
					let wrapped: (Void!Error)? = Maybe::new<Void!Error>(value)
				}
			`,
		},
		{
			name: "Void error",
			source: `
				fn main() {
					let value: Int!Void = Result::err(())
					let wrapped: (Int!Void)? = Maybe::new<Int!Void>(value)
				}
			`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := lowerSource(t, tt.source)
			if err := RunProgram(program, []string{"ard", "run", "test.ard"}); err != nil {
				t.Fatalf("RunProgram error = %v", err)
			}
		})
	}
}
