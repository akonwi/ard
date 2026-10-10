package gotarget

import (
	"strings"
	"testing"
)

// `%` shares precedence and left associativity with `*` and `/`, so mixed
// arithmetic evaluates the way it does in Go and most other languages. (#530)
func TestModuloPrecedenceEvaluation(t *testing.T) {
	program := lowerParitySource(t, `
		fn wrap(a: Int, d: Int, c: Int) Int {
			((a + d) % c + c) % c
		}

		fn main() [Int] {
			[
				7 % 3 + 1,
				7 % 3 - 1,
				1 + 7 % 3,
				7 % 3 * 2,
				2 * 7 % 3,
				20 % 7 % 4,
				(7 % 3) + 1,
				wrap(-1, 0, 3),
			]
		}
	`)
	got := strings.TrimSpace(runGoTargetParityJSON(t, program))
	want := "[2,0,2,2,2,2,2,2]"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
