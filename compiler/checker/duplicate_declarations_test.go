package checker_test

import (
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// duplicateDeclarations returns the duplicate_declaration diagnostics of
// source and fails on any other error, so a test also proves the duplicate
// does not cascade into unrelated diagnostics.
func duplicateDeclarations(t *testing.T, source string) []checker.Diagnostic {
	t.Helper()
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	duplicates := []checker.Diagnostic{}
	for _, diagnostic := range c.Diagnostics() {
		if diagnostic.Code == checker.DiagnosticCodeDuplicateDeclaration {
			duplicates = append(duplicates, diagnostic)
			continue
		}
		if diagnostic.Kind == checker.Error {
			t.Fatalf("unexpected error: %s at %v", diagnostic.Message, diagnostic.Location().Start)
		}
	}
	return duplicates
}

// Ard has one top-level namespace for values, functions, and types, and no
// overloading: a second declaration of a name is an error whatever its kind
// (#624).
func TestDuplicateTopLevelDeclarations(t *testing.T) {
	type span struct{ row, col int }
	tests := []struct {
		name      string
		source    string
		duplicate string
		// primary is the start of the second declaration's name, original
		// the start of the first's.
		primary, original span
	}{
		{
			name:      "functions with the same signature",
			source:    "fn f() Int { 1 }\nfn f() Int { 2 }\n",
			duplicate: "f",
			primary:   span{2, 4}, original: span{1, 4},
		},
		{
			name:      "functions with different signatures",
			source:    "fn f(a: Int) Int { a }\nfn f(a: Str) Str { a }\n",
			duplicate: "f",
			primary:   span{2, 4}, original: span{1, 4},
		},
		{
			name:      "private and public functions",
			source:    "fn f() Int { 1 }\nprivate fn f() Int { 2 }\n",
			duplicate: "f",
			primary:   span{2, 12}, original: span{1, 4},
		},
		{
			name:      "test functions",
			source:    "test fn t() Void!Str { Result::ok(()) }\ntest fn t() Void!Str { Result::ok(()) }\n",
			duplicate: "t",
			primary:   span{2, 9}, original: span{1, 9},
		},
		{
			name:      "let bindings",
			source:    "let v = 1\nlet v = 2\n",
			duplicate: "v",
			primary:   span{2, 5}, original: span{1, 5},
		},
		{
			name:      "mut and let bindings",
			source:    "mut v = 1\nlet v = 2\n",
			duplicate: "v",
			primary:   span{2, 5}, original: span{1, 5},
		},
		{
			name:      "function then binding",
			source:    "fn v() Int { 1 }\nlet v = 2\n",
			duplicate: "v",
			primary:   span{2, 5}, original: span{1, 4},
		},
		{
			name:      "struct then function",
			source:    "struct A {}\nfn A() Int { 1 }\n",
			duplicate: "A",
			primary:   span{2, 4}, original: span{1, 8},
		},
		{
			name:      "function then struct",
			source:    "fn A() Int { 1 }\nstruct A {}\n",
			duplicate: "A",
			primary:   span{2, 8}, original: span{1, 4},
		},
		{
			name:      "binding then enum",
			source:    "let E = 1\nenum E { a }\n",
			duplicate: "E",
			primary:   span{2, 6}, original: span{1, 5},
		},
		{
			name:      "union then function",
			source:    "type U = Int | Str\nfn U() Int { 1 }\n",
			duplicate: "U",
			primary:   span{2, 4}, original: span{1, 6},
		},
		{
			name:      "trait then binding",
			source:    "trait T {\n  fn a() Int\n}\nlet T = 1\n",
			duplicate: "T",
			primary:   span{4, 5}, original: span{1, 7},
		},
		{
			name:      "static functions",
			source:    "struct P { x: Int }\nfn P::new() P { P{x: 1} }\nfn P::new() P { P{x: 2} }\n",
			duplicate: "P::new",
			primary:   span{3, 4}, original: span{2, 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duplicates := duplicateDeclarations(t, tt.source)
			if len(duplicates) != 1 {
				t.Fatalf("duplicate declarations = %#v, want one", duplicates)
			}
			diagnostic := duplicates[0]
			if diagnostic.Message != "Duplicate declaration: "+tt.duplicate {
				t.Fatalf("message = %q", diagnostic.Message)
			}
			if got := diagnostic.Primary.Span.Location.Start; got != (parse.Point{Row: tt.primary.row, Col: tt.primary.col}) {
				t.Fatalf("primary start = %v, want %v", got, tt.primary)
			}
			if len(diagnostic.Secondary) != 1 {
				t.Fatalf("secondary = %#v, want the first declaration", diagnostic.Secondary)
			}
			if got := diagnostic.Secondary[0].Span.Location.Start; got != (parse.Point{Row: tt.original.row, Col: tt.original.col}) {
				t.Fatalf("original start = %v, want %v", got, tt.original)
			}
		})
	}
}

func TestEachRepeatedTopLevelDeclarationPointsBackToTheFirst(t *testing.T) {
	duplicates := duplicateDeclarations(t, "fn f() Int { 1 }\nlet f = 2\nstruct f {}\n")
	if len(duplicates) != 2 {
		t.Fatalf("duplicate declarations = %#v, want two", duplicates)
	}
	for i, diagnostic := range duplicates {
		if diagnostic.Primary.Span.Location.Start.Row != i+2 {
			t.Fatalf("duplicate %d primary = %v", i, diagnostic.Primary.Span.Location)
		}
		if diagnostic.Secondary[0].Span.Location.Start.Row != 1 {
			t.Fatalf("duplicate %d original = %v", i, diagnostic.Secondary[0].Span.Location)
		}
	}
}

// The first declaration keeps the name: references bind to it, and the
// duplicate is not checked, so it adds no diagnostics of its own.
func TestFirstTopLevelDeclarationKeepsTheName(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "function",
			source: "fn f(a: Int) Int { a }\nfn f(a: Str) Str { a }\nfn main() {\n  let x: Int = f(1)\n}\n",
		},
		{
			name:   "binding",
			source: "let v = 1\nlet v = \"two\"\nfn main() {\n  let x: Int = v\n}\n",
		},
		{
			name:   "duplicate body is not checked",
			source: "fn f() Int { 1 }\nfn f() Int { \"two\" }\n",
		},
		{
			name:   "struct before function",
			source: "struct A { x: Int }\nfn A() Str { \"a\" }\nfn main() {\n  let a = A{x: 1}\n}\n",
		},
		{
			name:   "duplicate alias is not validated",
			source: "struct G { value: $T }\ntype A = Int\ntype A = G\n",
		},
		{
			name:   "duplicate struct is not checked for recursion",
			source: "struct S { x: Int }\nstruct S { next: S }\n",
		},
		{
			name:   "duplicate struct is not checked for map keys",
			source: "struct S { x: Int }\nstruct S { m: [[Int]:Int] }\n",
		},
		{
			name:   "duplicate generic struct is not checked for cycles",
			source: "struct S { value: $T }\nstruct S { value: $T, next: S<[$T]>? }\n",
		},
		{
			name:   "function before struct",
			source: "fn A() Int { 1 }\nstruct A { x: Int }\nfn main() {\n  let a: Int = A()\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if duplicates := duplicateDeclarations(t, tt.source); len(duplicates) != 1 {
				t.Fatalf("duplicate declarations = %#v, want one", duplicates)
			}
		})
	}
}

