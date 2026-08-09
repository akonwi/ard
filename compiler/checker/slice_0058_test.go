package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestSliceBuiltinsTypeCheck(t *testing.T) {
	result := parse.Parse([]byte(`
fn identity(view: Slice<$T>) Slice<$T> { view }
fn next_index() Int { 1 }

fn consume(view: Slice<Int>) Int {
  mut total = 0
  for value in view {
    total = total + value
  }
  total
}

fn main() Bool {
  let values = [10, 20, 30, 40]
  let full: Slice<Int>? = values.slice()
  let tail: Slice<Int>? = values.slice(start: 1)
  let computed: Slice<Int>? = values.slice(start: next_index())
  let head: Slice<Int>? = values.slice(end: 3)
  let middle = values.slice(start: 1, end: 3).expect("bounds")
  let generic: Slice<Int> = identity(middle)
  let nested: Slice<Int>? = middle.slice(start: 1)
  let copied: [Int] = middle.to_list()
  let writable: mut Slice<Int> = mut middle
  let changed: Bool = writable.set(0, 99)
  writable.swap(0, 1)
  changed and (not middle.is_empty()) and middle.size() == 2 and middle.at(0).or(0) == 30 and copied.at(0).or(0) == 20 and generic.size() == 2 and consume(middle) == 129 and full.is_some() and tail.is_some() and computed.is_some() and head.is_some() and nested.is_some()
}
`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}

func TestStringSliceBuiltinTypeChecks(t *testing.T) {
	result := parse.Parse([]byte(`
fn main() Bool {
  let full: Str? = "hello".slice()
  let tail: Str? = "hello".slice(start: 1)
  let head: Str? = "hello".slice(end: 4)
  let middle: Str? = "hello".slice(start: 1, end: 4)
  full.is_some() and tail.is_some() and head.is_some() and middle.is_some()
}
`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
}

func TestSliceBuiltinNameCannotBeRedeclared(t *testing.T) {
	result := parse.Parse([]byte(`struct Slice { value: Int }`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("test.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected Slice redeclaration to be rejected")
	}
}

func TestSliceRejectsLengthChangingAndSortMethods(t *testing.T) {
	for _, source := range []string{
		`let view = [1, 2].slice().expect("bounds")
let writable = mut view
writable.push(3)`,
		`let view = [2, 1].slice().expect("bounds")
let writable = mut view
writable.sort(fn(a, b) { a < b })`,
	} {
		result := parse.Parse([]byte(source), "test.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		c := New("test.ard", result.Program, nil)
		c.Check()
		if !c.HasErrors() {
			t.Fatalf("expected checker error for:\n%s", source)
		}
	}
}
