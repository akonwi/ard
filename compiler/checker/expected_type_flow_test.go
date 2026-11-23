package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/compiler/checker"
)

// A function's declared return type types a list or map literal that is the
// body's final expression, as a `let` annotation does.
func TestReturnTypeTypesFinalCollectionLiteral(t *testing.T) {
	run(t, []test{
		{
			name: "list of distinct Go interface implementers",
			input: `use go:bytes
use go:io
use go:strings

fn readers() [io::Reader] {
  [strings::NewReader("a"), bytes::NewBufferString("b")]
}`,
		},
		{
			name: "list of union members",
			input: `type Val = Int | Str

fn values() [Val] {
  [1, "two"]
}`,
		},
		{
			name: "map of union members",
			input: `type Val = Int | Str

fn values() [Str: Val] {
  ["a": 1, "b": "two"]
}`,
		},
		{
			name: "list of tuples of union members",
			input: `type Val = Int | Str

fn pairs() [(Val, Val)] {
  [(1, 2), ("x", 3)]
}`,
		},
		{
			name: "element outside the union is still rejected",
			input: `type Val = Int | Str

fn values() [Val] {
  [1, true]
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Val, got Bool"},
			},
		},
	})
}

// if/else branches producing different members of the expected type unify to
// that type, as match arms do.
func TestIfBranchesUnifyToExpectedType(t *testing.T) {
	run(t, []test{
		{
			name: "branches return different union members",
			input: `struct Square { size: Int }
struct Circle { radius: Int }
type Shape = Square | Circle

fn pick(square: Bool) Shape {
  if square {
    Square{size: 1}
  } else {
    Circle{radius: 2}
  }
}`,
		},
		{
			name: "else-if chain returns union members",
			input: `type Val = Int | Str

fn describe(n: Int) Val {
  if n == 0 {
    0
  } else if n == 1 {
    "one"
  } else {
    n
  }
}`,
		},
		{
			name: "branches return different trait implementers",
			input: `trait Named {
  fn name() Str
}

struct A {}
impl Named for A {
  fn name() Str {
    "a"
  }
}

struct B {}
impl Named for B {
  fn name() Str {
    "b"
  }
}

fn pick(a: Bool) Named {
  if a {
    A{}
  } else {
    B{}
  }
}`,
		},
		{
			name: "branch outside the union is still rejected",
			input: `type Val = Int | Str

fn pick(flag: Bool) Val {
  if flag {
    1
  } else {
    true
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "All branches must have the same result type"},
			},
		},
	})
}

// A tuple literal argument is typed by its parameter, as list and map
// literal arguments are.
func TestParameterTypesTupleLiteralArgument(t *testing.T) {
	run(t, []test{
		{
			name: "function argument",
			input: `type Shape = Int | Str

fn take(pair: (Shape, Shape)) Int {
  1
}

take((1, 2))
take((1, "x"))`,
		},
		{
			name: "nullable parameter",
			input: `type Shape = Int | Str

fn take(pair: (Shape, Shape)?) Int {
  1
}

take((1, "x"))`,
		},
		{
			name: "method argument",
			input: `type Shape = Int | Str

struct Holder { pair: (Shape, Shape) }

impl Holder {
  fn put(pair: (Shape, Shape)) Int {
    1
  }
}

let holder = Holder{pair: (1, 2)}
holder.put((3, "y"))`,
		},
		{
			name: "element outside the union is still rejected",
			input: `type Shape = Int | Str

fn take(pair: (Shape, Shape)) Int {
  1
}

take((1, true))`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Shape, got Bool"},
			},
		},
	})
}
