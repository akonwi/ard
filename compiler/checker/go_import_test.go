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

func TestGoImportResolvesCommaOkFunction(t *testing.T) {
	run(t, []test{
		{
			name: "os LookupEnv returns Maybe",
			input: `use go:os
let home: Str? = os::LookupEnv("HOME")`,
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

func TestGoImportSupportsForeignMethods(t *testing.T) {
	run(t, []test{
		{
			name: "method on opaque pointer Go type",
			input: `use go:regexp

fn main() Bool {
  let re = try regexp::Compile("[a-z]+") -> err { panic(err) }
  re.MatchString("abc")
}`,
		},
		{
			name: "method on opaque value Go type",
			input: `use go:time

fn main() Str {
  let loc = try time::LoadLocation("UTC") -> err { panic(err) }
  let when = time::Date(2024, time::January, 2, 0, 0, 0, 0, loc)
  when.Format(time::RFC3339)
}`,
		},
		{
			name: "method chain through returned opaque value keeps methods",
			input: `use go:time

fn main() Str {
  time::Now().Local().Format(time::RFC3339)
}`,
		},
		{
			name: "pointer receiver method on mutable opaque value",
			input: `use go:time

fn main() Void!Str {
  mut when = time::Now()
  mut text = "2024-01-02T00:00:00Z".bytes()
  when.UnmarshalText(text)
}`,
		},
		{
			name: "unsupported foreign method signature is reported",
			input: `use go:regexp

fn main() {
  let re = try regexp::Compile("[a-z]+") -> err { panic(err) }
  re.FindReaderIndex("abc")
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported foreign method mut regexp::Regexp.FindReaderIndex: parameter 1 has unsupported type io.RuneReader: Go interface types are not supported yet"}},
		},
		{
			name: "pointer receiver method on immutable opaque value rejected",
			input: `use go:time

fn main() {
  let when = time::Now()
  mut text = "2024-01-02T00:00:00Z".bytes()
  when.UnmarshalText(text)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot call pointer receiver method time::Time.UnmarshalText on immutable value"}},
		},
	})
}

func TestGoImportSupportsOpaqueNamedTypes(t *testing.T) {
	run(t, []test{
		{
			name: "pointer to named Go type can be returned and passed back",
			input: `use go:time

fn main() {
  let loc = try time::LoadLocation("UTC") -> err { panic(err) }
  let _ = time::Date(2024, time::January, 2, 0, 0, 0, 0, loc)
}`,
		},
	})
}

func TestGoImportResolvesExportedPackageConstant(t *testing.T) {
	run(t, []test{
		{
			name: "typed named scalar constant",
			input: `use go:time

fn main() {
  time::Sleep(time::Nanosecond)
}`,
		},
		{
			name: "untyped float constant",
			input: `use go:math
let pi: Float64 = math::Pi`,
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
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported Go function fmt::Fprint: parameter 1 has unsupported type io.Writer: Go interface types are not supported yet"}},
		},
		{
			name: "named func type is unsupported",
			input: `use go:net/http
http::HandleFunc("/", fn() { () })`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported Go function http::HandleFunc: parameter 2 has unsupported type func(net/http.ResponseWriter, *net/http.Request): only basic scalar, slice, and any types are supported"}},
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
