package formatter

import (
	"testing"

	"github.com/akonwi/ard/compiler/parse"
)

func TestDeclarationSignature(t *testing.T) {
	result := parse.Parse([]byte(`fn watched(  sig:os::Signal ,count: Int) [os::Signal] { [os::Interrupt] }
private fn Point::new(x: Int) Point { Point{x: x} }
mut size: Int = 1
let inferred = 2
private type Fd = Int
impl Point {
  fn b() Int { 1 }
  fn a(x: Int) {}
}
impl Show for Point {
  fn show() Str { "p" }
}
struct Point { x: Int }
`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	want := []struct {
		signature string
		ok        bool
	}{
		{"fn watched(sig: os::Signal, count: Int) [os::Signal]", true},
		{"private fn Point::new(x: Int) Point", true},
		{"mut size: Int", true},
		{"let inferred", true},
		{"private type Fd", true},
		{"impl Point\nfn a(x: Int)\nfn b() Int", true},
		{"impl Show for Point\nfn show() Str", true},
		{"", false},
	}
	if len(result.Program.Statements) != len(want) {
		t.Fatalf("statements = %d", len(result.Program.Statements))
	}
	for index, statement := range result.Program.Statements {
		got, ok := DeclarationSignature(statement)
		if got != want[index].signature || ok != want[index].ok {
			t.Errorf("%T signature = %q, %v; want %q, %v", statement, got, ok, want[index].signature, want[index].ok)
		}
	}
}
