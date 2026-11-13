package gotarget

import (
	"strings"
	"testing"
)

// Map literal keys are full expressions, so calls, member access, and
// operators work as keys without parentheses. (#581)
func TestMapLiteralExpressionKeys(t *testing.T) {
	program := lowerParitySource(t, `
		struct User {
			id: Int,
		}

		impl User {
			fn next_id() Int {
				self.id + 1
			}
		}

		fn key() Int {
			1
		}

		fn main() [Int: Str] {
			let user = User{id: 10}
			let a = 20
			[
				key(): "call",
				user.id: "member",
				user.next_id(): "method",
				a + 1: "operator",
				-a: "unary",
				(key() * 100): "parenthesized",
			]
		}
	`)
	got := strings.TrimSpace(runGoTargetParityJSON(t, program))
	want := `{"-20":"unary","1":"call","10":"member","100":"parenthesized","11":"method","21":"operator"}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
