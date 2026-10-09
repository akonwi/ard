package migrate_test

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/migrate"
	"github.com/akonwi/ard/parse"
)

const prelude = `struct Box {
  value: Int,
}

impl Box {
  fn mut set(value: Int) { self.value = value }
}

trait View {
  fn value() Int
}

impl View for Box {
  fn value() Int { self.value }
}

`

func TestRewritePointerSyntax(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		manual int
		// invalid marks inputs that do not check; their rewrite is not
		// re-checked.
		invalid bool
	}{
		{
			name:  "parameter type",
			input: "fn reset(box: mut Box) { box.value = 0 }",
			want:  "fn reset(box: &mut Box) { box.value = 0 }",
		},
		{
			name:  "nullable and list parameter types",
			input: "fn f(a: mut Box?, b: mut [Int]) {}",
			want:  "fn f(a: &mut Box?, b: &mut [Int]) {}",
		},
		{
			name:  "binding, field, and return types",
			input: "struct Holder {\n  box: mut Box,\n}\nfn make() mut Box {\n  let holder = Holder{box: mut Box{value: 1}}\n  holder.box\n}\nlet box: mut Box = make()",
			want:  "struct Holder {\n  box: &mut Box,\n}\nfn make() &mut Box {\n  let holder = Holder{box: &mut Box{value: 1}}\n  holder.box\n}\nlet box: &mut Box = make()",
		},
		{
			name:  "function type parameter",
			input: "fn apply(f: fn(mut Box) Void, box: mut Box) { f(box) }",
			want:  "fn apply(f: fn(&mut Box) Void, box: &mut Box) { f(box) }",
		},
		{
			name:  "trait types stay",
			input: "fn show(view: mut View) Int { view.value() }",
			want:  "fn show(view: mut View) Int { view.value() }",
		},
		{
			name:    "unresolved referent stays",
			input:   "fn show(view: mut Missing) {}",
			want:    "fn show(view: mut Missing) {}",
			invalid: true,
		},
		{
			name:  "redundant mut on pointer parameter types",
			input: "fn f(a: mut &mut Box, g: fn(mut &mut Box) Void) {}",
			want:  "fn f(a: &mut Box, g: fn(&mut Box) Void) {}",
		},
		{
			name:    "mut on a pointer type outside parameters needs a manual change",
			input:   "struct Holder {\n  box: mut &mut Box,\n}",
			want:    "struct Holder {\n  box: mut &mut Box,\n}",
			manual:  1,
			invalid: true,
		},
		{
			name:  "borrow of mut binding",
			input: "fn reset(box: mut Box) { box.value = 0 }\nmut box = Box{value: 1}\nreset(mut box)",
			want:  "fn reset(box: &mut Box) { box.value = 0 }\nmut box = Box{value: 1}\nreset(&mut box)",
		},
		{
			name:  "borrow of let binding makes it mut",
			input: "fn reset(box: mut Box) { box.value = 0 }\nfn main() {\n  let box = Box{value: 1}\n  reset(mut box)\n  reset(mut box)\n}",
			want:  "fn reset(box: &mut Box) { box.value = 0 }\nfn main() {\n  mut box = Box{value: 1}\n  reset(&mut box)\n  reset(&mut box)\n}",
		},
		{
			name:  "borrow of let field makes the root mut",
			input: "struct Pair {\n  left: Box,\n}\nfn reset(box: mut Box) { box.value = 0 }\nfn main() {\n  let pair = Pair{left: Box{value: 1}}\n  reset(mut pair.left)\n}",
			want:  "struct Pair {\n  left: Box,\n}\nfn reset(box: &mut Box) { box.value = 0 }\nfn main() {\n  mut pair = Pair{left: Box{value: 1}}\n  reset(&mut pair.left)\n}",
		},
		{
			name:   "borrow of module-level let needs a manual change",
			input:  "let shared = Box{value: 1}\nfn get() mut Box { (mut shared) }",
			want:   "let shared = Box{value: 1}\nfn get() &mut Box { (mut shared) }",
			manual: 1,
		},
		{
			name:  "redundant mut on pointer",
			input: "fn reset(box: mut Box) { box.value = 0 }\nfn twice(box: mut Box) {\n  reset(mut box)\n  reset(box)\n}",
			want:  "fn reset(box: &mut Box) { box.value = 0 }\nfn twice(box: &mut Box) {\n  reset(box)\n  reset(box)\n}",
		},
		{
			name:  "fresh values",
			input: "let a = mut Box{value: 1}\nlet b = mut [1, 2]\na.set(2)\nb.push(3)",
			want:  "let a = &mut Box{value: 1}\nlet b = &mut [1, 2]\na.set(2)\nb.push(3)",
		},
		{
			name:  "dereference",
			input: "fn copy(box: mut Box) Box { box.@ }",
			want:  "fn copy(box: &mut Box) Box { box.* }",
		},
		{
			name:  "loose operand is parenthesized",
			input: "let n = mut 1 + 2\nn.* = 4",
			want:  "let n = &mut (1 + 2)\nn.* = 4",
		},
		{
			name:   "trait snapshot needs a manual change",
			input:  "fn snap(view: mut View) View { view.@ }",
			want:   "fn snap(view: mut View) View { view.@ }",
			manual: 1,
		},
		{
			name:   "trait borrow needs a manual change",
			input:  "fn show(view: mut View) Int { view.value() }\nlet view: View = Box{value: 1}\nshow(mut view)",
			want:   "fn show(view: mut View) Int { view.value() }\nlet view: View = Box{value: 1}\nshow(mut view)",
			manual: 1,
		},
		{
			name:   "parameter borrow needs a manual change",
			input:  "fn reset(box: mut Box) { box.value = 0 }\nfn local(box: Box) { reset(mut box) }",
			want:   "fn reset(box: &mut Box) { box.value = 0 }\nfn local(box: Box) { reset(mut box) }",
			manual: 1,
		},
		{
			name:   "borrowing a dereferenced copy needs a manual change",
			input:  "fn reset(box: mut Box) { box.value = 0 }\nfn copy(box: mut Box) { reset(mut box.@) }",
			want:   "fn reset(box: &mut Box) { box.value = 0 }\nfn copy(box: &mut Box) { reset(mut box.@) }",
			manual: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, manual, err := migrate.RewriteSource("main.ard", []byte(prelude+tt.input))
			if err != nil {
				t.Fatalf("rewrite: %v", err)
			}
			if want := prelude + tt.want; string(got) != want {
				t.Fatalf("rewrite mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
			if len(manual) != tt.manual {
				t.Fatalf("manual deprecations = %d, want %d: %v", len(manual), tt.manual, manual)
			}
			if !tt.invalid {
				assertNoErrors(t, got)
			}
		})
	}
}

