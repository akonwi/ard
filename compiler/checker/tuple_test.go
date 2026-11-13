package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestTupleTypes(t *testing.T) {
	run(t, []test{
		{
			name:  "tuple literal infers its type",
			input: `let pair: (Int, Str) = (1, "a")`,
		},
		{
			name: "tuple type is inferred without an annotation",
			input: `let pair = (1, "a")
let same: (Int, Str) = pair`,
		},
		{
			name:  "expected element types contextually type the literal",
			input: `let pair: ([Str], Byte, Int?) = ([], 2, Maybe::new())`,
		},
		{
			name:  "nested tuples",
			input: `let nested: ((Int, Str), Bool) = ((1, "a"), true)`,
		},
		{
			name: "tuples are function parameters and results",
			input: `fn swap(pair: (Int, Str)) (Str, Int) {
  let number, text = pair
  (text, number)
}

let swapped: (Str, Int) = swap((1, "a"))`,
		},
		{
			name: "tuples compose with Maybe, Result, lists, and struct fields",
			input: `struct Span {
  bounds: (Int, Int),
}

fn find() (Int, Str)? {
  Maybe::new((1, "a"))
}

fn load() (Int, Str)!Str {
  Result::ok((1, "a"))
}

let spans: [(Int, Int)] = [(1, 2), (3, 4)]
let span = Span{bounds: (0, 1)}`,
		},
		{
			name: "generic tuple elements are inferred",
			input: `fn first(pair: ($A, $B)) $A {
  let a, _ = pair
  a
}

fn pair_of(a: $A, b: $B) ($A, $B) {
  (a, b)
}

let number: Int = first((1, "a"))
let made: (Str, Bool) = pair_of("a", true)`,
		},
		{
			name: "tuples can be wrapped as Any",
			input: `let pair = (1, "a")
let value: Any = pair`,
		},
		{
			name: "element types participate in type identity",
			input: `let a = (1, "a")
let b: (Int, Int) = a`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected (Int, Int), got (Int, Str)"},
			},
		},
		{
			name: "arity participates in type identity",
			input: `let a = (1, 2, 3)
let b: (Int, Int) = a`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected (Int, Int), got (Int, Int, Int)"},
			},
		},
		{
			name:  "literal elements are checked against the expected tuple",
			input: `let pair: (Int, Str) = (1, 2)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name:  "literal arity is checked against the expected tuple",
			input: `let pair: (Int, Str) = (1, "a", true)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected (Int, Str), got (Int, Str, Bool)"},
			},
		},
		{
			name: "tuples have no equality",
			input: `let a = (1, 2)
let same = a == (1, 2)`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Invalid: (Int, Int) == (Int, Int)"},
			},
		},
		{
			name:  "tuples are not map keys",
			input: `let scores: [(Int, Int): Str] = [:]`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Invalid map key type (Int, Int): map keys must be comparable (primitives, enums, or structs)"},
			},
		},
		{
			name: "tuples have no members",
			input: `let pair = (1, "a")
let first = pair.first`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined: pair.first"},
			},
		},
	})
}

func TestTupleDestructuring(t *testing.T) {
	run(t, []test{
		{
			name: "binds each element with its type",
			input: `fn main() {
  let number, text, flag = (1, "a", true)
  let a: Int = number
  let b: Str = text
  let c: Bool = flag
}`,
		},
		{
			name: "mut makes every binding writable",
			input: `fn main() {
  mut count, label = (1, "a")
  count = 2
  label = "b"
}`,
		},
		{
			name: "let bindings are immutable",
			input: `fn main() {
  let count, label = (1, "a")
  count = 2
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Immutable variable: count"},
			},
		},
		{
			name: "underscore discards an element",
			input: `fn main() {
  let _, text, _ = (1, "a", true)
  let b: Str = text
}`,
		},
		{
			name: "discarded elements are not bound",
			input: `fn main() {
  let _, text = (1, "a")
  let n = _
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Undefined variable: _"},
			},
		},
		{
			name: "name count must match the tuple arity",
			input: `fn main() {
  let a, b = (1, "a", true)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Cannot destructure (Int, Str, Bool) into 2 names"},
			},
		},
		{
			name: "a failed destructuring still binds its names",
			input: `fn main() {
  let a, b = (1, "a", true)
  let c = a
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Cannot destructure (Int, Str, Bool) into 2 names"},
			},
		},
		{
			name: "only tuples can be destructured",
			input: `fn main() {
  let value = 1
  let a, b = value
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Cannot destructure Int: expected a tuple"},
			},
		},
		{
			name: "a nullable tuple must be unwrapped first",
			input: `fn find() (Int, Str)? {
  Maybe::new((1, "a"))
}

fn main() {
  let a, b = find()
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Cannot destructure (Int, Str)?: expected a tuple"},
			},
		},
		{
			name: "names in one destructuring must be distinct",
			input: `fn main() {
  let a, a = (1, 2)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Duplicate name in tuple destructuring: a"},
			},
		},
		{
			name: "try unwraps a fallible tuple before destructuring",
			input: `fn load() (Int, Str)!Str {
  Result::ok((1, "a"))
}

fn use_it() Int!Str {
  let number, _ = try load()
  Result::ok(number)
}`,
		},
		{
			name: "destructuring is not the value of a block",
			input: `fn make() Int {
  let a, b = (1, 2)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Void"},
			},
		},
	})
}

