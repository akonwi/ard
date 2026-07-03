package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestGoImportSupportsGoInterfaces(t *testing.T) {
	run(t, []test{
		{
			name: "named Go interface can be used as parameter",
			input: `use go:io

fn consume(writer: io::Writer) {}`,
		},
		{
			name: "Go concrete value satisfies Go interface",
			input: `use go:bytes
use go:fmt

fn main() Int!Str {
  fmt::Fprint(bytes::NewBufferString("hello"), "!")
}`,
		},
		{
			name: "Ard struct explicitly implements Go interface",
			input: `use go:io

struct Sink {
  written: Int,
}

impl io::Writer for Sink {
  fn mut write(bytes: [Byte]) Int!Str {
    self.written =+ bytes.size()
    Result::ok(bytes.size())
  }
}

fn consume(writer: io::Writer) {}

fn main() {
  mut sink = Sink{written: 0}
  consume(sink)
}`,
		},
		{
			name: "Go interface impl rejects mutable parameters",
			input: `use go:io

struct Sink {}

impl io::Writer for Sink {
  fn write(mut bytes: [Byte]) Int!Str {
    Result::ok(bytes.size())
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Go interface method 'write' parameter 'bytes' cannot be mutable"}},
		},
		{
			name: "mutable Go interface impl accepts mutable field",
			input: `use go:io

struct Sink {
  written: Int,
}

struct Box {
  sink: Sink,
}

impl io::Writer for Sink {
  fn mut write(bytes: [Byte]) Int!Str {
    self.written =+ bytes.size()
    Result::ok(bytes.size())
  }
}

fn consume(writer: io::Writer) {}

fn main() {
  mut box = Box{sink: Sink{written: 0}}
  consume(box.sink)
}`,
		},
		{
			name: "mutable Go interface impl requires mutable value",
			input: `use go:io

struct Sink {
  written: Int,
}

impl io::Writer for Sink {
  fn mut write(bytes: [Byte]) Int!Str {
    self.written =+ bytes.size()
    Result::ok(bytes.size())
  }
}

fn consume(writer: io::Writer) {}

fn main() {
  let sink = Sink{written: 0}
  consume(sink)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected io::Writer, got Sink"}},
		},
		{
			name: "invalid Go interface impl is not recorded",
			input: `use go:io

struct Sink {}

impl io::Writer for Sink {
}

fn consume(writer: io::Writer) {}

fn main() {
  let sink = Sink{}
  consume(sink)
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Missing method 'write' in Go interface 'io::Writer'"},
				{Kind: checker.Error, Message: "Type mismatch: Expected io::Writer, got Sink"},
			},
		},
		{
			name: "duplicate inherent method conflicts with Go interface impl",
			input: `use go:io

struct Sink {}

impl Sink {
  fn write(bytes: [Byte]) Int!Str {
    Result::ok(bytes.size())
  }
}

impl io::Writer for Sink {
  fn write(bytes: [Byte]) Int!Str {
    Result::ok(bytes.size())
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Duplicate method: write"}},
		},
		{
			name: "missing explicit Go interface impl is rejected",
			input: `use go:io

struct Sink {
  written: Int,
}

impl Sink {
  fn mut write(bytes: [Byte]) Int!Str {
    self.written =+ bytes.size()
    Result::ok(bytes.size())
  }
}

fn consume(writer: io::Writer) {}

fn main() {
  mut sink = Sink{written: 0}
  consume(sink)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected io::Writer, got Sink"}},
		},
	})
}

func TestGoImportAcceptsFunctionCallbacks(t *testing.T) {
	run(t, []test{
		{
			name: "function callback parameter",
			input: `use go:sort

fn find() Int {
  sort::Search(10, fn(i) { i == 5 })
}`,
		},
	})
}

func TestGoImportConstructsGoStructLiterals(t *testing.T) {
	run(t, []test{
		{
			name: "keyed struct literal",
			input: `use go:image

fn make() Int {
  let point = image::Point{X: 10, Y: 20}
  point.X
}`,
		},
		{
			name: "partial keyed struct literal",
			input: `use go:image

fn make() Int {
  let point = image::Point{X: 10}
  point.X
}`,
		},
		{
			name: "unknown field rejected",
			input: `use go:image

fn make() {
  image::Point{Z: 10}
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unknown field: Z"}},
		},
		{
			name: "non-struct named type rejected",
			input: `use go:time

fn make() {
  time::Duration{}
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Go struct literals require a non-pointer Go struct type"}},
		},
		{
			name: "duplicate field rejected",
			input: `use go:image

fn make() {
  image::Point{X: 1, X: 2}
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Duplicate field: X"}},
		},
	})
}

func TestGoImportAssignsExportedStructFields(t *testing.T) {
	run(t, []test{
		{
			name: "mutable struct field assignment",
			input: `use go:image

fn update() {
  mut rect = image::Rect(1, 2, 3, 4)
  rect.Min.X = 10
}`,
		},
		{
			name: "immutable struct field assignment rejected",
			input: `use go:image

fn update() {
  let rect = image::Rect(1, 2, 3, 4)
  rect.Min.X = 10
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Immutable: rect.Min.X"}},
		},
	})
}

func TestGoImportResolvesExportedStructFields(t *testing.T) {
	run(t, []test{
		{
			name: "nested exported struct fields",
			input: `use go:image

fn min_x() Int {
  let rect = image::Rect(1, 2, 3, 4)
  rect.Min.X
}`,
		},
	})
}

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

func TestGoImportSupportsForeignMethodValues(t *testing.T) {
	run(t, []test{
		{
			name: "bound method value with direct return",
			input: `use go:regexp

fn main() Bool {
  let re = try regexp::Compile("[a-z]+") -> err { panic(err) }
  let matches: fn(Str) Bool = re.MatchString
  matches("abc")
}`,
		},
		{
			name: "bound method value with adapted result return",
			input: `use go:time

fn main() [Byte]!Str {
  let when = time::Now()
  let marshal: fn() [Byte]!Str = when.MarshalText
  marshal()
}`,
		},
		{
			name: "pointer receiver method value on mutable opaque value",
			input: `use go:time

fn main() Void!Str {
  mut when = time::Now()
  let unmarshal: fn(mut [Byte]) Void!Str = when.UnmarshalText
  mut text = "2024-01-02T00:00:00Z".bytes()
  unmarshal(text)
}`,
		},
		{
			name: "pointer receiver method value on immutable opaque value rejected",
			input: `use go:time

fn main() {
  let when = time::Now()
  let _ = when.UnmarshalText
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot access pointer receiver method time::Time.UnmarshalText on immutable value"}},
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
			name: "variadic foreign method is one Ard argument",
			input: `use go:log

fn main() {
  let logger = log::Default()
  logger.Println("hello")
}`,
		},
		{
			name: "variadic foreign method rejects zero variadic args",
			input: `use go:log

fn main() {
  let logger = log::Default()
  logger.Println()
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "missing argument for parameter: v"}},
		},
		{
			name: "variadic foreign method rejects multiple variadic args",
			input: `use go:log

fn main() {
  let logger = log::Default()
  logger.Println("hello", "world")
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 1, got 2"}},
		},
		{
			name: "foreign interface method argument type mismatch is reported",
			input: `use go:regexp

fn main() {
  let re = try regexp::Compile("[a-z]+") -> err { panic(err) }
  re.FindReaderIndex("abc")
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected io::RuneReader, got Str"}},
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

func TestGoImportReportsFunctionCallErrors(t *testing.T) {
	run(t, []test{
		{
			name: "interface function reports arity after signature is supported",
			input: `use go:fmt
fmt::Fprint("hello")`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 2, got 1"}},
		},
		{
			name: "function callback reports arity after interface parameters are supported",
			input: `use go:net/http
http::HandleFunc()`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 2, got 0"}},
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

func TestGoNamedMapTypesExposeMapMethods(t *testing.T) {
	run(t, []test{
		{
			name: "url Values returned from Go exposes map get",
			input: `use go:net/url
fn first() {
  let values = try url::ParseQuery("a=1") -> err { panic(err) }
  values.get("a").expect("missing")
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
