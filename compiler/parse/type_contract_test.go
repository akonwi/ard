package parse

import (
	"strings"
	"testing"
)

// TestMalformedTypesReportOnce pins the parseType contract (issue #258 class):
// a malformed type in any required position produces at least one parse
// error, never a panic, and reports the missing type exactly once.
func TestMalformedTypesReportOnce(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"type alias", "type Bad = { value: Str }\n"},
		{"type union arm", "type Bad = Int | { value: Str }\n"},
		{"mut with no inner type", "type Bad = mut\nfn main() {}\n"},
		{"struct field", "struct S {\n  f: mut\n}\n"},
		{"struct field struct-shape", "struct S {\n  f: { x: Int }\n}\n"},
		{"function parameter", "fn f(a: mut) Int { 1 }\n"},
		{"function parameter struct-shape", "fn f(a: { x: Int }) Int { 1 }\n"},
		{"variable annotation", "fn main() {\n  let x: mut = 1\n}\n"},
		{"list element type", "fn main() {\n  let x: [mut] = []\n}\n"},
		{"map key type", "fn main() {\n  let x: [mut: Int] = [:]\n}\n"},
		{"map value type", "fn main() {\n  let x: [Str: mut] = [:]\n}\n"},
		{"result error type", "fn f() Int!mut { Result::ok(1) }\n"},
		{"result error type missing", "type Bad = Int!\nfn main() {}\n"},
		{"grouped inner type", "fn main() {\n  let x: (mut)? = 1\n}\n"},
		{"generic type argument", "struct Box { v: Int }\nfn main() {\n  let x: Box<mut> = Box{v: 1}\n}\n"},
		{"fn type parameter", "fn apply(f: fn(mut)) {}\n"},
		{"trait method parameter", "trait T {\n  fn go(a: mut)\n}\n"},
		{"function return type", "fn f() mut {\n  1\n}\n"},
		{"impl method return type", "struct S {}\n\nimpl S {\n  fn m() mut {\n    1\n  }\n}\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) == 0 {
				t.Fatalf("Expected parse errors, got none")
			}
			messages := make([]string, len(result.Errors))
			reports := 0
			for i, err := range result.Errors {
				messages[i] = err.Message
				if strings.Contains(err.Message, "Expected a type") {
					reports++
				}
				// Recovery must not consume a following body block: an eof
				// cascade means a brace-skip swallowed the function body.
				if strings.Contains(err.Message, "Unexpected token: eof") {
					t.Fatalf("Recovery consumed past the malformed type: %v", messages)
				}
			}
			if reports == 0 {
				t.Fatalf("Expected an 'Expected a type' error, got: %v", messages)
			}
			if reports > 1 {
				t.Fatalf("Expected exactly one 'Expected a type' error, got %d: %v", reports, messages)
			}
		})
	}
}

// TestValidTypePositionsStayClean is the positive axis for the parseType
// contract change: positions where a type is genuinely optional (or probed
// speculatively) must not start reporting errors for valid programs.
func TestValidTypePositionsStayClean(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"void function declaration", "fn main() {}\n"},
		{"void closure", "fn main() {\n  let f = fn() {}\n}\n"},
		{"fn type before struct field comma", "struct S {\n  cb: fn(Int),\n  n: Int,\n}\n"},
		{"fn type before closing paren", "fn apply(f: fn(Int)) {}\n"},
		{"fn type as map value", "fn main() {\n  let m: [Str: fn(Int)] = [:]\n}\n"},
		{"fn type as list element", "fn main() {\n  let l: [fn(Int)] = []\n}\n"},
		{"fn type before equals", "fn main() {\n  let f: fn(Int) = fn(n: Int) {}\n}\n"},
		{"fn type as return type", "fn make() fn(Int) Int {\n  fn(n: Int) Int { n }\n}\n"},
		{"grouped nullable fn type", "use ard/maybe\n\nfn main() {\n  let f: (fn(Int) Str)? = maybe::none()\n}\n"},
		{"fn type as call-site type argument", "fn id(v: $T) $T { v }\nfn main() {\n  let a = id<fn(Int)>(fn(n: Int) {})\n}\n"},
		{"trait method without return type", "trait T {\n  fn go(a: Int)\n}\n"},
		{"fn type return followed by body brace", "fn make() fn(Int) {\n  fn(n: Int) {}\n}\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("Expected no parse errors, got: %v", result.Errors)
			}
		})
	}
}
