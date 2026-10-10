package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// Only declarations may appear at the top level of a module. Executable
// statements have no defined time to run, so they are rejected rather than
// silently ignored (#533).
func TestTopLevelExecutableStatementsAreRejected(t *testing.T) {
	check := func(t *testing.T, source string) *checker.Checker {
		t.Helper()
		result := parse.Parse([]byte(source), "test.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		c := checker.New("test.ard", result.Program, nil)
		c.Check()
		return c
	}

	rejected := []struct {
		name   string
		source string
		row    int
	}{
		{name: "call beside main", source: "fn main() {}\n\nmain()\n", row: 3},
		{name: "expression", source: "42\n", row: 1},
		{name: "assignment to module mut", source: "mut counter = 0\ncounter = counter + 1\n", row: 2},
		{name: "if statement", source: "let ready = true\nif ready {\n  ready\n}\n", row: 2},
		{name: "while loop", source: "mut n = 0\nwhile n < 3 {\n  n = n + 1\n}\n", row: 2},
		{name: "for loop", source: "for i in 0..3 {\n  i\n}\n", row: 1},
		{name: "defer", source: "defer {\n  42\n}\n", row: 1},
		{name: "break", source: "break\n", row: 1},
	}
	for _, tt := range rejected {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			c := check(t, tt.source)
			diagnostics := c.Diagnostics()
			if len(diagnostics) != 1 {
				t.Fatalf("diagnostics = %#v, want one top-level statement error", diagnostics)
			}
			diagnostic := diagnostics[0]
			if diagnostic.Kind != checker.Error || diagnostic.Code != checker.DiagnosticCodeTopLevelStatement {
				t.Fatalf("diagnostic = %#v, want top_level_statement error", diagnostic)
			}
			if got := diagnostic.Primary.Span.Location.Start.Row; got != tt.row {
				t.Fatalf("diagnostic row = %d, want %d", got, tt.row)
			}
			for _, stmt := range c.Module().Program().Statements {
				if stmt.Expr != nil {
					if _, ok := stmt.Expr.(*checker.FunctionDef); !ok {
						t.Fatalf("rejected statement reached the checked program: %#v", stmt.Expr)
					}
				}
			}
		})
	}

	t.Run("allows declarations", func(t *testing.T) {
		c := check(t, `use ard/testing

// a comment
let limit = 3
mut count = 0

type Id = Int
type Value = Int | Str

struct Point {
  x: Int,
}

enum Color {
  red,
  green,
}

trait Named {
  fn name() Str
}

impl Point {
  fn sum() Int { self.x }
}

impl Named for Point {
  fn name() Str { "point" }
}

fn Point::origin() Point { Point{x: 0} }

fn main() {
  count = count + limit
}

test fn checks() Void!Str {
  testing::assert(limit == 3, "limit")
}
`)
		if len(c.Diagnostics()) != 0 {
			t.Fatalf("diagnostics = %#v, want none", c.Diagnostics())
		}
	})
}
