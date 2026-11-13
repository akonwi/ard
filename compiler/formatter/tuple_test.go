package formatter

import (
	"strings"
	"testing"
)

func TestFormatTupleTypes(t *testing.T) {
	input := `struct Span {
bounds:(Int,Int),
}
fn swap(pair:(Int,Str)) (Str,Int) {
pair
}
fn find(maybe:(Int,Str)?,result:(Int,Str)!Str,nested:((Int,Int),[Str]),callback:fn(Int) (Int,Int)) {}
let pairs:[(Int,Str)]=[]
`
	want := `struct Span {
  bounds: (Int, Int),
}
fn swap(pair: (Int, Str)) (Str, Int) {
  pair
}
fn find(
  maybe: (Int, Str)?,
  result: (Int, Str)!Str,
  nested: ((Int, Int), [Str]),
  callback: fn(Int) (Int, Int),
) {}
let pairs: [(Int, Str)] = []
`

	assertDerefFormat(t, input, want)
}

func TestFormatTupleLiteralsAndDestructuring(t *testing.T) {
	input := `fn main() {
let pair=(1,"a")
let nested=((1,2),[3],true,)
let number,text=pair
mut count,_,rest=triple()
let grouped=(1+2)*3
use_pair((1,"a"))
(text,number)
}
`
	want := `fn main() {
  let pair = (1, "a")
  let nested = ((1, 2), [3], true)
  let number, text = pair
  mut count, _, rest = triple()
  let grouped = (1 + 2) * 3
  use_pair((1, "a"))
  (text, number)
}
`

	assertDerefFormat(t, input, want)
}

// Parentheses around destructured names and a comma after the last name parse
// with a warning; formatting removes them.
func TestFormatRemovesRedundantDestructuringSyntax(t *testing.T) {
	input := `fn main() {
  let (number, text) = pair
  mut (count, _, rest,) = triple()
  let low, high, = bounds()
}
`
	want := `fn main() {
  let number, text = pair
  mut count, _, rest = triple()
  let low, high = bounds()
}
`

	assertDerefFormat(t, input, want)
}

func TestFormatBreaksLongTupleLiterals(t *testing.T) {
	input := `fn main() {
  let value = (first_long_identifier_name, second_long_identifier_name, third_long_identifier_name, fourth_long_identifier_name)
}
`
	want := `fn main() {
  let value = (
    first_long_identifier_name,
    second_long_identifier_name,
    third_long_identifier_name,
    fourth_long_identifier_name,
  )
}
`

	assertDerefFormat(t, input, want)
}

func TestFormatKeepsCommentsInTupleLiterals(t *testing.T) {
	input := `fn bounds() (Int, Int) {
  (
    1,
    // the upper bound is inclusive
    2,
  )
}
`

	assertDerefFormat(t, input, input)
}

func TestUnusedImportRemovalKeepsTupleUses(t *testing.T) {
	inputs := []struct {
		name  string
		input string
	}{
		{
			name:  "module used only in a tuple type",
			input: "use go:example.com/ui\n\nfn f(pair: (ui::Theme, Int)) {}\n",
		},
		{
			name:  "module used only in a tuple literal",
			input: "use go:example.com/ui\n\nfn f() {\n  let pair = (ui::Default(), 1)\n}\n",
		},
		{
			name:  "module used only in a destructured value",
			input: "use go:example.com/ui\n\nfn f() {\n  let theme, size = ui::Load()\n}\n",
		},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format([]byte(tt.input), "test.ard")
			if err != nil {
				t.Fatalf("format failed: %v", err)
			}
			if !strings.Contains(string(got), "use go:example.com/ui") {
				t.Fatalf("expected import to be preserved, got:\n%s", got)
			}
		})
	}
}
