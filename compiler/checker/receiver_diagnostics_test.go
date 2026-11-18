package checker_test

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

const receiverDiagnosticsPrelude = `use go:io
use go:strings

struct User {
  name: Str,
}

struct Counter {
  n: Int,
  label: Str,
}

impl Counter {
  fn mut bump() {
    self.n = self.n + 1
  }
}

enum Color {
  red,
  green,
}

type Shape = User | Color

type Person = User

fn make_user() User {
  User{name: "a"}
}

fn make_counter() Counter {
  Counter{n: 1, label: "c"}
}
`

func checkReceiverDiagnostics(t *testing.T, body string) []checker.Diagnostic {
	t.Helper()
	source := receiverDiagnosticsPrelude + "\n" + body + "\n"
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("test.ard", checker.WrapStatementRuns(result.Program), nil, checker.CheckOptions{GoResolver: standardLibraryGoResolver(t)})
	c.Check()
	return c.Diagnostics()
}

// Member diagnostics name the type whose members were searched. The receiver
// expression itself is never rendered: most checked expressions have no
// readable form and used to leak as raw Go struct dumps. Ard references are
// transparent to member access, so they name the referent; Go pointers keep
// the pointer because their method set differs.
func TestUndefinedMemberDiagnosticsNameTheReceiverType(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		receiver string
		member   string
		kind     string
	}{
		{name: "Int variable", body: "fn main() {\n  let i = 1\n  i.nope()\n}", receiver: "Int"},
		{name: "Int literal", body: "fn main() {\n  1.nope()\n}", receiver: "Int"},
		{name: "Str literal", body: "fn main() {\n  \"s\".nope()\n}", receiver: "Str"},
		{name: "Bool literal", body: "fn main() {\n  true.nope()\n}", receiver: "Bool"},
		{name: "Float literal", body: "fn main() {\n  1.5.nope()\n}", receiver: "Float64"},
		{name: "template string", body: "fn main() {\n  let i = 1\n  \"a{i}b\".nope()\n}", receiver: "Str"},
		{name: "binary expression", body: "fn main() {\n  (1 + 2).nope()\n}", receiver: "Int"},
		{name: "struct variable", body: "fn main() {\n  let u = User{name: \"a\"}\n  u.nope()\n}", receiver: "User"},
		{name: "struct literal", body: "fn main() {\n  (User{name: \"b\"}).nope()\n}", receiver: "User"},
		{name: "function call", body: "fn main() {\n  make_user().nope()\n}", receiver: "User"},
		{name: "struct field", body: "fn main() {\n  let u = User{name: \"a\"}\n  u.name.nope()\n}", receiver: "Str"},
		{name: "method call", body: "fn main() {\n  let u = User{name: \"a\"}\n  u.name.size().nope()\n}", receiver: "Int"},
		{name: "enum variant", body: "fn main() {\n  Color::red.nope()\n}", receiver: "Color"},
		{name: "union", body: "fn main() {\n  let s: Shape = User{name: \"a\"}\n  s.nope()\n}", receiver: "Shape"},
		{name: "type alias", body: "fn main() {\n  let p: Person = User{name: \"a\"}\n  p.nope()\n}", receiver: "User"},
		{name: "Maybe", body: "fn main() {\n  let m: Int? = Maybe::new()\n  m.nope()\n}", receiver: "Int?"},
		{name: "Maybe from list access", body: "fn main() {\n  let l = [1, 2]\n  l.at(0).nope()\n}", receiver: "Int?"},
		{name: "Result", body: "fn main() {\n  let r: Int!Str = Result::ok(1)\n  r.nope()\n}", receiver: "Int!Str"},
		{name: "list variable", body: "fn main() {\n  let l = [1, 2]\n  l.nope()\n}", receiver: "[Int]"},
		{name: "list literal", body: "fn main() {\n  [1, 2].nope()\n}", receiver: "[Int]"},
		{name: "map literal", body: "fn main() {\n  [\"a\": 1].nope()\n}", receiver: "[Str: Int]"},
		{name: "tuple literal", body: "fn main() {\n  (1, \"a\").nope()\n}", receiver: "(Int, Str)"},
		{name: "function value", body: "fn main() {\n  let f = fn(x: Int) Int { x }\n  f.nope()\n}", receiver: "fn(Int) Int"},
		{name: "type variable", body: "fn id(x: $T) $T {\n  x.nope()\n  x\n}", receiver: "$T"},
		{name: "read-only reference", body: "fn main() {\n  let u = User{name: \"a\"}\n  let r = &u\n  r.nope()\n}", receiver: "User"},
		{name: "writable reference", body: "fn main() {\n  mut u = User{name: \"a\"}\n  let r = &mut u\n  r.nope()\n}", receiver: "User"},
		{name: "Go struct", body: "fn main() {\n  let b = strings::Builder{}\n  b.nope()\n}", receiver: "strings::Builder"},
		{name: "Go pointer from call", body: "fn main() {\n  strings::NewReader(\"x\").nope()\n}", receiver: "&mut strings::Reader"},
		{name: "Go interface", body: "fn read(r: io::Reader) {\n  r.nope()\n}", receiver: "io::Reader"},
		{name: "field on struct literal", body: "fn main() {\n  (User{name: \"b\"}).missing\n}", receiver: "User", member: "missing", kind: "field"},
		{name: "field on tuple literal", body: "fn main() {\n  (1, 2).missing\n}", receiver: "(Int, Int)", member: "missing", kind: "field"},
		{name: "field on list literal", body: "fn main() {\n  [1].missing\n}", receiver: "[Int]", member: "missing", kind: "field"},
		{name: "field on function call", body: "fn main() {\n  make_user().missing\n}", receiver: "User", member: "missing", kind: "field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			member, kind := tt.member, tt.kind
			if member == "" {
				member = "nope"
			}
			if kind == "" {
				kind = "method"
			}
			diagnostic := requireDiagnosticCode(t, checkReceiverDiagnostics(t, tt.body), checker.DiagnosticCodeUndefinedMember)
			wantMessage := "Undefined: " + tt.receiver + "." + member
			wantLabel := "`" + member + "` is not defined for `" + tt.receiver + "`"
			if diagnostic.Message != wantMessage || diagnostic.Title != "Undefined "+kind || diagnostic.Primary.Message != wantLabel {
				t.Fatalf("got message/title/label %q / %q / %q\nwant %q / %q / %q",
					diagnostic.Message, diagnostic.Title, diagnostic.Primary.Message,
					wantMessage, "Undefined "+kind, wantLabel)
			}
		})
	}
}

