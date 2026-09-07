package gotarget

import "testing"

func TestGoTargetParityFinalRecursiveLocalDeclarations(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "ordinary function block",
			source: `
				fn make_add(base: Int) fn(Int) Int {
					fn add_inner(value: Int) Int {
						if value == 0 { base } else { 1 + add_inner(value - 1) }
					}
				}
				fn main() Int { make_add(40)(2) }
			`,
		},
		{
			name: "global initializer block",
			source: `
				let add: fn(Int) Int = {
					let base = 40
					fn add_inner(value: Int) Int {
						if value == 0 { base } else { 1 + add_inner(value - 1) }
					}
				}
				fn main() Int { add(2) }
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := lowerParitySource(t, test.source)
			if got := runGoTargetParityJSON(t, program); got != "42" {
				t.Fatalf("got %s, want 42", got)
			}
		})
	}
}
