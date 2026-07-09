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
			name: "Go interface impl allows mutable descriptor parameters",
			input: `use go:io

struct Sink {}

impl io::Writer for Sink {
  fn write(bytes: mut [Byte]) Int!Str {
    Result::ok(bytes.size())
  }
}`,
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

func TestGoImportResolvesChannelFunction(t *testing.T) {
	run(t, []test{
		{
			name: "time After returns receive-only channel",
			input: `use go:time
fn main() {
  let timeout: Receiver<time::Time> = time::After(1)
  let _ = timeout.recv()
}`,
		},
		{
			name: "receive-only Go channel rejects send",
			input: `use go:time
fn main() {
  let timeout = time::After(1)
  timeout.send(time::Now())
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Undefined: timeout.send"}},
		},
		{
			name: "Go struct channel field is receive-only channel",
			input: `use go:time
fn main() {
  let ticker = time::NewTicker(1)
  let ticks: Receiver<time::Time> = ticker.C
}`,
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
			name: "variadic foreign method allows zero variadic args",
			input: `use go:log

fn main() {
  let logger = log::Default()
  logger.Println()
}`,
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

func TestMutableForeignAnnotationsMatchGoPointerTypes(t *testing.T) {
	run(t, []test{
		{
			name: "mut foreign parameter forwards to another mut foreign parameter",
			input: `use go:strings

fn inner(b: mut strings::Builder) {}

fn outer(b: mut strings::Builder) {
  inner(b)
}`,
		},
		{
			name: "mut foreign parameter exposes pointer receiver methods",
			input: `use go:strings

fn write(b: mut strings::Builder) {
  try b.WriteString("hello") -> err { panic(err) }
}`,
		},
		{
			name: "mut foreign parameter allows field writes",
			input: `use go:image

fn reset(p: mut image::Point) {
  p.X = 0
}`,
		},
	})
}

func TestForeignScalarNarrowingLimits(t *testing.T) {
	run(t, []test{
		{
			name: "foreign scalar narrows to primitive value parameter",
			input: `use go:time

fn double(value: Int) Int {
  value * 2
}

fn main() {
  let _ = double(time::January)
}`,
		},
		{
			name: "foreign scalar is rejected for mutable primitive parameter",
			input: `use go:time

fn bump(value: mut Int) {
  value = value + 1
}

fn main() {
  mut month = time::January
  bump(month)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected a mutable Int"}},
		},
		{
			name: "foreign scalar Maybe does not compare against primitive Maybe",
			input: `use go:time

fn main() {
  let month: time::Month? = Maybe::some(time::January)
  let number: Int? = Maybe::some(1)
  let _ = month == number
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Invalid: time::Month? == Int?"}},
		},
	})
}

func TestForeignScalarWideningAndArdSites(t *testing.T) {
	run(t, []test{
		{
			name: "Str widens into a foreign string newtype parameter",
			input: `use go:encoding/json

fn take(n: json::Number) Str {
  "got"
}

fn main() {
  let _ = take("42")
}`,
		},
		{
			name: "Str widens into foreign string newtype map keys",
			input: `use go:encoding/json

fn main() {
  let m: [json::Number: Int] = ["a": 1]
  let _ = m.get("a")
}`,
		},
		{
			name: "foreign string newtype interpolates as its underlying Str",
			input: `use go:encoding/json

fn show(n: json::Number) Str {
  "{n}"
}`,
		},
		{
			name: "foreign string newtype falls back to primitive methods",
			input: `use go:encoding/json

fn size(n: json::Number) Int {
  n.size()
}`,
		},
		{
			name: "real Go methods on the newtype win over primitive fallback",
			input: `use go:encoding/json

fn as_float(n: json::Number) Float64!Str {
  n.Float64()
}`,
		},
		{
			name: "pkg::T(x) converts a Str into the foreign newtype",
			input: `use go:encoding/json

fn main() {
  let n: json::Number = json::Number("42")
}`,
		},
		{
			name: "pkg::T(x) accepts an identity conversion",
			input: `use go:encoding/json

fn main() {
  let n = json::Number("42")
  let again: json::Number = json::Number(n)
}`,
		},
		{
			name: "pkg::T(x) converts between foreign scalars sharing an underlying",
			input: `use go:html/template

fn main() {
  let js = template::JS("1")
  let html: template::HTML = template::HTML(js)
}`,
		},
		{
			name: "pkg::T(x) rejects a second argument",
			input: `use go:encoding/json

fn main() {
  let _ = json::Number("4", "2")
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 1, got 2"}},
		},
		{
			name: "pkg::T(x) rejects a mismatched argument",
			input: `use go:encoding/json

fn main() {
  let _ = json::Number(42)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"}},
		},
		{
			name: "Int does not widen into a numeric foreign scalar implicitly",
			input: `use go:time

fn take(month: time::Month) Bool {
  month == time::January
}

fn main() {
  let _ = take(1)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected time::Month, got Int"}},
		},
		{
			name: "Str is rejected for a mutable foreign newtype parameter",
			input: `use go:encoding/json

fn rewrite(n: mut json::Number) {
}

fn main() {
  mut s = "42"
  rewrite(s)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected mut json::Number, got Str"}},
		},
	})
}

func TestForeignNamedScalarsSupportEquality(t *testing.T) {
	run(t, []test{
		{
			name: "foreign named scalar values compare with ==",
			input: `use go:time

fn is_january(month: time::Month) Bool {
  month == time::January
}`,
		},
		{
			name: "foreign named scalars in match-like dispatch",
			input: `use go:time

fn label(day: time::Weekday) Str {
  match day == time::Sunday {
    true => "rest",
    false => "work",
  }
}`,
		},
	})
}

func TestUnsafeCastAcceptsForeignTargets(t *testing.T) {
	run(t, []test{
		{
			name: "value cast to foreign struct type",
			input: `use ard/unsafe
use go:image

fn origin(value: Any) Int {
  match unsafe::cast<image::Point>(value) {
    point => point.X,
    _ => 0,
  }
}`,
		},
		{
			name: "mutable cast to foreign struct type allows mutation",
			input: `use ard/unsafe
use go:image

fn reset(value: Any) {
  match unsafe::cast<mut image::Point>(value) {
    point => { point.X = 0 },
    _ => {},
  }
}`,
		},
	})
}

func TestForeignTypeMatchOverDynamicSubjects(t *testing.T) {
	run(t, []test{
		{
			name: "match over Any with foreign type patterns",
			input: `use go:image

fn describe(value: Any) Str {
  match value {
    image::Point(point) => "point at {point.X}",
    image::Rectangle(_) => "rectangle",
    _ => "unknown",
  }
}`,
		},
		{
			name: "match over empty-interface alias subject",
			input: `use go:encoding/xml
use go:image

fn describe(token: xml::Token) Str {
  match token {
    image::Point(_) => "point",
    _ => "other",
  }
}`,
		},
		{
			name: "catch-all is required",
			input: `use go:image

fn describe(value: Any) Str {
  match value {
    image::Point(_) => "point",
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Match on a dynamic value requires a catch-all '_' case because the type set is open"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Void"},
			},
		},
		{
			name: "duplicate type patterns are diagnosed",
			input: `use go:image

fn describe(value: Any) Str {
  match value {
    image::Point(_) => "point",
    image::Point(_) => "again",
    _ => "other",
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Warn, Message: "Duplicate case: image::Point"}},
		},
		{
			name: "interface patterns are rejected",
			input: `use go:io

fn describe(value: Any) Str {
  match value {
    io::Writer(_) => "writer",
    _ => "other",
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Foreign type pattern must name a concrete foreign type, got io::Writer"}},
		},
		{
			name: "non-dynamic subjects keep existing match semantics",
			input: `fn describe(value: Int) Str {
  match value {
    1 => "one",
    _ => "many",
  }
}`,
		},
	})
}

func TestGoImportMapsEmptyInterfaceAliasesToAny(t *testing.T) {
	run(t, []test{
		{
			name: "empty-interface alias resolves as Any in signatures",
			input: `use go:encoding/xml

fn describe(token: xml::Token) Str {
  "token"
}

fn main() {
  let _ = describe("raw")
  let _ = describe(42)
}`,
		},
		{
			name: "empty-interface alias accepts any Ard value as Any",
			input: `use go:database/sql/driver

struct Row {
  id: Int,
}

fn store(value: driver::Value) {}

fn main() {
  store(Row{id: 1})
  let boxed: Any = true
  store(boxed)
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

func TestGoImportResolvesExportedPackageVariable(t *testing.T) {
	run(t, []test{
		{
			name: "pointer variable passed to interface parameter",
			input: `use go:fmt
use go:os

fn main() {
  let _ = try fmt::Fprintln(os::Stdout, "hello") -> err { panic(err) }
}`,
		},
	})
}

func TestGoImportAssignsExportedPackageVariable(t *testing.T) {
	run(t, []test{
		{
			name: "package variable assignment accepted",
			input: `use go:os

fn main() {
  os::Stdout = os::Stdout
}`,
		},
		{
			name: "package variable type mismatch rejected",
			input: `use go:os

fn main() {
  os::Stdout = 5
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected mut os::File, got Int"}},
		},
		{
			name: "package constant assignment rejected",
			input: `use go:time

fn main() {
  time::Nanosecond = 1
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Cannot assign to Go constant: time::Nanosecond"}},
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
			// The variadic tail is omittable, so one argument is valid arity;
			// the remaining error is the first argument's type.
			name: "interface function checks provided args when variadic tail is omitted",
			input: `use go:fmt
fmt::Fprint("hello")`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected io::Writer, got Str"}},
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
			name: "zero variadic arguments allowed",
			input: `use go:fmt
fn main() {
  let _ = fmt::Println()
}`,
		},
		{
			name: "omitting a required argument still rejected",
			input: `use go:strings
fn main() {
  let _ = strings::ToUpper()
}`,
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

// A named Go map type accepts an Ard map value or literal with the same
// key/value shape, mirroring Go's unnamed-to-named assignability
// (for example `ui.ShortcutMap map[string]Intent`).
func TestNamedGoMapTypesAcceptArdMaps(t *testing.T) {
	run(t, []test{
		{
			name: "map literal contextually types as a named Go map",
			input: `use go:net/url
fn build() url::Values {
  let values: url::Values = ["a": ["1"]]
  values
}`,
		},
		{
			name: "map value satisfies a named Go map parameter",
			input: `use go:net/url
fn encode() Str {
  let query: [Str: [Str]] = ["a": ["1"]]
  let values: url::Values = query
  values.Encode()
}`,
		},
		{
			name: "mismatched map shape is rejected",
			input: `use go:net/url
fn bad() {
  let values: url::Values = ["a": 1]
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected [Str], got Int"}},
		},
	})
}

// A named Go slice type accepts an Ard list value or literal with the same
// element type, mirroring Go's unnamed-to-named assignability (for example
// `sort.IntSlice []int`).
func TestNamedGoSliceTypesAcceptArdLists(t *testing.T) {
	run(t, []test{
		{
			name: "list literal contextually types as a named Go slice",
			input: `use go:sort
fn build() sort::IntSlice {
  let nums: sort::IntSlice = [3, 1, 2]
  nums
}`,
		},
		{
			name: "list value satisfies a named Go slice annotation",
			input: `use go:sort
fn build() sort::IntSlice {
  let raw: [Int] = [1, 2]
  let nums: sort::IntSlice = raw
  nums
}`,
		},
		{
			name: "named Go slices expose list methods",
			input: `use go:sort
fn peek(nums: sort::IntSlice) Int {
  nums.at(0).expect("bounds") + nums.size()
}`,
		},
		{
			name: "real Go methods on the named slice still resolve",
			input: `use go:sort
fn sorted(nums: mut sort::IntSlice) sort::IntSlice {
  nums.Sort()
  nums
}`,
		},
		{
			name: "mismatched element type is rejected",
			input: `use go:sort
fn bad() {
  let nums: sort::IntSlice = ["a"]
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Str"}},
		},
	})
}

// A freshly constructed container literal is new storage with no other
// observer, so it satisfies a mutable Go slice/map parameter directly.
func TestFreshContainerLiteralsSatisfyMutableGoParams(t *testing.T) {
	run(t, []test{
		{
			name: "list literal passes to a mutable Go slice parameter",
			input: `use go:sort
fn main() {
  sort::Ints([3, 1, 2])
}`,
		},
		{
			name: "map literal passes to a mutable Ard map parameter",
			input: `fn consume(m: mut [Str: Int]) Int {
  m.size()
}
fn main() {
  let _ = consume(["a": 1])
}`,
		},
		{
			name: "list literal against a non-list annotation reports a diagnostic",
			input: `fn main() {
  let x: Int = [1]
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Expected Int but got a list"}},
		},
		{
			name: "immutable bindings are still rejected for mutable Go params",
			input: `use go:sort
fn main() {
  let nums = [3, 1]
  sort::Ints(nums)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected a mutable [Int]"}},
		},
	})
}

// Named empty Go interfaces keep their type identity (they are foreign
// interface types, not Any) but still accept any value and support dynamic
// matching. driver.Value is `type Value any`.
func TestNamedEmptyGoInterfacesKeepIdentity(t *testing.T) {
	run(t, []test{
		{
			name: "any value passes to a named empty interface parameter",
			input: `use go:database/sql/driver
fn check() Bool {
  driver::IsValue("hello")
}`,
		},
		{
			name: "named empty interface values are dynamically matchable",
			input: `use go:database/sql/driver
use go:time
fn describe(value: driver::Value) Str {
  match value {
    time::Duration(d) => "duration",
    _ => "other",
  }
}`,
		},
	})
}

// A named Go func type accepts an Ard closure with a matching signature,
// mirroring Go's unnamed-to-named assignability.
func TestNamedGoFuncTypesAcceptClosures(t *testing.T) {
	run(t, []test{
		{
			name: "closure satisfies a named Go func annotation",
			input: `use go:net/http
fn handler() http::HandlerFunc {
  let f: http::HandlerFunc = fn(w: http::ResponseWriter, r: mut http::Request) {}
  f
}`,
		},
		{
			name: "mismatched closure is rejected",
			input: `use go:net/http
fn bad_fn(x: Int) {}
fn bad() {
  let f: http::HandlerFunc = bad_fn
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected http::HandlerFunc, got fn(Int)"}},
		},
	})
}

// Named Go func values are callable with their underlying signature and flow
// where Ard function types are expected (Go's named-to-unnamed assignability).
func TestNamedGoFuncValuesAreCallable(t *testing.T) {
	run(t, []test{
		{
			name: "named func value satisfies an Ard fn annotation",
			input: `use go:context
fn wrap(cancel: context::CancelFunc) fn() Void {
  let f: fn() Void = cancel
  f
}`,
		},
		{
			name: "calling a named func value directly",
			input: `use go:context
fn run(cancel: context::CancelFunc) {
  cancel()
}`,
		},
		{
			name: "calling a named func value with wrong arity is rejected",
			input: `use go:net/http
fn call(handler: http::HandlerFunc) {
  handler()
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "missing argument for parameter: arg1"}},
		},
	})
}

// A `mut` parameter in a function-type annotation resolves like a named `mut`
// parameter: foreign Go types take their pointer form, so an annotation like
// `fn(http::ResponseWriter, mut http::Request)` unifies with an imported Go
// signature such as http.HandlerFunc. The value form does not match a Go
// pointer parameter: pointer-ness must be spelled with `mut`.
func TestFunctionTypeAnnotationsUnifyWithGoSignatures(t *testing.T) {
	run(t, []test{
		{
			name: "mut annotation matches an imported Go pointer parameter",
			input: `use go:net/http
fn store(handler: http::HandlerFunc) fn(http::ResponseWriter, mut http::Request) {
  let f: fn(http::ResponseWriter, mut http::Request) = handler
  f
}`,
		},
		{
			name: "value annotation does not match a Go pointer parameter",
			input: `use go:net/http
fn store(handler: http::HandlerFunc) {
  let f: fn(http::ResponseWriter, http::Request) = handler
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected fn(http::ResponseWriter, http::Request), got http::HandlerFunc"}},
		},
		{
			name: "closure with mut foreign param satisfies the annotation",
			input: `use go:net/http
fn main() {
  let f: fn(http::ResponseWriter, mut http::Request) = fn(w: http::ResponseWriter, r: mut http::Request) {}
  let _ = f
}`,
		},
	})
}
