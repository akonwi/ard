package parser

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func assertEquality(t *testing.T, got, want string) {
	t.Helper()
	diff := cmp.Diff(want, got, cmp.Transformer("SpaceRemover", strings.TrimSpace))
	if diff != "" {
		t.Errorf("Output does not match (-want +got):\n%s", diff)
	}
}

func TestEmptyDocument(t *testing.T) {
	doc := MakeDoc("")
	assertEquality(t, doc.String(), "")
}

func TestLines(t *testing.T) {
	doc := MakeDoc("")
	doc.Line("line 1")
	doc.Line("")
	doc.Line("line 2")
	doc.Line("line 3")

	want := `
line 1

line 2
line 3`
	assertEquality(t, doc.String(), want)
}

func TestIndents(t *testing.T) {
	doc := MakeDoc("")
	doc.Line("line 1")
	doc.Indent()
	doc.Line("nested 1")
	doc.Line("nested 2")
	doc.Indent()
	doc.Line("nested nested 1")
	doc.Dedent()
	doc.Line("nested 3")
	doc.Dedent()
	doc.Line("line 2")

	want := `
line 1
  nested 1
  nested 2
    nested nested 1
  nested 3
line 2`

	assertEquality(t, doc.String(), want)
}

func TestNesting(t *testing.T) {
	whileBlock := MakeDoc("")
	whileBlock.Line("print('volcano is stable')")
	whileBlock.Line("volcano.heat()")

	doc := MakeDoc("")
	doc.Line("let volcano = find_volcano()")
	doc.Line("while volcano.is_stable {")
	doc.Nest(whileBlock)
	doc.Line("}")
	doc.Line("volcano.erupt()")

	want := `
let volcano = find_volcano()
while volcano.is_stable {
  print('volcano is stable')
  volcano.heat()
}
volcano.erupt()`

	assertEquality(t, doc.String(), want)
}

func TestAppending(t *testing.T) {
	imports := MakeDoc("")
	imports.Line("use ard/io")
	imports.Line("use my_library/foo")

	code := MakeDoc("")
	code.Line(`io.print("hello")`)
	code.Line("let bar = foo.Bar{}")
	code.Line("")
	code.Line("for thing in bar.things {")
	code.Nest(MakeDoc("io.print(thing)"))
	code.Line("}")

	doc := MakeDoc("")
	doc.Append(imports)
	doc.Line("")
	doc.Append(code)

	want := `
use ard/io
use my_library/foo

io.print("hello")
let bar = foo.Bar{}

for thing in bar.things {
  io.print(thing)
}`
	assertEquality(t, doc.String(), want)
}
