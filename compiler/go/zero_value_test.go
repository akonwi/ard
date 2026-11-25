package gotarget

import (
	"bytes"
	"fmt"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/compiler/air"
	"github.com/akonwi/ard/compiler/frontend"
)

// zeroValueFixture is a Go package declaring one named type per Go type kind
// that can cross the FFI boundary.
const zeroValueFixture = `package fixture

type Iface interface{ Name() string }
type Empty interface{}
type Fn func(int) int
type M map[string]int
type S []int
type A [3]int
type Str string
type B bool
type F float64
type St struct{ X int }
type Box[T any] struct{ V T }
`

// TestGoTargetSynthesizesValidZeroValues covers every position where the Go
// backend synthesizes a zero value for a value it never produces (the unset
// success result of a failing ABI return, and the unreachable value after a
// panic), for every kind of Go type an Ard type can lower to. The generated
// program must compile, so each zero must be a valid Go expression of exactly
// that type.
func TestGoTargetSynthesizesValidZeroValues(t *testing.T) {
	cases := []struct {
		name    string
		imports string
		typ     string
		// zero is the expression a typed position (an ABI result) uses.
		zero string
		// panicZero is the expression after a panic, which must carry its
		// own type because it can be bound with :=.
		panicZero string
	}{
		{name: "go named interface", typ: "fx::Iface", zero: "nil", panicZero: "*new(fixture.Iface)"},
		{name: "go named empty interface", typ: "fx::Empty", zero: "nil", panicZero: "*new(fixture.Empty)"},
		{name: "go stdlib interface", imports: "use go:io as goio", typ: "goio::Reader", zero: "nil", panicZero: "*new(io.Reader)"},
		{name: "go error", typ: "Error", zero: "nil", panicZero: "*new(error)"},
		{name: "go named func", typ: "fx::Fn", zero: "nil", panicZero: "*new(fixture.Fn)"},
		{name: "function", typ: "(fn(Int) Int)", zero: "nil", panicZero: "*new(func(int) int)"},
		{name: "chan", typ: "Chan<Int>", zero: "nil", panicZero: "*new(chan int)"},
		{name: "receiver", typ: "Receiver<Int>", zero: "nil", panicZero: "*new(<-chan int)"},
		{name: "sender", typ: "Sender<Int>", zero: "nil", panicZero: "*new(chan<- int)"},
		{name: "go named map", typ: "fx::M", zero: "fixture.M{}", panicZero: "fixture.M{}"},
		{name: "map", typ: "[Str:Int]", zero: "map[string]int{}", panicZero: "map[string]int{}"},
		{name: "go named slice", typ: "fx::S", zero: "fixture.S{}", panicZero: "fixture.S{}"},
		{name: "list", typ: "[Int]", zero: "[]int{}", panicZero: "[]int{}"},
		{name: "go named array", typ: "fx::A", zero: "fixture.A{}", panicZero: "fixture.A{}"},
		{name: "go pointer", typ: "(&mut fx::St)", zero: "nil", panicZero: "*new(*fixture.St)"},
		{name: "go struct", typ: "fx::St", zero: "fixture.St{}", panicZero: "fixture.St{}"},
		{name: "go generic struct", typ: "fx::Box<Int>", zero: "fixture.Box[int]{}", panicZero: "fixture.Box[int]{}"},
		{name: "go named string", typ: "fx::Str", zero: "\"\"", panicZero: "*new(fixture.Str)"},
		{name: "go named bool", typ: "fx::B", zero: "false", panicZero: "*new(fixture.B)"},
		{name: "go named float", typ: "fx::F", zero: "0", panicZero: "*new(fixture.F)"},
		{name: "go duration", imports: "use go:time", typ: "time::Duration", zero: "0", panicZero: "*new(time.Duration)"},
		{name: "go file mode", imports: "use go:os", typ: "os::FileMode", zero: "0", panicZero: "*new(fs.FileMode)"},
		{name: "int", typ: "Int", zero: "0", panicZero: "0"},
		{name: "float", typ: "Float64", zero: "0", panicZero: "*new(float64)"},
		{name: "byte", typ: "Byte", zero: "0", panicZero: "*new(byte)"},
		{name: "rune", typ: "Rune", zero: "0", panicZero: "*new(rune)"},
		{name: "str", typ: "Str", zero: "\"\"", panicZero: "\"\""},
		{name: "bool", typ: "Bool", zero: "false", panicZero: "false"},
		{name: "enum", typ: "Color", zero: "0", panicZero: "*new(Color)"},
		{name: "struct", typ: "Point", zero: "Point{}", panicZero: "Point{}"},
		{name: "any", typ: "Any", zero: "nil", panicZero: "*new(any)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			generated := runZeroValueProgram(t, tc.imports, tc.typ)
			for _, want := range []string{
				"return " + tc.zero + ", false",
				"return " + tc.zero + ", errors.New(",
				"value := " + tc.panicZero + "\n",
			} {
				if !strings.Contains(generated, want) {
					t.Errorf("generated Go is missing %q:\n%s", want, generated)
				}
			}
		})
	}
}