// Same-named declarations for different platforms are siblings, not
// duplicates (ADR 0082). Selection keeps one per name, so the duplicate pass
// never sees the others.
func TestPlatformSiblingsAreNotDuplicateDeclarations(t *testing.T) {
	source := `fn value() Int { 1 }

#platform(unix)
fn value() Int { 2 }

let limit: Int = 1

#platform(windows)
let limit: Int = 2

fn main() {
  let v = value() + limit
}
`
	for _, target := range []struct {
		name string
		c    *checker.Checker
	}{
		{"linux", checkForPlatform(t, source, linuxAMD64)},
		{"windows", checkForPlatform(t, source, windowsAMD64)},
	} {
		if len(target.c.Diagnostics()) != 0 {
			t.Fatalf("%s diagnostics:\n%s", target.name, diagnosticSummary(target.c))
		}
	}
}

// Two fallbacks in a platform group are already a platform_sibling error;
// the duplicate pass must not report the same declaration again.
func TestSecondPlatformFallbackIsNotAlsoADuplicateDeclaration(t *testing.T) {
	source := `fn value() Int { 1 }

fn value() Int { 2 }

#platform(windows)
fn value() Int { 3 }

fn main() {
  let v = value()
}
`
	c := checkForPlatform(t, source, linuxAMD64)
	if len(c.Diagnostics()) != 1 || c.Diagnostics()[0].Code != checker.DiagnosticCodePlatformSibling {
		t.Fatalf("diagnostics:\n%s", diagnosticSummary(c))
	}
}

// A duplicate must not replace the first declaration in later passes: the
// first struct's instantiation cycle is still reported.
func TestDuplicateStructDoesNotHideTheFirstDeclarationsErrors(t *testing.T) {
	source := "struct S { value: $T, next: S<[$T]>? }\nstruct S { value: $T }\n"
	result := parse.Parse([]byte(source), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	codes := map[checker.DiagnosticCode]int{}
	for _, diagnostic := range c.Diagnostics() {
		codes[diagnostic.Code]++
	}
	if codes[checker.DiagnosticCodeDuplicateDeclaration] != 1 || codes[checker.DiagnosticCodeGenericInstantiationCycle] != 1 || len(c.Diagnostics()) != 2 {
		t.Fatalf("diagnostics = %#v", c.Diagnostics())
	}
}
