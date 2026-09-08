package gotarget

import "testing"

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
