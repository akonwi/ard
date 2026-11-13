package checker_test

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
)

const functionLayoutPrelude = `struct Holder {
  f: fn() $T,
  items: [fn() $T],
}

struct Box {
  value: $V,
}

impl Holder {
  fn first() $T {
    let item = self.items.at(0).expect("empty")
    item()
  }

  fn replace(items: [fn() $T]) Int {
    items.size()
  }

  fn all() [fn() $T] {
    self.items
  }
}

struct Runner {
  run: fn([fn() $T]) Int,
}

fn runner(witness: $T) Runner<$T> {
  Runner{run: fn(fs: [fn() $T]) Int { fs.size() }}
}

fn first(fs: [fn() $T]) $T {
  fs.at(0).expect("empty")()
}

fn wrap(f: fn() $T) [fn() $T] {
  [f]
}

fn named(f: fn() $T) [Str: fn() $T] {
  ["a": f]
}

fn by_name(fs: [Str: fn() $T]) Int {
  fs.size()
}

fn through_pointer(fs: &mut [fn() $T]) Int {
  fs.size()
}

fn unbox(box: Box<fn() $T>) $T {
  box.value()
}

fn each(g: fn([fn() $T]) Int, f: fn() $T) Int {
  g([f])
}

fn fallible(fs: [fn() $T!Str]) Int {
  fs.size()
}

fn failing(fs: [fn() Int!$E]) Int {
  fs.size()
}

fn call(f: fn() $T) $T {
  f()
}

fn forward(fs: [fn() $T]) $T {
  first(fs)
}

fn size(items: [$A]) Int {
  items.size()
}

fn maybe_first(f: (fn() $T)?) Int {
  1
}

fn count(fs: [fn() Int?]) Int {
  fs.size()
}

fn relay(h: Holder<$T>) $T {
  let items = h.all()
  let item = items.at(0).expect("empty")
  h.replace(items)
  first(h.items)
  item()
}

fn hold(f: fn() $T) Holder<$T> {
  Holder{f: f, items: [f]}
}

fn holder() Holder<Int?> {
  hold(fn() Int? { Maybe::new(1) })
}
`