func TestGoMultiResultFunctionsReturnTuples(t *testing.T) {
	run(t, []test{
		{
			name: "two independent results become a tuple",
			input: `use go:context

fn main() {
  let ctx, cancel = context::WithCancel(context::Background())
  let same: context::Context = ctx
  let stop: context::CancelFunc = cancel
}`,
		},
		{
			name: "the tuple is an ordinary value",
			input: `use go:math

fn split(value: Float64) (Float64, Float64) {
  math::Modf(value)
}

fn main() {
  let parts: (Float64, Float64) = split(1.5)
  let whole, fraction = parts
}`,
		},
		{
			name: "a trailing error wraps the remaining results in a Result",
			input: `use go:strings

fn main() {
  let reader = strings::NewReader("héllo")
  let read: (Int32, Int)!Error = reader.ReadRune()
}`,
		},
		{
			name: "try unwraps the tuple of a fallible multi-result call",
			input: `use go:os

fn open_pipe() Void!Error {
  let reader, writer = try os::Pipe()
  try reader.Close()
  writer.Close()
}`,
		},
		{
			name: "a trailing bool after two or more values stays an element",
			input: `use go:strings

fn main() {
  let before, after, found = strings::Cut("key=value", "=")
  let a: Str = before
  let b: Str = after
  let c: Bool = found
}`,
		},
		{
			name: "value and bool pairs stay Maybe",
			input: `use go:strings

fn main() {
  let rest: Str? = strings::CutPrefix("key=value", "key=")
}`,
		},
		{
			name: "methods with multiple results return tuples",
			input: `use go:time

fn main() {
  let year, month, day = time::Now().Date()
  let y: Int = year
  let m: time::Month = month
  let d: Int = day
}`,
		},
		{
			name: "multi-result functions are tuple-returning function values",
			input: `use go:math

fn main() {
  let split: fn(Float64) (Float64, Float64) = math::Modf
  let whole, fraction = split(1.5)
}`,
		},
	})
}

func TestTupleDestructuringDiagnosticsAreStructured(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		legacyMessage string
		text          string
		secondaries   int
	}{
		{
			name:          "name count",
			source:        "fn main() {\n  let a, b = (1, \"a\", true)\n}\n",
			legacyMessage: "Cannot destructure (Int, Str, Bool) into 2 names",
			text:          "Bind every element, using `_` for the ones to discard.",
			secondaries:   1,
		},
		{
			name:          "non-tuple value",
			source:        "fn main() {\n  let a, b = 1\n}\n",
			legacyMessage: "Cannot destructure Int: expected a tuple",
		},
		{
			name:          "nullable tuple",
			source:        "fn find() (Int, Str)? {\n  Maybe::new()\n}\n\nfn main() {\n  let a, b = find()\n}\n",
			legacyMessage: "Cannot destructure (Int, Str)?: expected a tuple",
			text:          "Unwrap the Maybe first, for example with `try`, `match`, or `.or(...)`.",
		},
		{
			name:          "fallible tuple",
			source:        "fn load() (Int, Str)!Str {\n  Result::err(\"no\")\n}\n\nfn main() {\n  let a, b = load()\n}\n",
			legacyMessage: "Cannot destructure (Int, Str)!Str: expected a tuple",
			text:          "Unwrap the Result first, for example with `try` or `match`.",
		},
		{
			name:          "pointer to a tuple",
			source:        "fn main() {\n  mut pair = (1, \"a\")\n  let pointer = &mut pair\n  let a, b = pointer\n}\n",
			legacyMessage: "Cannot destructure &mut (Int, Str): expected a tuple",
			text:          "Read the tuple through the pointer first with `.*`.",
		},
		{
			name:          "duplicate name",
			source:        "fn main() {\n  let a, a = (1, 2)\n}\n",
			legacyMessage: "Duplicate name in tuple destructuring: a",
			secondaries:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := checker.New("main.ard", result.Program, nil)
			c.Check()
			if len(c.Diagnostics()) != 1 {
				t.Fatalf("diagnostics = %v, want exactly one", c.Diagnostics())
			}
			diagnostic := requireDiagnosticCode(t, c.Diagnostics(), checker.DiagnosticCodeInvalidTupleDestructuring)
			if diagnostic.Message != tt.legacyMessage || diagnostic.Text != tt.text || diagnostic.Primary.Message == "" || len(diagnostic.Secondary) != tt.secondaries {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
		})
	}
}
