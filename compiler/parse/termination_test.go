package parse

import (
	"testing"
	"time"
)

// TestParserTerminatesOnUnterminatedDelimiters guards against parser
// non-termination when input ends inside a delimited construct (issue #349).
// Each malformed program must produce errors instead of spinning at EOF.
func TestParserTerminatesOnUnterminatedDelimiters(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{name: "struct literal", input: "fn main() {\n  let x = Foo {"},
		{name: "struct literal open paren", input: "fn main() {\n  let x = Foo { ("},
		{name: "struct literal mut keyword", input: "fn main() {\n  let x = Foo { mut"},
		{name: "generic struct literal", input: "fn main() {\n  let x = Foo<Int> {"},
		{name: "struct literal after field", input: "fn main() {\n  let x = Foo { a: 1,"},
		{name: "list literal", input: "fn main() {\n  let x = [1,"},
		{name: "map literal", input: "fn main() {\n  let x = [\"a\": 1,"},
		{name: "select", input: "fn main() {\n  select {"},
		{name: "call arguments", input: "fn main() {\n  foo(1,"},
		{name: "match", input: "fn main() {\n  match true {"},
		{name: "block", input: "fn main() {"},
		{name: "parameters", input: "fn take(a: Int,"},
		{name: "enum", input: "enum Color {"},
		{name: "enum with variant", input: "enum Color {\n  Red,"},
		{name: "trait", input: "trait T {"},
		{name: "struct definition", input: "struct S {"},
		{name: "struct definition with field", input: "struct S {\n  name: Str,"},
		{name: "impl block", input: "impl S {"},
		{name: "paren mut", input: "fn main() {\n  let x = (mut [1]"},
		{name: "interpolation", input: "fn main() {\n  let x = \"{"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan ParseResult, 1)
			go func() {
				done <- Parse([]byte(tc.input), "test.ard")
			}()
			select {
			case result := <-done:
				if len(result.Errors) == 0 {
					t.Fatal("expected parse errors for unterminated input")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("parser did not terminate on unterminated input")
			}
		})
	}
}