// A list, map, pointer, or generic type argument holding a function typed
// with a type parameter has two Go types: one when it is built in concrete
// code, another when it is built in generic code. The checker reports a use
// that needs the type the value was not built with (#588). Every case here
// fails in the Go compiler without the diagnostic.
func TestUnconvertibleFunctionLayoutIsRejected(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		message string
		label   string
		// declarations are extra top-level declarations. The diagnostic may
		// then be reported inside one of them.
		declarations string
	}{
		{
			name:    "concrete list to a generic parameter",
			body:    "let fs: [fn() Int?] = [fn() Int? { Maybe::new(1) }]\n  first(fs)",
			message: "Cannot use [fn() Int?] as [fn() $T]: Ard does not convert a function held in a list",
			label:   "built as `[fn() Int?]`, used as `[fn() $T]`",
		},
		{
			name:    "list literal to a generic parameter",
			body:    "first([fn() Int? { Maybe::new(1) }])",
			message: "Cannot use [fn() Int?] as [fn() $T]: Ard does not convert a function held in a list",
		},
		{
			name:    "map to a generic parameter",
			body:    "let fs: [Str: fn() Int?] = [\"a\": fn() Int? { Maybe::new(1) }]\n  by_name(fs)",
			message: "Cannot use [Str: fn() Int?] as [Str: fn() $T]: Ard does not convert a function held in a map",
		},
		{
			name:    "pointer to a generic parameter",
			body:    "mut fs: [fn() Int?] = [fn() Int? { Maybe::new(1) }]\n  through_pointer(&mut fs)",
			message: "Ard does not convert a function held in a list",
		},
		{
			name:    "generic type argument",
			body:    "let box = Box{value: fn() Int? { Maybe::new(1) }}\n  unbox(box)",
			message: "Ard does not convert a function held in a type argument",
		},
		{
			name:    "function taking a list",
			body:    "each(fn(fs: [fn() Int?]) Int { fs.size() }, fn() Int? { Maybe::new(1) })",
			message: "Ard does not convert a function held in a list",
		},
		{
			name:    "result return",
			body:    "let fs: [fn() Int!Str] = [fn() Int!Str { Result::ok(1) }]\n  first(fs)",
			message: "Cannot use [fn() Int!Str] as [fn() $T]",
		},
		{
			name:    "tuple return",
			body:    "let fs: [fn() (Int, Int)] = [fn() (Int, Int) { (1, 2) }]\n  first(fs)",
			message: "Cannot use [fn() (Int, Int)] as [fn() $T]",
		},
		{
			name:    "void return",
			body:    "let fs: [fn() Void] = [fn() Void { () }]\n  first(fs)",
			message: "as [fn() $T]: Ard does not convert a function held in a list",
		},
		{
			name:    "tuple success value",
			body:    "let fs: [fn() (Int, Int)!Str] = [fn() (Int, Int)!Str { Result::ok((1, 2)) }]\n  fallible(fs)",
			message: "Ard does not convert a function held in a list",
		},
		{
			name:    "generic error type",
			body:    "let fs: [fn() Int!Str] = [fn() Int!Str { Result::ok(1) }]\n  failing(fs)",
			message: "Ard does not convert a function held in a list",
		},
		{
			name:    "concrete list in a generic struct literal",
			body:    "let fs: [fn() Int?] = [fn() Int? { Maybe::new(1) }]\n  let made = Holder<Int?>{f: fn() Int? { Maybe::new(1) }, items: fs}",
			message: "Cannot use [fn() Int?] as [fn() $T]: Ard does not convert a function held in a list",
		},
		{
			name:    "concrete list to a generic method",
			body:    "let fs: [fn() Int?] = [fn() Int? { Maybe::new(1) }]\n  holder().replace(fs)",
			message: "Cannot use [fn() Int?] as [fn() $T]: Ard does not convert a function held in a list",
		},
		{
			name:    "concrete list to a function held in a generic field",
			body:    "let fs: [fn() Int?] = [fn() Int? { Maybe::new(1) }]\n  let one: Int? = Maybe::new(1)\n  runner(one).run(fs)",
			message: "Cannot use [fn() Int?] as [fn() $T]: Ard does not convert a function held in a list",
		},
		{
			name:    "concrete list to a function read from a generic field",
			body:    "let one: Int? = Maybe::new(1)\n  let run = runner(one).run\n  run([fn() Int? { Maybe::new(1) }])",
			message: "Cannot use [fn() Int?] as [fn() $T]: Ard does not convert a function held in a list",
		},
		{
			name:    "element of a generic map used in a loop",
			body:    "for name, item in named(fn() Int? { Maybe::new(1) }) {\n    item()\n  }",
			message: "Cannot use [Str: fn() $T] as [Str: fn() Int?]: Ard does not convert a function held in a map",
		},
		{
			name:    "concrete list assigned to a generic field",
			body:    "mut target = holder()\n  let fs: [fn() Int?] = [fn() Int? { Maybe::new(1) }]\n  target.items = fs",
			message: "Cannot use [fn() Int?] as [fn() $T]: Ard does not convert a function held in a list",
		},
		{
			name:    "concrete list assigned to a generic-built binding",
			body:    "mut fs = holder().items\n  fs = [fn() Int? { Maybe::new(1) }]",
			message: "Cannot use [fn() Int?] as [fn() $T]: Ard does not convert a function held in a list",
		},
		{
			name:    "element read from a generic field",
			body:    "let item = holder().items.at(0)",
			message: "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
			label:   "built as `[fn() $T]`, used as `[fn() Int?]`",
		},
		{
			name:    "loop over a generic field",
			body:    "for item in holder().items {\n    item()\n  }",
			message: "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
		},
		{
			name:    "push to a generic field",
			body:    "mut target = holder()\n  let shared = &mut target\n  shared.items.push(fn() Int? { Maybe::new(1) })",
			message: "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
		},
		{
			name:    "element read from a generic method result",
			body:    "let item = holder().all().at(0)",
			message: "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
		},
		{
			name:    "element read from a generic-built binding",
			body:    "let fs = wrap(fn() Int? { Maybe::new(1) })\n  let item = fs.at(0)",
			message: "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
		},
		{
			name:    "map read from a generic result",
			body:    "let item = named(fn() Int? { Maybe::new(1) }).get(\"a\")",
			message: "Cannot use [Str: fn() $T] as [Str: fn() Int?]: Ard does not convert a function held in a map",
		},
		{
			name:    "generic-built list to a concrete parameter",
			body:    "count(holder().items)",
			message: "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
		},
		{
			name:         "generic-built list returned from a concrete function",
			body:         "held(holder())",
			message:      "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
			declarations: "fn held(h: Holder<Int?>) [fn() Int?] {\n  h.items\n}\n",
		},
		{
			name:    "generic-built list returned from a closure",
			body:    "let items = fn() [fn() Int?] { holder().items }",
			message: "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
		},
		{
			name:    "generic field read through a maybe",
			body:    "let some: Holder<Int?>? = Maybe::new(holder())\n  let n = match some.items {\n    items => items.size(),\n    _ => 0,\n  }",
			message: "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
		},
		{
			name:         "generic field read through try",
			body:         "project(Maybe::new(holder()))",
			message:      "Cannot use [fn() $T] as [fn() Int?]: Ard does not convert a function held in a list",
			declarations: "fn project(h: Holder<Int?>?) Int? {\n  let items = try h.items\n  Maybe::new(items.size())\n}\n",
		},
		{
			name:         "generic list to a parameter with another layout",
			body:         "opt([fn() Int? { Maybe::new(1) }])",
			message:      "Cannot use [fn() $T?] as [fn() $T]: Ard does not convert a function held in a list",
			declarations: "fn opt(fs: [fn() $T?]) $T? {\n  first(fs)\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := functionLayoutPrelude + tt.declarations + "\nfn main() {\n  " + tt.body + "\n}\n"
			diagnostics := checkSource(t, source)
			if len(diagnostics) != 1 {
				t.Fatalf("diagnostics = %v, want one", diagnostics)
			}
			diagnostic := diagnostics[0]
			if diagnostic.Code != checker.DiagnosticCodeUnconvertibleFunctionLayout {
				t.Fatalf("code = %q, want %q (%v)", diagnostic.Code, checker.DiagnosticCodeUnconvertibleFunctionLayout, diagnostic)
			}
			if !strings.Contains(diagnostic.Message, tt.message) {
				t.Fatalf("message = %q, want it to contain %q", diagnostic.Message, tt.message)
			}
			if tt.label != "" && diagnostic.Primary.Message != tt.label {
				t.Fatalf("label = %q, want %q", diagnostic.Primary.Message, tt.label)
			}
			if row, firstRow := diagnostic.Primary.Span.Location.Start.Row, strings.Count(functionLayoutPrelude, "\n")+1; row < firstRow {
				t.Fatalf("diagnostic at row %d, want it after the shared declarations (row %d or later)", row, firstRow)
			}
		})
	}
}

