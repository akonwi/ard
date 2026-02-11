package vm

import "testing"

func TestBytecodeVMParityCoreExpressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "reassigning variables",
			input: `
				mut val = 1
				val = 2
				val = 3
				val
			`,
			want: 3,
		},
		{name: "unary not", input: `not true`, want: false},
		{name: "unary negative float", input: `-20.1`, want: -20.1},
		{name: "arithmetic precedence", input: `30 + (20 * 4)`, want: 110},
		{name: "chained comparisons", input: `200 <= 250 <= 300`, want: true},
		{
			name: "if/else-if/else",
			input: `
				let is_on = false
				mut result = ""
				if is_on { result = "then" }
				else if result.size() == 0 { result = "else if" }
				else { result = "else" }
				result
			`,
			want: "else if",
		},
		{
			name: "inline block expression",
			input: `
				let value = {
					let x = 10
					let y = 32
					x + y
				}
				value
			`,
			want: 42,
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
