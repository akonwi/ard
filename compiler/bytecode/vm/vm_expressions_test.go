package vm

import (
	"testing"

	"github.com/akonwi/ard/formatter"
)

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

func TestBytecodeVMNotGroupingChangesSemantics(t *testing.T) {
	if got := runBytecode(t, `(not false) and false`); got != false {
		t.Fatalf("Expected (not false) and false to be false, got %v", got)
	}

	if got := runBytecode(t, `not false and false`); got != true {
		t.Fatalf("Expected not false and false to be true, got %v", got)
	}
}

func TestBytecodeVMFormatterPreservesGroupedNotSemantics(t *testing.T) {
	input := `(not false) and false`
	formatted, err := formatter.Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}

	before := runBytecode(t, input)
	after := runBytecode(t, string(formatted))

	if before != false {
		t.Fatalf("Expected source to evaluate to false, got %v", before)
	}
	if after != false {
		t.Fatalf("Expected formatted source to evaluate to false, got %v", after)
	}
}