func runZeroValueProgram(t *testing.T, imports, typ string) string {
	t.Helper()
	projectDir := t.TempDir()
	writeTestFile(t, filepath.Join(projectDir, "ard.toml"), "name = \"zerovalue\"\nard = \">= 0.1.0\"\n")
	writeTestFile(t, filepath.Join(projectDir, "go.mod"), "module zerovalue\n\ngo 1.27\n")
	writeTestFile(t, filepath.Join(projectDir, "ffi", "fixture", "fixture.go"), zeroValueFixture)
	source := strings.ReplaceAll(fmt.Sprintf(`use go:zerovalue/ffi/fixture as fx
%s

enum Color { red, green }

struct Point {
  x: Int,
}

// Maybe ABI return: the none path returns (zero, false).
fn none_value() __T__? {
  Maybe::new()
}

// Result ABI return: the error path returns (zero, err).
fn err_value() __T__!Str {
  Result::err("failed")
}

// Tuple-spread Result ABI return: one zero per element.
fn err_tuple() (__T__, Int)!Str {
  Result::err("failed")
}

// try on a Maybe inside a Maybe ABI return propagates (zero, false).
fn try_value(value: __T__?) __T__? {
  let unwrapped = try value
  Maybe::new(unwrapped)
}

// The value of a panic expression is unreachable but must still type-check
// where it is bound.
fn panic_value(fail: Bool) __T__ {
  let value: __T__ = panic("unreachable")
  value
}

fn panic_reassign(start: __T__) __T__ {
  mut value = start
  value = panic("unreachable")
  value
}

fn panic_return() __T__ {
  panic("unreachable")
}

// A generic Maybe return instantiated with T.
fn generic_none(values: [$V]) $V? {
  Maybe::new()
}

fn main() {
  if none_value().is_some() {
    panic("none_value")
  }
  if err_value().is_ok() {
    panic("err_value")
  }
  if err_tuple().is_ok() {
    panic("err_tuple")
  }
  if try_value(Maybe::new()).is_some() {
    panic("try_value")
  }
  let values: [__T__] = []
  if generic_none(values).is_some() {
    panic("generic_none")
  }
}
`, imports), "__T__", typ)
	mainPath := filepath.Join(projectDir, "main.ard")
	writeTestFile(t, mainPath, source)
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v\nsource:\n%s", err, source)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	err = RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo)
	generated, _ := os.ReadFile(filepath.Join(projectDir, "ard-out", "go", "run", "main.go"))
	if err != nil {
		t.Fatalf("RunProgram error = %v\ngenerated:\n%s", err, generated)
	}
	return string(generated)
}

// TestZeroValueExprFallsBackToNewForUnclassifiedTypes pins the fallback that
// keeps zero values sound when the backend cannot tell a type's Go kind: a
// foreign type without a recorded struct, map, slice or underlying shape, or a
// kind with no literal form, is written *new(T), which is valid for every Go
// type, rather than a composite literal that Go rejects for interfaces,
// functions, channels and basic types.
func TestZeroValueExprFallsBackToNewForUnclassifiedTypes(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeForeignType, ForeignTarget: "go", ForeignNamespace: "example.com/pkg", ForeignQualifier: "pkg", ForeignSymbol: "Opaque"},
		{ID: 2, Kind: air.TypeForeignType, ForeignTarget: "go", ForeignNamespace: "example.com/pkg", ForeignQualifier: "pkg", ForeignSymbol: "Record", ForeignStruct: true},
		{ID: 3, Kind: air.TypeParam, Name: "T"},
	}}
	l := &lowerer{program: program, currentImports: map[string]string{}}
	for typeID, want := range map[air.TypeID]string{1: "*new(pkg.Opaque)", 2: "pkg.Record{}", 3: "*new(T)"} {
		zero, err := l.zeroValueExpr(typeID)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := format.Node(&out, token.NewFileSet(), zero); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != want {
			t.Errorf("zero value of type %d = %s, want %s", typeID, got, want)
		}
	}
}