func TestReceiverDiagnosticsNeverRenderCheckedExpressions(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		code    checker.DiagnosticCode
		message string
		label   string
	}{
		{
			name:    "non-function field called on a temporary",
			body:    "fn main() {\n  make_counter().label()\n}",
			code:    checker.DiagnosticCodeNotCallable,
			message: "Counter.label is not a function",
			label:   "`Counter.label` is not a function",
		},
		{
			name:    "mutating method on a temporary",
			body:    "fn main() {\n  make_counter().bump()\n}",
			code:    checker.DiagnosticCodeValueInteriorMutation,
			message: "Cannot call mutating method 'Counter.bump': receiver is not a writable pointer",
			label:   "`.bump()` requires a `&mut` receiver",
		},
		{
			name:    "mutating method on a value binding",
			body:    "fn main() {\n  let c = make_counter()\n  c.bump()\n}",
			code:    checker.DiagnosticCodeValueInteriorMutation,
			message: "Cannot call mutating method 'Counter.bump': receiver is not a writable pointer",
			label:   "`.bump()` requires a `&mut` receiver",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostic := requireDiagnosticCode(t, checkReceiverDiagnostics(t, tt.body), tt.code)
			if diagnostic.Message != tt.message || diagnostic.Primary.Message != tt.label {
				t.Fatalf("got message/label %q / %q\nwant %q / %q", diagnostic.Message, diagnostic.Primary.Message, tt.message, tt.label)
			}
		})
	}
}

// A broad sweep over receiver forms: no diagnostic text may contain a Go
// value dump (`&{...}`, pointer addresses, or fmt's `%!verb` errors).
func TestReceiverDiagnosticsContainNoGoValueDumps(t *testing.T) {
	body := `fn main() {
  (User{name: "b"}).a()
  make_user().b()
  [1, 2].c()
  ["a": 1].d()
  (1, "a").e()
  [1].at(0).f()
  (1 + 2).g()
  "x{1}y".h()
  "s".size().i()
  strings::NewReader("x").j()
  make_counter().label()
  make_counter().bump()
  [make_counter()].at(0).bump()
  (1, 2).k
  [1].l
  make_user().m
}`
	for _, diagnostic := range checkReceiverDiagnostics(t, body) {
		texts := []string{diagnostic.Message, diagnostic.Title, diagnostic.Text, diagnostic.Primary.Message}
		for _, secondary := range diagnostic.Secondary {
			texts = append(texts, secondary.Message)
		}
		for _, text := range texts {
			if strings.Contains(text, "&{") || strings.Contains(text, "%!") || strings.Contains(text, "0x") || strings.Contains(text, "TemplateStr") {
				t.Errorf("diagnostic text leaks a Go value: %q", text)
			}
		}
	}
}