// Crossings the backend converts, types whose Go layout does not depend on
// the type argument, and values that stay in the layout they were built with
// are accepted. Every case here compiles and runs.
func TestConvertibleFunctionLayoutIsAccepted(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "single function argument", body: "call(fn() Int? { Maybe::new(1) })"},
		{name: "single function field", body: "let f = holder().f\n  f()"},
		{name: "call through a field", body: "holder().f()"},
		{name: "method reading its own field", body: "holder().first()"},
		{name: "maybe of a function", body: "let f: (fn() Int?)? = Maybe::new(fn() Int? { Maybe::new(1) })\n  maybe_first(f)"},
		{name: "plain return type", body: "let fs: [fn() Int] = [fn() Int { 1 }]\n  first(fs)"},
		{name: "function as the type argument", body: "let fs: [fn() Int?] = [fn() Int? { Maybe::new(1) }]\n  size(fs)"},
		{name: "same result layout", body: "let fs: [fn() Int!Str] = [fn() Int!Str { Result::ok(1) }]\n  fallible(fs)"},
		{name: "concrete list in concrete code", body: "let fs: [fn() Int?] = [fn() Int? { Maybe::new(1) }]\n  count(fs)\n  let item = fs.at(0)"},
		{name: "generic field measured", body: "let n = holder().items.size()"},
		{name: "generic field passed back to generic code", body: "first(holder().items)"},
		{name: "generic field bound and passed back", body: "let items = holder().items\n  first(items)\n  let n = items.size()"},
		{name: "generic field bound with an annotation", body: "let items: [fn() Int?] = holder().items\n  first(items)"},
		{name: "generic field stored in a generic struct literal", body: "let from = holder()\n  let made = Holder<Int?>{f: from.f, items: from.items}"},
		{name: "generic field assigned to a generic field", body: "mut target = holder()\n  target.items = holder().items"},
		{name: "generic field passed to a generic method", body: "holder().replace(holder().items)"},
		{name: "generic result passed to generic code", body: "first(wrap(fn() Int? { Maybe::new(1) }))"},
		{name: "generic method result passed to generic code", body: "first(holder().all())"},
		{name: "generic result through a forwarding function", body: "forward(wrap(fn() Int? { Maybe::new(1) }))"},
		{name: "generic-built list to a function held in a generic field", body: "let one: Int? = Maybe::new(1)\n  let made = runner(one)\n  let fs = wrap(fn() Int? { Maybe::new(1) })\n  made.run(fs)"},
		{name: "generic-built list to a function read from a generic field", body: "let one: Int? = Maybe::new(1)\n  let run = runner(one).run\n  let fs = wrap(fn() Int? { Maybe::new(1) })\n  run(fs)"},
		{name: "loop over a generic field that ignores its elements", body: "mut n = 0\n  for item in holder().items {\n    n = n + 1\n  }"},
		{name: "loop over a generic map that uses only its keys", body: "mut names: [Str] = []\n  for name, item in named(fn() Int? { Maybe::new(1) }) {\n    names = [name]\n  }"},
		{name: "generic field through a pointer", body: "mut target = holder()\n  let shared = &mut target\n  first(shared.items)"},
		{name: "generic-built pointer passed back", body: "mut fs = wrap(fn() Int? { Maybe::new(1) })\n  through_pointer(&mut fs)"},
		{name: "generic method inside a generic function", body: "relay(holder())"},
		{name: "generic map measured", body: "let n = by_name(named(fn() Int? { Maybe::new(1) }))"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := functionLayoutPrelude + "\nfn main() {\n  " + tt.body + "\n}\n"
			if diagnostics := checkSource(t, source); len(diagnostics) != 0 {
				t.Fatalf("diagnostics = %v, want none", diagnostics)
			}
		})
	}
}

