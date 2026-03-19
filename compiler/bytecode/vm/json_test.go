package vm

import "testing"

func TestBytecodeEncodeJsonPrimitives(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{name: "encoding Str", input: `use ard/encode
encode::json("hello")`, want: `"hello"`},
		{name: "encoding Int", input: `use ard/encode
encode::json(200)`, want: `200`},
		{name: "encoding Float", input: `use ard/encode
encode::json(98.6)`, want: `98.6`},
		{name: "encoding Bool", input: `use ard/encode
encode::json(true)`, want: `true`},
	})
}
