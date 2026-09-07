package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func checkNestedNamedFunctionSource(t *testing.T, source string) *Checker {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	return checker
}

func TestNestedNamedFunctionKeepsCanonicalLocalDeclarationIdentity(t *testing.T) {
	checker := checkNestedNamedFunctionSource(t, `
		fn outer(value: Int) Int {
			fn inner() Int { value }
			let callback = inner
			let _ = callback
			inner()
		}
	`)
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	outer, ok := checker.program.Statements[0].Expr.(*FunctionDef)
	if !ok {
		t.Fatalf("top-level declaration = %T, want FunctionDef", checker.program.Statements[0].Expr)
	}
	if outer.LocalNamed {
		t.Fatal("top-level function marked as a local named declaration")
	}
	inner, ok := outer.Body.Stmts[0].Expr.(*FunctionDef)
	if !ok {
		t.Fatalf("local declaration = %T, want FunctionDef", outer.Body.Stmts[0].Expr)
	}
	if !inner.LocalNamed {
		t.Fatal("nested function is not marked as a local named declaration")
	}
	binding, ok := outer.Body.Stmts[1].Stmt.(*VariableDef)
	if !ok {
		t.Fatalf("function reference binding = %T, want VariableDef", outer.Body.Stmts[1].Stmt)
	}
	reference, ok := binding.Value.(*Variable)
	if !ok || reference.Declaration() != inner {
		t.Fatalf("function reference declaration = %p, want local declaration %p", reference.Declaration(), inner)
	}
	call, ok := outer.Body.Stmts[3].Expr.(*FunctionCall)
	if !ok || call.Declaration() != inner {
		t.Fatalf("function call declaration = %p, want local declaration %p", call.Declaration(), inner)
	}
}

func TestNestedNamedFunctionCannotUseGenericSignature(t *testing.T) {
	tests := []struct {
		name     string
		function string
		source   string
	}{
		{
			name:     "independent generic",
			function: "identity",
			source: `
				fn outer() Bool {
					fn identity(value: $T) $T { value }
					identity(1) == 1
				}
			`,
		},
		{
			name:     "enclosing declaration generic",
			function: "read",
			source: `
				fn outer(value: $T) $T {
					fn read() $T { value }
					read()
				}
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := checkNestedNamedFunctionSource(t, test.source)
			diagnostics := checker.Diagnostics()
			if len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticCodeUnsupportedLocalGeneric {
				t.Fatalf("checker diagnostics = %v, want one generic local function diagnostic", diagnostics)
			}
			want := "generic local function " + test.function + " is not supported"
			if diagnostics[0].Message != want {
				t.Fatalf("diagnostic = %q, want %q", diagnostics[0].Message, want)
			}
		})
	}
}

func TestNestedStaticFunctionIsRejected(t *testing.T) {
	tests := []struct {
		name        string
		declaration string
	}{
		{name: "capturing", declaration: "fn Box::read() Int { value }"},
		{name: "noncapturing", declaration: "fn Box::read() Int { 42 }"},
		{name: "test", declaration: "test fn Box::read() Void!Str { Result::ok(()) }"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := checkNestedNamedFunctionSource(t, `
				struct Box { value: Int }
				fn outer(value: Int) {
					`+test.declaration+`
					()
				}
			`)
			diagnostics := checker.Diagnostics()
			if len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticCodeStaticFunctionNotTopLevel {
				t.Fatalf("checker diagnostics = %v, want one static function scope diagnostic", diagnostics)
			}
		})
	}
}

func TestNestedTestFunctionIsRejected(t *testing.T) {
	checker := checkNestedNamedFunctionSource(t, `
		fn outer() {
			test fn local_test() Void!Str { Result::ok(()) }
			()
		}
	`)
	for _, diagnostic := range checker.Diagnostics() {
		if diagnostic.Code == DiagnosticCodeTestNotTopLevel {
			return
		}
	}
	t.Fatalf("checker diagnostics = %v, want nested test function rejection", checker.Diagnostics())
}

func TestNestedNamedFunctionKeepsSequentialScope(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		undefined string
	}{
		{
			name: "forward call",
			source: `
				fn outer() Int {
					inner()
					fn inner() Int { 42 }
				}
			`,
			undefined: "inner",
		},
		{
			name: "mutual recursion through later declaration",
			source: `
				fn outer() {
					fn first() { second() }
					fn second() { first() }
					first()
				}
			`,
			undefined: "second",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := checkNestedNamedFunctionSource(t, test.source)
			if !checker.HasErrors() {
				t.Fatal("checker succeeded; expected later local declaration to remain undefined")
			}
			want := "Undefined function: " + test.undefined
			if got := checker.Diagnostics()[0].Message; got != want {
				t.Fatalf("first diagnostic = %q, want %q", got, want)
			}
		})
	}
}
