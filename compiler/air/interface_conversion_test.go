package air

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLowerPreservesGenericGoInterfaceWidening(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "ard.toml"), []byte("name = \"probe\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.com/probe\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffi := filepath.Join(project, "ffi")
	if err := os.MkdirAll(ffi, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffi, "ffi.go"), []byte(`package ffi

type Reader[T any] interface { Read() T }
type ReadWriter[T any] interface { Read() T; Write(T) }
type Box[T any] struct { Value T }
func (b Box[T]) Read() T { return b.Value }
type Getter[T any] interface { Read() T }
type StringReadWriter struct{}
func (StringReadWriter) Read() string { return "reader" }
func (StringReadWriter) Write(string) {}
func NewReadWriter() ReadWriter[string] { return StringReadWriter{} }
func NewBox() Box[string] { return Box[string]{Value: "box"} }
func NewBoxPointer() *Box[string] { return &Box[string]{Value: "pointer"} }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	lowerProjectSource(t, project, `use go:example.com/probe/ffi

fn narrow(value: ffi::ReadWriter<$T>) ffi::Reader<$T> {
  value
}

fn getter(value: ffi::Box<$T>) ffi::Getter<$T> {
  value
}

fn pointer_getter(value: mut ffi::Box<$T>) ffi::Getter<$T> {
  value
}

fn main() Str {
  let reader = narrow(ffi::NewReadWriter())
  let getter_value = getter(ffi::NewBox())
  let pointer_value = pointer_getter(ffi::NewBoxPointer())
  "{reader.Read()}:{getter_value.Read()}:{pointer_value.Read()}"
}`)
}

func TestLowerPreservesAnyConversionOwnership(t *testing.T) {
	program := lowerSource(t, `
struct User {}

fn consume(value: Any) {}

fn pass_reference(value: mut $T) {
  consume(value)
}

fn pass_value(value: $T) {
  consume(value)
}

fn main() {
  let user = User{}
  pass_reference(mut user)
  pass_value(user)
}
`)

	var findConversion func(*Expr) (*Expr, bool)
	findConversion = func(expr *Expr) (*Expr, bool) {
		if expr == nil {
			return nil, false
		}
		if expr.Kind == ExprInterfaceConversion {
			return expr, true
		}
		if conversion, ok := findConversion(expr.Target); ok {
			return conversion, true
		}
		for i := range expr.Args {
			if conversion, ok := findConversion(&expr.Args[i]); ok {
				return conversion, true
			}
		}
		return nil, false
	}

	modeFor := func(name string) InterfaceConversionMode {
		t.Helper()
		for _, fn := range program.Functions {
			if !strings.HasPrefix(fn.Name, name) {
				continue
			}
			if conversion, ok := findConversion(fn.Body.Result); ok {
				if program.Types[conversion.Type-1].Kind != TypeAny {
					t.Fatalf("%s conversion type = %v, want Any", fn.Name, program.Types[conversion.Type-1].Kind)
				}
				return conversion.InterfaceMode
			}
			for i := range fn.Body.Stmts {
				conversion, ok := findConversion(fn.Body.Stmts[i].Expr)
				if !ok {
					conversion, ok = findConversion(fn.Body.Stmts[i].Value)
				}
				if !ok {
					continue
				}
				if program.Types[conversion.Type-1].Kind != TypeAny {
					t.Fatalf("%s conversion type = %v, want Any", fn.Name, program.Types[conversion.Type-1].Kind)
				}
				return conversion.InterfaceMode
			}
		}
		t.Fatalf("interface conversion in function %s not found", name)
		return InterfaceValue
	}

	if got := modeFor("pass_reference"); got != InterfaceReference {
		t.Fatalf("reference mode = %v, want %v", got, InterfaceReference)
	}
	if got := modeFor("pass_value"); got != InterfaceValue {
		t.Fatalf("value mode = %v, want %v", got, InterfaceValue)
	}
}