// The diagnostic names the two ways to write the program so both sides agree:
// spell the return shape in the generic declaration, or hold a struct with
// the function as a field.
func TestUnconvertibleFunctionLayoutNamesTheFixes(t *testing.T) {
	const wrap = "To keep `$T` open to any type, wrap the function in a struct, such as `struct Handler { run: fn() $T }`, and hold that struct instead."
	tests := []struct {
		name string
		body string
		text string
	}{
		{
			name: "maybe",
			body: "first([fn() Int? { Maybe::new(1) }])",
			text: "`fn() Int?` and `fn() $T` return their results differently in Go, and Ard converts a single function between them but not one held in a list. In the generic declaration, write the return type as `$T?` instead of `$T`. " + wrap,
		},
		{
			name: "built in generic code",
			body: "let item = holder().items.at(0)",
			text: "`fn() $T` and `fn() Int?` return their results differently in Go, and Ard converts a single function between them but not one held in a list. In the generic declaration, write the return type as `$T?` instead of `$T`. " + wrap,
		},
		{
			name: "result",
			body: "first([fn() Int!Str { Result::ok(1) }])",
			text: "In the generic declaration, write the return type as `$T!Str` instead of `$T`. " + wrap,
		},
		{
			name: "void",
			body: "first([fn() Void { () }])",
			text: "In the generic declaration, write the return type as `Void` instead of `$T`. " + wrap,
		},
		{
			name: "tuple",
			body: "first([fn() (Int, Int) { (1, 2) }])",
			text: "but not one held in a list. Declare both with the same return type, or wrap the function in a struct and hold that struct instead.",
		},
		{
			name: "map",
			body: "by_name([\"a\": fn() Int? { Maybe::new(1) }])",
			text: "but not one held in a map. In the generic declaration, write the return type as `$T?` instead of `$T`. " + wrap,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostics := checkSource(t, functionLayoutPrelude+"\nfn main() {\n  "+tt.body+"\n}\n")
			if len(diagnostics) != 1 {
				t.Fatalf("diagnostics = %v, want one", diagnostics)
			}
			diagnostic := diagnostics[0]
			if diagnostic.Title != "Generic function type cannot be converted" {
				t.Fatalf("title = %q", diagnostic.Title)
			}
			if !strings.HasSuffix(diagnostic.Text, tt.text) {
				t.Fatalf("text = %q, want it to end with %q", diagnostic.Text, tt.text)
			}
		})
	}
}
