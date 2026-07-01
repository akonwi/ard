package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestGoImportResolvesExportedPackageFunction(t *testing.T) {
	run(t, []test{
		{
			name: "fmt Println accepts any single Ard value and returns Result",
			input: `use go:fmt

fn main() Int!Str {
  fmt::Println("hello")
}`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{Expr: &checker.FunctionDef{
						Name:       "main",
						Parameters: []checker.Parameter{},
						ReturnType: checker.MakeResult(checker.Int, checker.Str),
						Body: &checker.Block{Stmts: []checker.Statement{
							{Expr: &checker.ForeignFunctionCall{
								Target:    "go",
								Namespace: "fmt",
								Qualifier: "fmt",
								Symbol:    "Println",
								Call: &checker.FunctionCall{
									Name:       "Println",
									Args:       []checker.Expression{&checker.StrLiteral{Value: "hello"}},
									ReturnType: checker.MakeResult(checker.Int, checker.Str),
								},
							}},
						}},
					}},
				},
			},
		},
	})
}

func TestGoImportResolvesErrorOnlyFunction(t *testing.T) {
	run(t, []test{
		{
			name: "os Setenv returns Void result",
			input: `use go:os

fn main() Void!Str {
  os::Setenv("ARD_TEST", "ok")
}`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{Expr: &checker.FunctionDef{
						Name:       "main",
						Parameters: []checker.Parameter{},
						ReturnType: checker.MakeResult(checker.Void, checker.Str),
						Body: &checker.Block{Stmts: []checker.Statement{
							{Expr: &checker.ForeignFunctionCall{
								Target:    "go",
								Namespace: "os",
								Qualifier: "os",
								Symbol:    "Setenv",
								Call: &checker.FunctionCall{
									Name:       "Setenv",
									Args:       []checker.Expression{&checker.StrLiteral{Value: "ARD_TEST"}, &checker.StrLiteral{Value: "ok"}},
									ReturnType: checker.MakeResult(checker.Void, checker.Str),
								},
							}},
						}},
					}},
				},
			},
		},
	})
}

func TestGoImportRejectsUnknownFunction(t *testing.T) {
	run(t, []test{
		{
			name: "unknown exported function",
			input: `use go:fmt
fmt::Nope("hello")`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Undefined Go function: fmt::Nope"}},
		},
	})
}

func TestGoImportReportsUnsupportedFunctionSignature(t *testing.T) {
	run(t, []test{
		{
			name: "exported function with unsupported signature",
			input: `use go:fmt
fmt::Fprint("hello")`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported Go function fmt::Fprint: parameter 1 has unsupported type io.Writer: only basic scalar, slice, and any types are supported"}},
		},
	})
}

func TestGoVariadicIsSingleArdArgument(t *testing.T) {
	run(t, []test{
		{
			name: "zero variadic arguments rejected",
			input: `use go:fmt
fmt::Println()`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 1, got 0"}},
		},
		{
			name: "multiple variadic arguments rejected",
			input: `use go:fmt
fmt::Println("a", "b")`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 1, got 2"}},
		},
	})
}

func TestGoFunctionCallsRejectNamedArguments(t *testing.T) {
	run(t, []test{
		{
			name: "named argument",
			input: `use go:fmt
fmt::Println(a: "hello")`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Go function calls do not support named arguments"}},
		},
	})
}

func TestGoSliceParametersRequireMutableLists(t *testing.T) {
	run(t, []test{
		{
			name: "mutable list accepted for Go slice parameter",
			input: `use go:sort
fn main() {
  mut values = [3, 1, 2]
  sort::Ints(values)
}`,
		},
		{
			name: "immutable list rejected for Go slice parameter",
			input: `use go:sort
fn main() {
  let values = [3, 1, 2]
  sort::Ints(values)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected a mutable [Int]"}},
		},
	})
}

func TestGoSliceReturnsMapToLists(t *testing.T) {
	run(t, []test{
		{
			name: "strings Split returns list of strings",
			input: `use go:strings
fn split() [Str] {
  strings::Split("a,b", ",")
}`,
		},
	})
}

func TestAnyAcceptsAnyArdValue(t *testing.T) {
	run(t, []test{
		{
			name: "assign primitives to Any",
			input: `let a: Any = "hello"
let b: Any = 1
let c: Any = true`,
		},
	})
}