func TestRewriteIsIdempotent(t *testing.T) {
	source := prelude + "fn reset(box: mut Box) { box.value = 0 }\nfn main() {\n  let box = Box{value: 1}\n  reset(mut box)\n}\n"
	once, _, err := migrate.RewriteSource("main.ard", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	twice, manual, err := migrate.RewriteSource("main.ard", once)
	if err != nil {
		t.Fatal(err)
	}
	if string(once) != string(twice) || len(manual) != 0 {
		t.Fatalf("second rewrite changed the source:\n%s", twice)
	}
}

func assertNoErrors(t *testing.T, source []byte) {
	t.Helper()
	result := parse.Parse(source, "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("rewritten source does not parse: %v\n%s", result.Errors, source)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	var errors []string
	for _, diagnostic := range c.Diagnostics() {
		if diagnostic.Kind == checker.Error {
			errors = append(errors, diagnostic.Message)
		}
	}
	if len(errors) > 0 {
		t.Fatalf("rewritten source has errors: %s\n%s", strings.Join(errors, "; "), source)
	}
}

func TestApplyRejectsPositionsPastTheirLine(t *testing.T) {
	source := []byte("ab\r\ncd\n")
	if _, err := migrate.Apply(source, []checker.TextEdit{{Start: parse.Point{Row: 1, Col: 4}, End: parse.Point{Row: 1, Col: 4}, NewText: "x"}}); err == nil {
		t.Fatal("expected an error for a column past the end of line 1")
	}
	got, err := migrate.Apply(source, []checker.TextEdit{{Start: parse.Point{Row: 1, Col: 3}, End: parse.Point{Row: 1, Col: 3}, NewText: "x"}})
	if err != nil || string(got) != "abx\r\ncd\n" {
		t.Fatalf("insertion at end of line = %q, %v", got, err)
	}
}

func TestApplyRejectsOverlappingEdits(t *testing.T) {
	source := []byte("mut value")
	_, err := migrate.Apply(source, []checker.TextEdit{
		{Start: parse.Point{Row: 1, Col: 1}, End: parse.Point{Row: 1, Col: 5}, NewText: ""},
		{Start: parse.Point{Row: 1, Col: 3}, End: parse.Point{Row: 1, Col: 6}, NewText: "x"},
	})
	if err == nil {
		t.Fatal("expected overlapping edits to fail")
	}
}
