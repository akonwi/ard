package checker

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	gotoken "go/token"
	gotypes "go/types"
	"strings"
	"testing"

	"github.com/akonwi/ard/parse"
)

type fakeGoResolver struct {
	packages map[string]*GoPackage
	err      error
}

func (r fakeGoResolver) LoadPackage(importPath string) (*GoPackage, error) {
	if r.err != nil {
		return nil, r.err
	}
	pkg, ok := r.packages[importPath]
	if !ok {
		return nil, fmt.Errorf("missing fake package")
	}
	return pkg, nil
}

func goPackageFromSource(t *testing.T, importPath string, name string, source string) *GoPackage {
	t.Helper()
	fset := gotoken.NewFileSet()
	file, err := goparser.ParseFile(fset, name+".go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(gotypes.Config).Check(importPath, fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return goPackageFromTypes(importPath, name, pkg)
}

func goParam(kind GoValueKind, expr string) GoValueType {
	value := GoValueType{Kind: kind, Expr: expr}
	switch expr {
	case "int8", "uint8":
		value.Bits = 8
	case "int16", "uint16":
		value.Bits = 16
	case "int32", "uint32":
		value.Bits = 32
	case "int64", "uint64":
		value.Bits = 64
	case "float32":
		value.Bits = 32
	case "float64":
		value.Bits = 64
	}
	return value
}

func goNamed(kind GoValueKind, expr string, importPath string, name string) GoValueType {
	return GoValueType{Kind: kind, Expr: expr, Named: true, ImportPath: importPath, Package: importPath, Name: name}
}

func TestDirectGoStaticFunctionCallInfersResultReturn(t *testing.T) {
	result := parse.Parse([]byte(`use go:strconv
fn parse(value: Str) Int!Str { strconv::Atoi(value) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"strconv": {ImportPath: "strconv", Name: "strconv", Functions: map[string]GoFunction{"Atoi": {Name: "Atoi", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueInt, "int"), goParam(GoValueError, "error")}}}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoPackageConstantResolvesAsScalarValue(t *testing.T) {
	result := parse.Parse([]byte(`use go:os
fn flags() Int { os::O_WRONLY }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"os": {ImportPath: "os", Name: "os", Constants: map[string]GoConstant{"O_WRONLY": {Name: "O_WRONLY", Type: goParam(GoValueInt, "int"), IntValue: 1, HasIntValue: true}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoPackageConstantRejectsUnsupportedValue(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/complex as complex
fn bad() Float { complex::Value }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/complex": {ImportPath: "example.com/complex", Name: "complex", Constants: map[string]GoConstant{"Value": {Name: "Value", Type: GoValueType{Kind: GoValueOther, Expr: "untyped complex"}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected unsupported constant diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Go constant complex.Value has unsupported type untyped complex") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoPackageVariableResolvesAsValue(t *testing.T) {
	result := parse.Parse([]byte(`use go:os
fn args() [Str] { os::Args }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	str := goParam(GoValueString, "string")
	args := GoValueType{Kind: GoValueSlice, Expr: "[]string", Elem: &str}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"os": {ImportPath: "os", Name: "os", Variables: map[string]GoVariable{"Args": {Name: "Args", Type: args}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoPackageVariableSupportsInstanceMethods(t *testing.T) {
	result := parse.Parse([]byte(`use go:encoding/base64 as base64
fn encode(bytes: [Byte]) Str { base64::StdEncoding.EncodeToString(bytes) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	byteType := goParam(GoValueUint, "uint8")
	byteSlice := GoValueType{Kind: GoValueSlice, Expr: "[]byte", Elem: &byteType}
	encoding := goNamed(GoValueOther, "base64.Encoding", "encoding/base64", "Encoding")
	ptrEncoding := GoValueType{Kind: GoValuePointer, Expr: "*base64.Encoding", Elem: &encoding}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"encoding/base64": {
			ImportPath: "encoding/base64",
			Name:       "base64",
			Variables:  map[string]GoVariable{"StdEncoding": {Name: "StdEncoding", Type: ptrEncoding}},
			Types: map[string]GoType{"Encoding": {Name: "Encoding", Methods: map[string]GoMethod{
				"EncodeToString": {Name: "EncodeToString", Signature: GoSignature{Receiver: &ptrEncoding, Params: []GoValueType{byteSlice}, Results: []GoValueType{goParam(GoValueString, "string")}}},
			}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoFunctionCallAcceptsConcreteTypeImplementingInterface(t *testing.T) {
	result := parse.Parse([]byte(`use go:io
use go:strings
fn read_all() [Byte]!Str { io::ReadAll(strings::NewReader("hello")) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	byteType := goParam(GoValueUint, "uint8")
	byteSlice := GoValueType{Kind: GoValueSlice, Expr: "[]byte", Elem: &byteType}
	reader := goNamed(GoValueOther, "io.Reader", "io", "Reader")
	stringsReader := goNamed(GoValueOther, "strings.Reader", "strings", "Reader")
	ptrStringsReader := GoValueType{Kind: GoValuePointer, Expr: "*strings.Reader", Elem: &stringsReader}
	readMethod := GoMethod{Name: "Read", Signature: GoSignature{Params: []GoValueType{byteSlice}, Results: []GoValueType{goParam(GoValueInt, "int"), goParam(GoValueError, "error")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"io": {
			ImportPath: "io",
			Name:       "io",
			Functions:  map[string]GoFunction{"ReadAll": {Name: "ReadAll", Signature: GoSignature{Params: []GoValueType{reader}, Results: []GoValueType{byteSlice, goParam(GoValueError, "error")}}}},
			Types:      map[string]GoType{"Reader": {Name: "Reader", Interface: true, Methods: map[string]GoMethod{"Read": readMethod}}},
		},
		"strings": {
			ImportPath: "strings",
			Name:       "strings",
			Functions:  map[string]GoFunction{"NewReader": {Name: "NewReader", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{ptrStringsReader}}}},
			Types:      map[string]GoType{"Reader": {Name: "Reader", PointerMethods: map[string]GoMethod{"Read": readMethod}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoInterfaceTypeAcceptsConcreteImplementerInArdCall(t *testing.T) {
	result := parse.Parse([]byte(`use go:io
use go:strings
fn takes_reader(r: io::Reader) Int { 1 }
fn main() Int { takes_reader(strings::NewReader("hello")) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	byteType := goParam(GoValueUint, "uint8")
	byteSlice := GoValueType{Kind: GoValueSlice, Expr: "[]byte", Elem: &byteType}
	stringsReader := goNamed(GoValueOther, "strings.Reader", "strings", "Reader")
	ptrStringsReader := GoValueType{Kind: GoValuePointer, Expr: "*strings.Reader", Elem: &stringsReader}
	readMethod := GoMethod{Name: "Read", Signature: GoSignature{Params: []GoValueType{byteSlice}, Results: []GoValueType{goParam(GoValueInt, "int"), goParam(GoValueError, "error")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"io": {
			ImportPath: "io",
			Name:       "io",
			Types:      map[string]GoType{"Reader": {Name: "Reader", Interface: true, Methods: map[string]GoMethod{"Read": readMethod}}},
		},
		"strings": {
			ImportPath: "strings",
			Name:       "strings",
			Functions:  map[string]GoFunction{"NewReader": {Name: "NewReader", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{ptrStringsReader}}}},
			Types:      map[string]GoType{"Reader": {Name: "Reader", PointerMethods: map[string]GoMethod{"Read": readMethod}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoInterfaceTypeAcceptsArdStructImplementer(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/iface as iface

struct Handler { prefix: Str }

impl Handler {
  fn Handle(value: Str) Int { value.size() }
}

fn main() Int { iface::Use(Handler{prefix: "ok"}) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	handler := goNamed(GoValueOther, "iface.Handler", "example.com/iface", "Handler")
	handleMethod := GoMethod{Name: "Handle", Signature: GoSignature{Receiver: &handler, Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueInt, "int")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/iface": {
			ImportPath: "example.com/iface",
			Name:       "iface",
			Functions:  map[string]GoFunction{"Use": {Name: "Use", Signature: GoSignature{Params: []GoValueType{handler}, Results: []GoValueType{goParam(GoValueInt, "int")}}}},
			Types:      map[string]GoType{"Handler": {Name: "Handler", Interface: true, Methods: map[string]GoMethod{"Handle": handleMethod}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoInterfaceTypeAcceptsArdStructImplementerInArdCall(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/iface as iface

struct Handler { prefix: Str }

impl Handler {
  fn Handle(value: Str) Int { value.size() }
}

fn apply(handler: iface::Handler) Int { 1 }
fn main() Int { apply(Handler{prefix: "ok"}) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	handler := goNamed(GoValueOther, "iface.Handler", "example.com/iface", "Handler")
	handleMethod := GoMethod{Name: "Handle", Signature: GoSignature{Receiver: &handler, Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueInt, "int")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/iface": {
			ImportPath: "example.com/iface",
			Name:       "iface",
			Types:      map[string]GoType{"Handler": {Name: "Handler", Interface: true, Methods: map[string]GoMethod{"Handle": handleMethod}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoInterfaceTypeRejectsArdStructWhenMethodWrappersCollide(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/iface as iface

struct Handler { prefix: Str }

impl Handler {
  fn Handle(value: Str) Int { value.size() }
  fn Handle_(value: Str) Int { value.size() }
}

fn main() Int { iface::Use(Handler{prefix: "no"}) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	handler := goNamed(GoValueOther, "iface.Handler", "example.com/iface", "Handler")
	handleMethod := GoMethod{Name: "Handle", Signature: GoSignature{Receiver: &handler, Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueInt, "int")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/iface": {
			ImportPath: "example.com/iface",
			Name:       "iface",
			Functions:  map[string]GoFunction{"Use": {Name: "Use", Signature: GoSignature{Params: []GoValueType{handler}, Results: []GoValueType{goParam(GoValueInt, "int")}}}},
			Types:      map[string]GoType{"Handler": {Name: "Handler", Interface: true, Methods: map[string]GoMethod{"Handle": handleMethod}}},
		},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected method collision diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Ard type Handler is not compatible with Go named type iface.Handler") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoInterfaceTypeRejectsArdStructForSealedInterface(t *testing.T) {
	pkg := goPackageFromSource(t, "example.com/sealedard", "sealedard", `package sealedard

type Handler interface { Handle(string) int; seal() }
func Use(Handler) int { return 0 }
`)
	result := parse.Parse([]byte(`use go:example.com/sealedard as sealedard

struct Handler { prefix: Str }

impl Handler {
  fn Handle(value: Str) Int { value.size() }
}

fn main() Int { sealedard::Use(Handler{prefix: "no"}) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected sealed interface diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Ard type Handler is not compatible with Go named type sealedard.Handler") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoInterfaceTypeRejectsArdStructNonImplementer(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/iface as iface

struct Handler { prefix: Str }

impl Handler {
  fn Wrong(value: Str) Int { value.size() }
}

fn main() Int { iface::Use(Handler{prefix: "no"}) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	handler := goNamed(GoValueOther, "iface.Handler", "example.com/iface", "Handler")
	handleMethod := GoMethod{Name: "Handle", Signature: GoSignature{Receiver: &handler, Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueInt, "int")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/iface": {
			ImportPath: "example.com/iface",
			Name:       "iface",
			Functions:  map[string]GoFunction{"Use": {Name: "Use", Signature: GoSignature{Params: []GoValueType{handler}, Results: []GoValueType{goParam(GoValueInt, "int")}}}},
			Types:      map[string]GoType{"Handler": {Name: "Handler", Interface: true, Methods: map[string]GoMethod{"Handle": handleMethod}}},
		},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected non-implementer diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Ard type Handler is not compatible with Go named type iface.Handler") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoInterfaceTypeRejectsConcreteNonImplementerInArdCall(t *testing.T) {
	result := parse.Parse([]byte(`use go:io
use go:strings
fn takes_writer(w: io::Writer) Int { 1 }
fn main() Int { takes_writer(strings::NewReader("hello")) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	byteType := goParam(GoValueUint, "uint8")
	byteSlice := GoValueType{Kind: GoValueSlice, Expr: "[]byte", Elem: &byteType}
	stringsReader := goNamed(GoValueOther, "strings.Reader", "strings", "Reader")
	ptrStringsReader := GoValueType{Kind: GoValuePointer, Expr: "*strings.Reader", Elem: &stringsReader}
	readMethod := GoMethod{Name: "Read", Signature: GoSignature{Params: []GoValueType{byteSlice}, Results: []GoValueType{goParam(GoValueInt, "int"), goParam(GoValueError, "error")}}}
	writeMethod := GoMethod{Name: "Write", Signature: GoSignature{Params: []GoValueType{byteSlice}, Results: []GoValueType{goParam(GoValueInt, "int"), goParam(GoValueError, "error")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"io": {
			ImportPath: "io",
			Name:       "io",
			Types:      map[string]GoType{"Writer": {Name: "Writer", Interface: true, Methods: map[string]GoMethod{"Write": writeMethod}}},
		},
		"strings": {
			ImportPath: "strings",
			Name:       "strings",
			Functions:  map[string]GoFunction{"NewReader": {Name: "NewReader", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{ptrStringsReader}}}},
			Types:      map[string]GoType{"Reader": {Name: "Reader", PointerMethods: map[string]GoMethod{"Read": readMethod}}},
		},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected non-implementer diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Expected io::Writer, got mut strings::Reader") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoInterfaceReceiverAcceptsLargerInterface(t *testing.T) {
	result := parse.Parse([]byte(`use go:net/http as http
fn close_body(resp: mut http::Response) Void!Str { resp.Body.Close() }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	closer := goNamed(GoValueOther, "io.Closer", "io", "Closer")
	readCloser := goNamed(GoValueOther, "io.ReadCloser", "io", "ReadCloser")
	closeMethod := GoMethod{Name: "Close", Signature: GoSignature{Receiver: &closer, Results: []GoValueType{goParam(GoValueError, "error")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"net/http": {
			ImportPath: "net/http",
			Name:       "http",
			Types: map[string]GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]GoField{
				"Body": {Name: "Body", Type: readCloser},
			}}},
		},
		"io": {
			ImportPath: "io",
			Name:       "io",
			Types: map[string]GoType{
				"Closer":     {Name: "Closer", Interface: true, Methods: map[string]GoMethod{"Close": closeMethod}},
				"ReadCloser": {Name: "ReadCloser", Interface: true, Methods: map[string]GoMethod{"Close": closeMethod}},
			},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoInterfaceTypeRejectsMutableReferenceToInterface(t *testing.T) {
	pkg := goPackageFromSource(t, "example.com/iface", "iface", `package iface

type Reader interface { Read([]byte) (int, error) }
`)
	result := parse.Parse([]byte(`use go:example.com/iface as iface
fn takes_reader(r: iface::Reader) Int { 1 }
fn wrap(r: mut iface::Reader) Int { takes_reader(r) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected pointer-to-interface diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Expected iface::Reader, got mut iface::Reader") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoFunctionCallRejectsSliceOfConcreteForSliceOfInterface(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/io as io
use go:strings
fn main() { io::UseReaders([strings::NewReader("hello")]) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	byteType := goParam(GoValueUint, "uint8")
	byteSlice := GoValueType{Kind: GoValueSlice, Expr: "[]byte", Elem: &byteType}
	reader := goNamed(GoValueOther, "io.Reader", "example.com/io", "Reader")
	readers := GoValueType{Kind: GoValueSlice, Expr: "[]io.Reader", Elem: &reader}
	stringsReader := goNamed(GoValueOther, "strings.Reader", "strings", "Reader")
	ptrStringsReader := GoValueType{Kind: GoValuePointer, Expr: "*strings.Reader", Elem: &stringsReader}
	readMethod := GoMethod{Name: "Read", Signature: GoSignature{Params: []GoValueType{byteSlice}, Results: []GoValueType{goParam(GoValueInt, "int"), goParam(GoValueError, "error")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/io": {
			ImportPath: "example.com/io",
			Name:       "io",
			Functions:  map[string]GoFunction{"UseReaders": {Name: "UseReaders", Signature: GoSignature{Params: []GoValueType{readers}}}},
			Types:      map[string]GoType{"Reader": {Name: "Reader", Interface: true, Methods: map[string]GoMethod{"Read": readMethod}}},
		},
		"strings": {
			ImportPath: "strings",
			Name:       "strings",
			Functions:  map[string]GoFunction{"NewReader": {Name: "NewReader", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{ptrStringsReader}}}},
			Types:      map[string]GoType{"Reader": {Name: "Reader", PointerMethods: map[string]GoMethod{"Read": readMethod}}},
		},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected slice invariance diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "list element") || !strings.Contains(got, "Go named type io.Reader") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoFunctionCallRejectsMapOfConcreteForMapOfInterface(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/io as io
use go:strings
fn main() { io::UseReaderMap(["a": strings::NewReader("hello")]) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	byteType := goParam(GoValueUint, "uint8")
	byteSlice := GoValueType{Kind: GoValueSlice, Expr: "[]byte", Elem: &byteType}
	strType := goParam(GoValueString, "string")
	reader := goNamed(GoValueOther, "io.Reader", "example.com/io", "Reader")
	readerMap := GoValueType{Kind: GoValueMap, Expr: "map[string]io.Reader", Key: &strType, Value: &reader}
	stringsReader := goNamed(GoValueOther, "strings.Reader", "strings", "Reader")
	ptrStringsReader := GoValueType{Kind: GoValuePointer, Expr: "*strings.Reader", Elem: &stringsReader}
	readMethod := GoMethod{Name: "Read", Signature: GoSignature{Params: []GoValueType{byteSlice}, Results: []GoValueType{goParam(GoValueInt, "int"), goParam(GoValueError, "error")}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/io": {
			ImportPath: "example.com/io",
			Name:       "io",
			Functions:  map[string]GoFunction{"UseReaderMap": {Name: "UseReaderMap", Signature: GoSignature{Params: []GoValueType{readerMap}}}},
			Types:      map[string]GoType{"Reader": {Name: "Reader", Interface: true, Methods: map[string]GoMethod{"Read": readMethod}}},
		},
		"strings": {
			ImportPath: "strings",
			Name:       "strings",
			Functions:  map[string]GoFunction{"NewReader": {Name: "NewReader", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{ptrStringsReader}}}},
			Types:      map[string]GoType{"Reader": {Name: "Reader", PointerMethods: map[string]GoMethod{"Read": readMethod}}},
		},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected map invariance diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "map value") || !strings.Contains(got, "Go named type io.Reader") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoInterfaceAssignabilityUsesFullGoMethodSet(t *testing.T) {
	pkg := goPackageFromSource(t, "example.com/sealed", "sealed", `package sealed

type Sealed interface { seal(); Read([]byte) (int, error) }
type Reader struct{}
func NewReader() *Reader { return nil }
func Use(Sealed) {}
func (*Reader) Read([]byte) (int, error) { return 0, nil }
`)
	result := parse.Parse([]byte(`use go:example.com/sealed as sealed
fn main() { sealed::Use(sealed::NewReader()) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected sealed-interface diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Go named type sealed.Sealed") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoInterfaceAssignabilityAllowsPromotedImplementer(t *testing.T) {
	pkg := goPackageFromSource(t, "example.com/promotediface", "promotediface", `package promotediface

type Reader interface { Read([]byte) (int, error) }
type Base struct{}
func (*Base) Read([]byte) (int, error) { return 0, nil }
type Wrapper struct{ *Base }
func NewWrapper() *Wrapper { return nil }
func Use(Reader) {}
`)
	result := parse.Parse([]byte(`use go:example.com/promotediface as promotediface
fn main() { promotediface::Use(promotediface::NewWrapper()) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructFieldRead(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/http as http
fn status() Int { http::DefaultResponse.StatusCode }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	response := goNamed(GoValueOther, "http.Response", "example.com/http", "Response")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/http": {
			ImportPath: "example.com/http",
			Name:       "http",
			Variables:  map[string]GoVariable{"DefaultResponse": {Name: "DefaultResponse", Type: response}},
			Types: map[string]GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]GoField{
				"StatusCode": {Name: "StatusCode", Type: goParam(GoValueInt, "int")},
			}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructFieldReadLoadsNestedFieldPackage(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/http as http
fn path(req: mut http::Request) Str { req.URL.Path }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	url := goNamed(GoValueOther, "url.URL", "example.com/url", "URL")
	ptrURL := GoValueType{Kind: GoValuePointer, Expr: "*url.URL", Elem: &url}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/http": {
			ImportPath: "example.com/http",
			Name:       "http",
			Types: map[string]GoType{"Request": {Name: "Request", Fields: map[string]GoField{
				"URL": {Name: "URL", Type: ptrURL},
			}}},
		},
		"example.com/url": {
			ImportPath: "example.com/url",
			Name:       "url",
			Types: map[string]GoType{"URL": {Name: "URL", Fields: map[string]GoField{
				"Path": {Name: "Path", Type: goParam(GoValueString, "string")},
			}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructFieldWriteAllowsMutableLocal(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/http as http
fn status() Int {
  mut res = http::DefaultResponse
  res.StatusCode = 201
  res.StatusCode
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	response := goNamed(GoValueOther, "http.Response", "example.com/http", "Response")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/http": {
			ImportPath: "example.com/http",
			Name:       "http",
			Variables:  map[string]GoVariable{"DefaultResponse": {Name: "DefaultResponse", Type: response}},
			Types: map[string]GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]GoField{
				"StatusCode": {Name: "StatusCode", Type: goParam(GoValueInt, "int")},
			}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructFieldWriteRejectsImmutableSubject(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/http as http
fn status() {
  let res = http::DefaultResponse
  res.StatusCode = 201
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	response := goNamed(GoValueOther, "http.Response", "example.com/http", "Response")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/http": {
			ImportPath: "example.com/http",
			Name:       "http",
			Variables:  map[string]GoVariable{"DefaultResponse": {Name: "DefaultResponse", Type: response}},
			Types: map[string]GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]GoField{
				"StatusCode": {Name: "StatusCode", Type: goParam(GoValueInt, "int")},
			}}},
		},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected immutable direct Go field diagnostic")
	}
	if got := c.Diagnostics()[0].Message; got != "Immutable: res.StatusCode" {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStructFieldWriteChecksValueCompatibility(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/http as http
fn status() {
  mut res = http::DefaultResponse
  res.StatusCode = "ok"
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	response := goNamed(GoValueOther, "http.Response", "example.com/http", "Response")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/http": {
			ImportPath: "example.com/http",
			Name:       "http",
			Variables:  map[string]GoVariable{"DefaultResponse": {Name: "DefaultResponse", Type: response}},
			Types: map[string]GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]GoField{
				"StatusCode": {Name: "StatusCode", Type: goParam(GoValueInt, "int")},
			}}},
		},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected direct Go field assignment type diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "field StatusCode: Ard type Str is not compatible with Go type int") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStructFieldWriteAllowsNestedPointerField(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/http as http
fn set_path(req: mut http::Request) {
  req.URL.Path = "/ready"
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	url := goNamed(GoValueOther, "url.URL", "example.com/url", "URL")
	ptrURL := GoValueType{Kind: GoValuePointer, Expr: "*url.URL", Elem: &url}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/http": {
			ImportPath: "example.com/http",
			Name:       "http",
			Types: map[string]GoType{"Request": {Name: "Request", Fields: map[string]GoField{
				"URL": {Name: "URL", Type: ptrURL},
			}}},
		},
		"example.com/url": {
			ImportPath: "example.com/url",
			Name:       "url",
			Types: map[string]GoType{"URL": {Name: "URL", Fields: map[string]GoField{
				"Path": {Name: "Path", Type: goParam(GoValueString, "string")},
			}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructFieldWriteAllowsNestedValueFieldOnMutableRoot(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/nested as nested
fn set_count() {
  mut outer = nested::DefaultOuter
  outer.Inner.Count = 7
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	inner := goNamed(GoValueOther, "nested.Inner", "example.com/nested", "Inner")
	outer := goNamed(GoValueOther, "nested.Outer", "example.com/nested", "Outer")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/nested": {
			ImportPath: "example.com/nested",
			Name:       "nested",
			Variables:  map[string]GoVariable{"DefaultOuter": {Name: "DefaultOuter", Type: outer}},
			Types: map[string]GoType{
				"Outer": {Name: "Outer", Fields: map[string]GoField{
					"Inner": {Name: "Inner", Type: inner},
				}},
				"Inner": {Name: "Inner", Fields: map[string]GoField{
					"Count": {Name: "Count", Type: goParam(GoValueInt, "int")},
				}},
			},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructFieldWriteRejectsIntForClosedEnumField(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/status as status
fn set_state() {
  mut box = status::DefaultBox
  box.State = 999
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	state := goNamed(GoValueInt, "status.State", "example.com/status", "State")
	box := goNamed(GoValueOther, "status.Box", "example.com/status", "Box")
	pkg := fakeEnumLikePackage("example.com/status", "status", state, []GoConstant{
		goEnumConstant("StateReady", state, 0),
		goEnumConstant("StateDone", state, 1),
	})
	pkg.Variables = map[string]GoVariable{"DefaultBox": {Name: "DefaultBox", Type: box}}
	pkg.Types["Box"] = GoType{Name: "Box", Fields: map[string]GoField{
		"State": {Name: "State", Type: state},
	}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected closed enum field assignment diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Type mismatch: Expected State, got Int") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStructFieldWriteRejectsExternValueForClosedEnumField(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/outer as outer
fn set_state() {
  let current = outer::Current()
  mut box = outer::DefaultBox
  box.State = current
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	state := goNamed(GoValueInt, "status.State", "example.com/status", "State")
	box := goNamed(GoValueOther, "outer.Box", "example.com/outer", "Box")
	statusPkg := fakeEnumLikePackage("example.com/status", "status", state, []GoConstant{
		goEnumConstant("StateReady", state, 0),
		goEnumConstant("StateDone", state, 1),
	})
	outerPkg := &GoPackage{
		ImportPath: "example.com/outer",
		Name:       "outer",
		Functions:  map[string]GoFunction{"Current": {Name: "Current", Signature: GoSignature{Results: []GoValueType{state}}}},
		Variables:  map[string]GoVariable{"DefaultBox": {Name: "DefaultBox", Type: box}},
		Types: map[string]GoType{"Box": {Name: "Box", Fields: map[string]GoField{
			"State": {Name: "State", Type: state},
		}}},
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		statusPkg.ImportPath: statusPkg,
		outerPkg.ImportPath:  outerPkg,
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected closed enum field direct-Go-value diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Type mismatch: Expected State, got example.com/status::State") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStructFieldWriteUsesClosedEnumExpectedTypeForDirectGoCall(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/outer as outer
fn set_state() {
  mut box = outer::DefaultBox
  box.State = outer::Current()
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	state := goNamed(GoValueInt, "status.State", "example.com/status", "State")
	box := goNamed(GoValueOther, "outer.Box", "example.com/outer", "Box")
	statusPkg := fakeEnumLikePackage("example.com/status", "status", state, []GoConstant{
		goEnumConstant("StateReady", state, 0),
		goEnumConstant("StateDone", state, 1),
	})
	outerPkg := &GoPackage{
		ImportPath: "example.com/outer",
		Name:       "outer",
		Functions:  map[string]GoFunction{"Current": {Name: "Current", Signature: GoSignature{Results: []GoValueType{state}}}},
		Variables:  map[string]GoVariable{"DefaultBox": {Name: "DefaultBox", Type: box}},
		Types: map[string]GoType{"Box": {Name: "Box", Fields: map[string]GoField{
			"State": {Name: "State", Type: state},
		}}},
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		statusPkg.ImportPath: statusPkg,
		outerPkg.ImportPath:  outerPkg,
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructFieldWriteAllowsClosedEnumValue(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/status as status
fn set_state() {
  mut box = status::DefaultBox
  box.State = status::StateReady
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	state := goNamed(GoValueInt, "status.State", "example.com/status", "State")
	box := goNamed(GoValueOther, "status.Box", "example.com/status", "Box")
	pkg := fakeEnumLikePackage("example.com/status", "status", state, []GoConstant{
		goEnumConstant("StateReady", state, 0),
		goEnumConstant("StateDone", state, 1),
	})
	pkg.Variables = map[string]GoVariable{"DefaultBox": {Name: "DefaultBox", Type: box}}
	pkg.Types["Box"] = GoType{Name: "Box", Fields: map[string]GoField{
		"State": {Name: "State", Type: state},
	}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructFieldWriteAllowsNarrowScalarConversion(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/narrow as narrow
fn set_code() {
  mut res = narrow::DefaultResponse
  res.Code = 7
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	response := goNamed(GoValueOther, "narrow.Response", "example.com/narrow", "Response")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/narrow": {
			ImportPath: "example.com/narrow",
			Name:       "narrow",
			Variables:  map[string]GoVariable{"DefaultResponse": {Name: "DefaultResponse", Type: response}},
			Types: map[string]GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]GoField{
				"Code": {Name: "Code", Type: goParam(GoValueInt, "int8")},
			}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructConstructionAllowsKeyedLiteral(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/image as image
fn sum() Int {
  let p = image::Point{X: 10, Y: 20}
  p.X + p.Y
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	point := GoType{Name: "Point", Struct: true, Fields: map[string]GoField{
		"X": {Name: "X", Type: goParam(GoValueInt, "int")},
		"Y": {Name: "Y", Type: goParam(GoValueInt, "int")},
	}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/image": {ImportPath: "example.com/image", Name: "image", Types: map[string]GoType{"Point": point}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructConstructionAllowsNestedLiteral(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/image as image
fn width() Int {
  let rect = image::Rectangle{
    Min: image::Point{X: 1, Y: 2},
    Max: image::Point{X: 5, Y: 6},
  }
  rect.Max.X - rect.Min.X
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	pointType := goNamed(GoValueOther, "image.Point", "example.com/image", "Point")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/image": {
			ImportPath: "example.com/image",
			Name:       "image",
			Types: map[string]GoType{
				"Point": {Name: "Point", Struct: true, Fields: map[string]GoField{
					"X": {Name: "X", Type: goParam(GoValueInt, "int")},
					"Y": {Name: "Y", Type: goParam(GoValueInt, "int")},
				}},
				"Rectangle": {Name: "Rectangle", Struct: true, Fields: map[string]GoField{
					"Min": {Name: "Min", Type: pointType},
					"Max": {Name: "Max", Type: pointType},
				}},
			},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructConstructionRequiresAllVisibleFields(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/image as image
fn point() image::Point {
  image::Point{X: 10}
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/image": {ImportPath: "example.com/image", Name: "image", Types: map[string]GoType{"Point": {Name: "Point", Struct: true, Fields: map[string]GoField{
			"X": {Name: "X", Type: goParam(GoValueInt, "int")},
			"Y": {Name: "Y", Type: goParam(GoValueInt, "int")},
		}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected missing direct Go struct field diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Missing Go field: Y") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStructConstructionRejectsUnknownField(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/image as image
fn point() image::Point {
  image::Point{X: 10, Y: 20, Z: 30}
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/image": {ImportPath: "example.com/image", Name: "image", Types: map[string]GoType{"Point": {Name: "Point", Struct: true, Fields: map[string]GoField{
			"X": {Name: "X", Type: goParam(GoValueInt, "int")},
			"Y": {Name: "Y", Type: goParam(GoValueInt, "int")},
		}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected unknown direct Go struct field diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, `Go type "Point" in package "example.com/image" has no exported field "Z"`) {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStructConstructionRejectsUnsupportedVisibleField(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/http as http
fn response() http::Response {
  http::Response{StatusCode: 200}
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/http": {ImportPath: "example.com/http", Name: "http", Types: map[string]GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]GoField{
			"StatusCode": {Name: "StatusCode", Type: goParam(GoValueInt, "int")},
			"Callback":   {Name: "Callback", Type: GoValueType{Kind: GoValueOther, Expr: "func()"}},
		}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected unsupported direct Go struct field diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Go field http.Response.Callback has unsupported type func()") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStructConstructionAllowsNarrowScalarConversion(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/narrow as narrow
fn response() narrow::Response {
  narrow::Response{Code: 7}
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/narrow": {ImportPath: "example.com/narrow", Name: "narrow", Types: map[string]GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]GoField{
			"Code": {Name: "Code", Type: goParam(GoValueInt, "int8")},
		}}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStructConstructionRejectsGenericGoStruct(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/generic as generic
fn box() generic::Box {
  generic::Box{Value: 1}
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/generic": {ImportPath: "example.com/generic", Name: "generic", Types: map[string]GoType{"Box": {Name: "Box", Struct: true, TypeParams: 1, Fields: map[string]GoField{
			"Value": {Name: "Value", Type: goParam(GoValueInt, "int")},
		}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected generic direct Go struct diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, `Go generic struct type "Box" in package "example.com/generic" cannot be constructed directly`) {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStructConstructionRejectsGenericGoFieldType(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/generic as generic
fn holder() generic::Holder {
  generic::Holder{Box: generic::IntBox}
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	box := goNamed(GoValueOther, "generic.Box", "example.com/generic", "Box")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/generic": {
			ImportPath: "example.com/generic",
			Name:       "generic",
			Variables:  map[string]GoVariable{"IntBox": {Name: "IntBox", Type: box}},
			Types: map[string]GoType{
				"Box": {Name: "Box", Struct: true, TypeParams: 1, Fields: map[string]GoField{
					"Value": {Name: "Value", Type: goParam(GoValueAny, "any")},
				}},
				"Holder": {Name: "Holder", Struct: true, Fields: map[string]GoField{
					"Box": {Name: "Box", Type: box},
				}},
			},
		},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected generic direct Go field diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Go field generic.Holder.Box has unsupported type generic.Box") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStructFieldReadReportsUnsupportedFieldType(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/http as http
fn callback() { http::DefaultResponse.Callback }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	response := goNamed(GoValueOther, "http.Response", "example.com/http", "Response")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/http": {
			ImportPath: "example.com/http",
			Name:       "http",
			Variables:  map[string]GoVariable{"DefaultResponse": {Name: "DefaultResponse", Type: response}},
			Types: map[string]GoType{"Response": {Name: "Response", Struct: true, Fields: map[string]GoField{
				"Callback": {Name: "Callback", Type: GoValueType{Kind: GoValueOther, Expr: "func()"}},
			}}},
		},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected unsupported field diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Go field http.Response.Callback has unsupported type func()") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoPackageVariableRejectsUnsupportedType(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/unsupported as unsupported
fn bad() Int { unsupported::Callback }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/unsupported": {ImportPath: "example.com/unsupported", Name: "unsupported", Variables: map[string]GoVariable{"Callback": {Name: "Callback", Type: GoValueType{Kind: GoValueOther, Expr: "func()"}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected unsupported variable type diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Go variable unsupported.Callback has unsupported type func()") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoPackageVariableAssignmentIsRejected(t *testing.T) {
	result := parse.Parse([]byte(`use go:os
fn main() {
  os::Args = ["ard"]
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	str := goParam(GoValueString, "string")
	args := GoValueType{Kind: GoValueSlice, Expr: "[]string", Elem: &str}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"os": {ImportPath: "os", Name: "os", Variables: map[string]GoVariable{"Args": {Name: "Args", Type: args}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected read-only Go package variable diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Cannot assign to Go package variable os::Args") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternTypeEqualityUsesBindingBeforeDisplayName(t *testing.T) {
	left := &ExternType{Name_: "p::T", ExternalBinding: "go:example.com/a as p::T"}
	right := &ExternType{Name_: "p::T", ExternalBinding: "go:example.com/b as p::T"}
	if equalTypes(left, right) {
		t.Fatal("direct Go types with different import paths should not be equal")
	}
	alias := &ExternType{Name_: "other::T", ExternalBinding: "go:example.com/a as other::T"}
	if !equalTypes(left, alias) {
		t.Fatal("direct Go types with same import path and symbol should be equal despite aliases")
	}
}

func TestDirectGoCallReturnMatchesExplicitDirectGoType(t *testing.T) {
	result := parse.Parse([]byte(`use go:time
fn now() time::Time { time::Now() }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	timeType := goNamed(GoValueOther, "time.Time", "time", "Time")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"time": {ImportPath: "time", Name: "time", Functions: map[string]GoFunction{"Now": {Name: "Now", Signature: GoSignature{Results: []GoValueType{timeType}}}}, Types: map[string]GoType{"Time": {Name: "Time"}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoStaticMethodExpressionRejectsCoercedReceiver(t *testing.T) {
	result := parse.Parse([]byte(`use go:time
fn bad() Str { time::Duration::String(42) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	duration := goNamed(GoValueInt, "time.Duration", "time", "Duration")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"time": {ImportPath: "time", Name: "time", Types: map[string]GoType{"Duration": {Name: "Duration", Methods: map[string]GoMethod{"String": {Name: "String", Signature: GoSignature{Receiver: &duration, Results: []GoValueType{goParam(GoValueString, "string")}}}}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected receiver mismatch diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "receiver for time.Duration.String") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoStaticMethodExpressionCall(t *testing.T) {
	result := parse.Parse([]byte(`use go:time
fn format(value: time::Time) Str { time::Time::Format(value, "2006-01-02") }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	timeType := goNamed(GoValueOther, "time.Time", "time", "Time")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"time": {ImportPath: "time", Name: "time", Types: map[string]GoType{"Time": {Name: "Time", Methods: map[string]GoMethod{"Format": {Name: "Format", Signature: GoSignature{Receiver: &timeType, Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueString, "string")}}}}}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoInstanceMethodCall(t *testing.T) {
	result := parse.Parse([]byte(`use go:time
fn format() Str { time::Now().Format("2006-01-02") }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	timeType := goNamed(GoValueOther, "time.Time", "time", "Time")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"time": {ImportPath: "time", Name: "time", Functions: map[string]GoFunction{"Now": {Name: "Now", Signature: GoSignature{Results: []GoValueType{timeType}}}}, Types: map[string]GoType{"Time": {Name: "Time", Methods: map[string]GoMethod{"Format": {Name: "Format", Signature: GoSignature{Receiver: &timeType, Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueString, "string")}}}}}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoImportAliasMustBeUsableAsGoSelector(t *testing.T) {
	for _, alias := range []string{"go", "init"} {
		t.Run(alias, func(t *testing.T) {
			result := parse.Parse([]byte(`use go:math as `+alias), "main.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{}}})
			c.Check()
			if !c.HasErrors() {
				t.Fatal("expected invalid Go import alias diagnostic")
			}
			if got := c.Diagnostics()[0].Message; !strings.Contains(got, `Go import alias "`+alias+`" cannot be used as a Go selector`) {
				t.Fatalf("diagnostic = %q", got)
			}
		})
	}
}

func TestDirectGoImportsRequireUniqueAliases(t *testing.T) {
	result := parse.Parse([]byte(`use go:crypto/rand
use go:math/rand`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"crypto/rand": {ImportPath: "crypto/rand", Name: "rand"},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected duplicate import alias diagnostic")
	}
	if got := c.Diagnostics()[0]; got.Kind != Warn || !strings.Contains(got.Message, "Duplicate import: rand") {
		t.Fatalf("diagnostic = %#v, want duplicate rand warning", got)
	}
}

func TestDirectGoTypeReferenceDoesNotRequireExternType(t *testing.T) {
	result := parse.Parse([]byte(`use go:time
fn sleep(duration: time::Duration) { time::Sleep(duration) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"time": {
			ImportPath: "time",
			Name:       "time",
			Functions: map[string]GoFunction{"Sleep": {Name: "Sleep", Signature: GoSignature{Params: []GoValueType{
				goNamed(GoValueInt, "time.Duration", "time", "Duration"),
			}}}},
			Types: map[string]GoType{"Duration": {Name: "Duration"}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
	fn := c.Module().Program().Statements[0].Expr.(*FunctionDef)
	duration, ok := fn.Parameters[0].Type.(*ExternType)
	if !ok {
		t.Fatalf("param type = %#v, want ExternType", fn.Parameters[0].Type)
	}
	if duration.ExternalBinding != "go:time as time::Duration" {
		t.Fatalf("duration binding = %q", duration.ExternalBinding)
	}
}

func TestDirectGoEnumLikeConstantsResolveAsClosedEnum(t *testing.T) {
	result := parse.Parse([]byte(`use go:git.sr.ht/~rockorager/vaxis as vaxis
fn active(status: vaxis::AnimationStatus) Bool {
  match status {
    vaxis::AnimationIdle => false
    vaxis::AnimationForward => true
    vaxis::AnimationCompleted => false
  }
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	status := goNamed(GoValueInt, "vaxis.AnimationStatus", "git.sr.ht/~rockorager/vaxis", "AnimationStatus")
	pkg := fakeEnumLikePackage("git.sr.ht/~rockorager/vaxis", "vaxis", status, []GoConstant{
		goEnumConstant("AnimationIdle", status, 0),
		goEnumConstant("AnimationForward", status, 1),
		goEnumConstant("AnimationCompleted", status, 2),
	})
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
	fn := c.Module().Program().Statements[0].Expr.(*FunctionDef)
	if enum, ok := fn.Parameters[0].Type.(*Enum); !ok || enum.ExternalBinding != "go:git.sr.ht/~rockorager/vaxis as vaxis::AnimationStatus" || len(enum.Values) != 3 {
		t.Fatalf("param type = %#v, want direct Go enum-like AnimationStatus", fn.Parameters[0].Type)
	}
}

func TestOpenDirectGoEnumLikeConstantsRequireCatchAll(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/status as status
fn describe(value: status::State) Str {
  match value {
    status::StateReady => "ready"
    status::StateDone => "done"
  }
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	state := goNamed(GoValueInt, "status.State", "example.com/status", "State")
	pkg := fakeEnumLikePackage("example.com/status", "status", state, []GoConstant{
		goEnumConstant("StateReady", state, 0),
		goEnumConstant("StateDone", state, 1),
	})
	typ := pkg.Types["State"]
	typ.ClosedEnum = false
	pkg.Types["State"] = typ
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected open enum-like catch-all diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "requires a catch-all") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoEnumLikeConstantAliasesDuplicateMatchByValue(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/status as status
fn describe(value: status::State) Str {
  match value {
    status::StateReady => "ready"
    status::StateAlsoReady => "alias"
    _ => "other"
  }
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	state := goNamed(GoValueInt, "status.State", "example.com/status", "State")
	pkg := fakeEnumLikePackage("example.com/status", "status", state, []GoConstant{
		goEnumConstant("StateReady", state, 1),
		goEnumConstant("StateAlsoReady", state, 1),
		goEnumConstant("StateDone", state, 2),
	})
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected duplicate enum alias match diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "has same value as State::StateReady") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func fakeEnumLikePackage(importPath string, name string, typ GoValueType, constants []GoConstant) *GoPackage {
	pkg := &GoPackage{
		ImportPath: importPath,
		Name:       name,
		Functions:  map[string]GoFunction{},
		Types:      map[string]GoType{typ.Name: {Name: typ.Name, EnumConstants: constants, ClosedEnum: true}},
		Constants:  map[string]GoConstant{},
	}
	for _, constant := range constants {
		pkg.Constants[constant.Name] = constant
	}
	return pkg
}

func goEnumConstant(name string, typ GoValueType, value int) GoConstant {
	return GoConstant{Name: name, Type: typ, IntValue: value, HasIntValue: true}
}

// A `mut <direct-Go handle>` value (a Go pointer handle such as *sql.DB) can be
// stored in a `mut T` struct field and re-passed to a direct-Go pointer
// parameter: the field read deref's to the value type but auto-borrows back into
// `mut T` at the call site (ADR 0031).
func TestDirectGoMutableHandleStoredInFieldAndRepassed(t *testing.T) {
	result := parse.Parse([]byte(`use go:database/sql as sql
struct Handle { db: mut sql::DB }
fn store(c: mut sql::DB) Handle { Handle{db: c} }
fn run(h: Handle) Void!Str { sql::Close(h.db) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	db := goNamed(GoValueOther, "sql.DB", "database/sql", "DB")
	ptrDB := GoValueType{Kind: GoValuePointer, Expr: "*sql.DB", Elem: &db}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"database/sql": {ImportPath: "database/sql", Name: "sql",
			Types:     map[string]GoType{"DB": {Name: "DB"}},
			Functions: map[string]GoFunction{"Close": {Name: "Close", Signature: GoSignature{Params: []GoValueType{ptrDB}, Results: []GoValueType{goParam(GoValueError, "error")}}}},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected errors: %v", c.Diagnostics())
	}
}

// A Go method with a pointer receiver can be called directly on a stored
// `mut <handle>` field, including under `try`: the field read deref's to the
// value type but auto-borrows back into `mut T` for the receiver (ADR 0031).
func TestDirectGoInstanceMethodOnStoredHandleUnderTry(t *testing.T) {
	result := parse.Parse([]byte(`use go:database/sql as sql
struct Database { _db: mut sql::DB }
fn close(d: Database) Void!Str {
	try d._db.Close()
	Result::ok(())
}`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	db := goNamed(GoValueOther, "sql.DB", "database/sql", "DB")
	ptrDB := GoValueType{Kind: GoValuePointer, Expr: "*sql.DB", Elem: &db}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"database/sql": {ImportPath: "database/sql", Name: "sql", Types: map[string]GoType{
			"DB": {Name: "DB", Methods: map[string]GoMethod{"Close": {Name: "Close", Signature: GoSignature{Receiver: &ptrDB, Results: []GoValueType{goParam(GoValueError, "error")}}}}},
		}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected errors: %v", c.Diagnostics())
	}
}

func TestDirectGoCallUsesExpectedResultForWidthScalarReturn(t *testing.T) {
	result := parse.Parse([]byte(`use go:strconv
fn main() Int!Str { strconv::ParseInt("42", 10, 64) }`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"strconv": {ImportPath: "strconv", Name: "strconv", Functions: map[string]GoFunction{
			"ParseInt": {Name: "ParseInt", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string"), goParam(GoValueInt, "int"), goParam(GoValueInt, "int")}, Results: []GoValueType{goParam(GoValueInt, "int64"), goParam(GoValueError, "error")}}},
		}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestGoValueTypeHandlesRecursiveNamedTypes(t *testing.T) {
	pkg := gotypes.NewPackage("example.com/recursive", "recursive")
	obj := gotypes.NewTypeName(gotoken.NoPos, pkg, "Loop", nil)
	named := gotypes.NewNamed(obj, nil, nil)
	named.SetUnderlying(gotypes.NewSlice(named))

	value := goValueType(named)
	if !value.Named || value.Name != "Loop" || value.ImportPath != "example.com/recursive" {
		t.Fatalf("recursive named metadata = %#v", value)
	}
	if value.Kind != GoValueSlice || value.Elem == nil {
		t.Fatalf("recursive named kind = %#v, want slice with opaque element", value)
	}
	if !value.Elem.Named || value.Elem.Name != "Loop" || value.Elem.Kind != GoValueOther {
		t.Fatalf("recursive element = %#v, want opaque named Loop", value.Elem)
	}
}

func TestGoPackageFromTypesDiscoversScalarConstants(t *testing.T) {
	fset := gotoken.NewFileSet()
	file, err := goparser.ParseFile(fset, "consts.go", `package consts

const Flag int = 1
const Name = "ard"
const Enabled = true
const Ratio = 1.5
const TooComplex = 1 + 2i
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(gotypes.Config).Check("example.com/consts", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	goPkg := goPackageFromTypes("example.com/consts", "consts", pkg)
	if flag := goPkg.Constants["Flag"]; !flag.HasIntValue || flag.IntValue != 1 || flag.Type.Kind != GoValueInt {
		t.Fatalf("Flag = %#v", flag)
	}
	if name := goPkg.Constants["Name"]; !name.HasStringValue || name.StringValue != "ard" || name.Type.Kind != GoValueString {
		t.Fatalf("Name = %#v", name)
	}
	if enabled := goPkg.Constants["Enabled"]; !enabled.HasBoolValue || !enabled.BoolValue || enabled.Type.Kind != GoValueBool {
		t.Fatalf("Enabled = %#v", enabled)
	}
	if ratio := goPkg.Constants["Ratio"]; !ratio.HasFloatValue || ratio.FloatValue != 1.5 || ratio.Type.Kind != GoValueFloat {
		t.Fatalf("Ratio = %#v", ratio)
	}
	if complex := goPkg.Constants["TooComplex"]; complex.HasIntValue || complex.HasFloatValue || complex.HasBoolValue || complex.HasStringValue {
		t.Fatalf("TooComplex should not have a scalar Ard value: %#v", complex)
	}
}

func TestGoPackageFromTypesDiscoversExportedVariables(t *testing.T) {
	fset := gotoken.NewFileSet()
	file, err := goparser.ParseFile(fset, "vars.go", `package vars

var Args []string
var unexported int

type Encoding struct{}
var StdEncoding *Encoding
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(gotypes.Config).Check("example.com/vars", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	goPkg := goPackageFromTypes("example.com/vars", "vars", pkg)
	args, ok := goPkg.Variables["Args"]
	if !ok {
		t.Fatalf("exported Args variable missing: %#v", goPkg.Variables)
	}
	if args.Type.Kind != GoValueSlice || args.Type.Elem == nil || args.Type.Elem.Kind != GoValueString {
		t.Fatalf("Args type = %#v, want []string", args.Type)
	}
	std, ok := goPkg.Variables["StdEncoding"]
	if !ok {
		t.Fatalf("exported StdEncoding variable missing: %#v", goPkg.Variables)
	}
	if std.Type.Kind != GoValuePointer || std.Type.Elem == nil || !std.Type.Elem.Named || std.Type.Elem.Name != "Encoding" {
		t.Fatalf("StdEncoding type = %#v, want *Encoding", std.Type)
	}
	if _, ok := goPkg.Variables["unexported"]; ok {
		t.Fatalf("unexported variable should be skipped: %#v", goPkg.Variables)
	}
}

func TestGoPackageFromTypesDiscoversExportedStructFields(t *testing.T) {
	fset := gotoken.NewFileSet()
	file, err := goparser.ParseFile(fset, "fields.go", `package fields

type Header map[string][]string
type MyInt = int

type Embedded struct { Name string }

type Empty struct{}

type HasAny struct { Value any }
type HasAlias struct { Value MyInt }

type Generic[T any] struct { Value T }
type GenericAlias[T any] = Generic[T]

type UsesGeneric struct {
	Box Generic[int]
	Alias GenericAlias[int]
}

type Response struct {
	StatusCode int
	Header Header
	unexported string
	Embedded
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(gotypes.Config).Check("example.com/fields", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	goPkg := goPackageFromTypes("example.com/fields", "fields", pkg)
	if goPkg.Types["Header"].Struct {
		t.Fatalf("named map Header should not be marked as a struct: %#v", goPkg.Types["Header"])
	}
	if empty := goPkg.Types["Empty"]; !empty.Struct || len(empty.Fields) != 0 {
		t.Fatalf("Empty type = %#v, want zero-field struct", empty)
	}
	anyField := goPkg.Types["HasAny"].Fields["Value"].Type
	if anyField.Kind != GoValueAny || anyField.Named {
		t.Fatalf("HasAny.Value type = %#v, want non-named any", anyField)
	}
	aliasField := goPkg.Types["HasAlias"].Fields["Value"].Type
	if aliasField.Kind != GoValueInt || aliasField.Named {
		t.Fatalf("HasAlias.Value type = %#v, want non-named int", aliasField)
	}
	if generic := goPkg.Types["Generic"]; !generic.Struct || generic.TypeParams != 1 {
		t.Fatalf("Generic type = %#v, want one type parameter", generic)
	}
	if alias := goPkg.Types["GenericAlias"]; !alias.Struct || alias.TypeParams != 1 {
		t.Fatalf("GenericAlias type = %#v, want one type parameter", alias)
	}
	usesGeneric := goPkg.Types["UsesGeneric"]
	if usesGeneric.Fields["Box"].Type.TypeParams != 1 {
		t.Fatalf("UsesGeneric.Box type = %#v, want one generic type argument", usesGeneric.Fields["Box"].Type)
	}
	if usesGeneric.Fields["Alias"].Type.TypeParams != 1 {
		t.Fatalf("UsesGeneric.Alias type = %#v, want one generic type argument", usesGeneric.Fields["Alias"].Type)
	}
	response := goPkg.Types["Response"]
	if !response.Struct {
		t.Fatalf("Response should be marked as a struct: %#v", response)
	}
	status, ok := response.Fields["StatusCode"]
	if !ok {
		t.Fatalf("exported StatusCode field missing: %#v", response.Fields)
	}
	if status.Type.Kind != GoValueInt || status.Type.Bits != 0 {
		t.Fatalf("StatusCode type = %#v, want int", status.Type)
	}
	header, ok := response.Fields["Header"]
	if !ok {
		t.Fatalf("exported Header field missing: %#v", response.Fields)
	}
	if !header.Type.Named || header.Type.Name != "Header" || header.Type.ImportPath != "example.com/fields" || header.Type.Kind != GoValueMap {
		t.Fatalf("Header type = %#v, want named Header map", header.Type)
	}
	if _, ok := response.Fields["unexported"]; ok {
		t.Fatalf("unexported field should be skipped: %#v", response.Fields)
	}
	if _, ok := response.Fields["Embedded"]; ok {
		t.Fatalf("embedded fields should be deferred for now: %#v", response.Fields)
	}
}

func TestGoPackageFromTypesDiscoversEnumLikeTypedConstants(t *testing.T) {
	fset := gotoken.NewFileSet()
	file, err := goparser.ParseFile(fset, "enumlike.go", `package enumlike

type AnimationStatus int
const (
	AnimationIdle AnimationStatus = iota
	AnimationForward
	AnimationCompleted
	AnimationAlias = AnimationForward
	Untyped = 42
)

type Label string
const LabelName Label = "name"
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(gotypes.Config).Check("example.com/enumlike", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	goPkg := goPackageFromTypes("example.com/enumlike", "enumlike", pkg)
	status := goPkg.Types["AnimationStatus"]
	if len(status.EnumConstants) != 4 {
		t.Fatalf("enum constants = %#v, want 4", status.EnumConstants)
	}
	values := map[string]int{}
	for _, constant := range status.EnumConstants {
		values[constant.Name] = constant.IntValue
	}
	if values["AnimationIdle"] != 0 || values["AnimationForward"] != 1 || values["AnimationCompleted"] != 2 || values["AnimationAlias"] != 1 {
		t.Fatalf("enum constant values = %#v", values)
	}
	if len(goPkg.Types["Label"].EnumConstants) != 0 {
		t.Fatalf("string typed constants should not become enum-like: %#v", goPkg.Types["Label"].EnumConstants)
	}
}

func TestGoPackageFromTypesSkipsPromotedMethods(t *testing.T) {
	fset := gotoken.NewFileSet()
	file, err := goparser.ParseFile(fset, "promoted.go", `package promoted

type Inner struct{}
func (Inner) M() {}

type Outer struct{ Inner }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(gotypes.Config).Check("example.com/promoted", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	goPkg := goPackageFromTypes("example.com/promoted", "promoted", pkg)
	if _, ok := goPkg.Types["Inner"].Methods["M"]; !ok {
		t.Fatalf("direct method Inner.M missing: %#v", goPkg.Types["Inner"].Methods)
	}
	if _, ok := goPkg.Types["Outer"].Methods["M"]; ok {
		t.Fatalf("promoted method Outer.M should be skipped")
	}
}

func TestGoPackageFromTypesMarksInterfacesAndReceiverMethodSets(t *testing.T) {
	goPkg := goPackageFromSource(t, "example.com/interfaces", "interfaces", `package interfaces

type Reader interface { Read([]byte) (int, error) }
type ReadCloser interface { Reader; Close() error }

type ValueReader struct{}
func (ValueReader) Read([]byte) (int, error) { return 0, nil }

type PointerReader struct{}
func (*PointerReader) Read([]byte) (int, error) { return 0, nil }
`)
	reader := goPkg.Types["Reader"]
	if !reader.Interface || reader.Methods["Read"].Name != "Read" {
		t.Fatalf("Reader metadata = %#v, want interface with Read", reader)
	}
	readCloser := goPkg.Types["ReadCloser"]
	if !readCloser.Interface || readCloser.Methods["Read"].Name != "Read" || readCloser.Methods["Close"].Name != "Close" {
		t.Fatalf("ReadCloser metadata = %#v, want embedded interface methods", readCloser)
	}
	valueReader := goPkg.Types["ValueReader"]
	if _, ok := valueReader.ValueMethods["Read"]; !ok {
		t.Fatalf("ValueReader value methods = %#v, want Read", valueReader.ValueMethods)
	}
	pointerReader := goPkg.Types["PointerReader"]
	if _, ok := pointerReader.ValueMethods["Read"]; ok {
		t.Fatalf("PointerReader value methods = %#v, should not include pointer receiver", pointerReader.ValueMethods)
	}
	if _, ok := pointerReader.PointerMethods["Read"]; !ok {
		t.Fatalf("PointerReader pointer methods = %#v, want Read", pointerReader.PointerMethods)
	}
}

func TestGoValueTypesMatchRejectsDistinctUnmodeledTypes(t *testing.T) {
	if goValueTypesMatch(GoValueType{Kind: GoValueOther, Expr: "func()"}, GoValueType{Kind: GoValueOther, Expr: "chan int"}) {
		t.Fatal("distinct unmodeled Go types should not match")
	}
}

func TestCloneExternTypePreservesDirectGoMetadata(t *testing.T) {
	method := GoMethod{Name: "Read"}
	ext := &ExternType{
		Name_:                  "io::Reader",
		ExternalBinding:        "go:io as io::Reader",
		ExternalBindings:       map[string]string{"go": "go:io as io::Reader"},
		DirectGoInterface:      true,
		DirectGoMethods:        map[string]GoMethod{"Read": method},
		DirectGoValueMethods:   map[string]GoMethod{"Read": method},
		DirectGoPointerMethods: map[string]GoMethod{"Read": method},
		DirectGoType:           gotypes.Typ[gotypes.Int],
	}
	cloned := cloneExternTypeWithTypeArgs(ext, []Type{Str})
	if !cloned.DirectGoInterface || cloned.DirectGoType != ext.DirectGoType || cloned.DirectGoMethods["Read"].Name != "Read" || len(cloned.TypeArgs) != 1 {
		t.Fatalf("cloned direct Go type metadata = %#v", cloned)
	}
	cloned.DirectGoMethods["Other"] = GoMethod{Name: "Other"}
	if _, ok := ext.DirectGoMethods["Other"]; ok {
		t.Fatal("cloned method map aliases original")
	}
}

func TestGoPackagesResolverLoadsStdlibFunctionsAndMethods(t *testing.T) {
	resolver := NewGoPackagesResolver(".")
	mathPkg, err := resolver.LoadPackage("math")
	if err != nil {
		t.Fatalf("load math: %v", err)
	}
	if _, ok := mathPkg.Functions["Floor"]; !ok {
		t.Fatalf("math functions missing Floor: %#v", mathPkg.Functions)
	}

	sqlPkg, err := resolver.LoadPackage("database/sql")
	if err != nil {
		t.Fatalf("load database/sql: %v", err)
	}
	db, ok := sqlPkg.Types["DB"]
	if !ok {
		t.Fatalf("database/sql types missing DB")
	}
	if _, ok := db.Methods["Close"]; !ok {
		t.Fatalf("database/sql DB methods missing Close: %#v", db.Methods)
	}

	timePkg, err := resolver.LoadPackage("time")
	if err != nil {
		t.Fatalf("load time: %v", err)
	}
	month, ok := timePkg.Types["Month"]
	if !ok {
		t.Fatalf("time types missing Month")
	}
	if len(month.EnumConstants) < 12 {
		t.Fatalf("time.Month enum constants = %#v, want months", month.EnumConstants)
	}
	if month.ClosedEnum {
		t.Fatal("inferred Go enum-like constants should remain open without explicit closed metadata")
	}
	duration, ok := timePkg.Types["Duration"]
	if !ok {
		t.Fatalf("time types missing Duration")
	}
	if len(duration.EnumConstants) != 0 {
		t.Fatalf("time.Duration should remain an open named scalar, got enum constants %#v", duration.EnumConstants)
	}

	httpPkg, err := resolver.LoadPackage("net/http")
	if err != nil {
		t.Fatalf("load net/http: %v", err)
	}
	response, ok := httpPkg.Types["Response"]
	if !ok {
		t.Fatalf("net/http types missing Response")
	}
	if status := response.Fields["StatusCode"]; status.Type.Kind != GoValueInt || status.Type.Bits != 0 {
		t.Fatalf("http.Response.StatusCode = %#v, want int", status.Type)
	}
	request, ok := httpPkg.Types["Request"]
	if !ok {
		t.Fatalf("net/http types missing Request")
	}
	urlField := request.Fields["URL"]
	if urlField.Type.Kind != GoValuePointer || urlField.Type.Elem == nil || !urlField.Type.Elem.Named || urlField.Type.Elem.ImportPath != "net/url" || urlField.Type.Elem.Name != "URL" {
		t.Fatalf("http.Request.URL = %#v, want *net/url.URL", urlField.Type)
	}

	reflectPkg, err := resolver.LoadPackage("reflect")
	if err != nil {
		t.Fatalf("load reflect: %v", err)
	}
	kind, ok := reflectPkg.Types["Kind"]
	if !ok {
		t.Fatalf("reflect types missing Kind")
	}
	if len(kind.EnumConstants) != 0 {
		t.Fatalf("reflect.Kind has unsigned backing and should not become enum-like yet: %#v", kind.EnumConstants)
	}
}

// TestGoPackagesResolverLoadsBundledStdlib verifies that the bundled standard
// library Go packages resolve regardless of the resolver's working directory
// (the regression: from a user project dir they previously failed to load,
// silently degrading stdlib calls to Void / empty bodies).
func TestGoPackagesResolverLoadsBundledStdlib(t *testing.T) {
	resolver := NewGoPackagesResolver(t.TempDir())
	pkg, err := resolver.LoadPackage("github.com/akonwi/ard/std_lib/ffi")
	if err != nil {
		t.Fatalf("load bundled ffi: %v", err)
	}
	if pkg == nil || pkg.Name != "ffi" {
		t.Fatalf("unexpected package %#v", pkg)
	}
	if _, ok := pkg.Functions["FloatFromInt"]; !ok {
		t.Fatalf("bundled ffi missing FloatFromInt; have %d functions", len(pkg.Functions))
	}
}

// TestGoValueTypesMatchAcrossSeparateLoads guards the structural fallback used
// when a concrete type and an interface (or their method signatures) come from
// distinct go/packages loads, where types.Identical/AssignableTo fail on
// instance identity even for the same Go type.
func TestGoValueTypesMatchAcrossSeparateLoads(t *testing.T) {
	src := "package p\n\ntype Rows struct{ x int }"
	a := goPackageFromSource(t, "example.com/p", "p", src)
	b := goPackageFromSource(t, "example.com/p", "p", src)
	rowsA, okA := a.Types["Rows"]
	rowsB, okB := b.Types["Rows"]
	if !okA || !okB || rowsA.Type == nil || rowsB.Type == nil {
		t.Fatal("missing Rows type metadata")
	}
	if gotypes.Identical(rowsA.Type, rowsB.Type) {
		t.Fatal("expected distinct type instances across separate loads")
	}
	left := GoValueType{Named: true, ImportPath: "example.com/p", Name: "Rows", Type: rowsA.Type}
	right := GoValueType{Named: true, ImportPath: "example.com/p", Name: "Rows", Type: rowsB.Type}
	if !goValueTypesMatch(left, right) {
		t.Fatal("same-named Go types from separate loads should match structurally")
	}
}

// TestDirectGoInterfaceCompatibleFallsThroughToMethodSet guards that interface
// satisfaction for struct-field/let assignment falls through to the structural
// method-set comparison when types.AssignableTo fails on cross-load identity.
func TestDirectGoInterfaceCompatibleFallsThroughToMethodSet(t *testing.T) {
	ifacePkg := goPackageFromSource(t, "example.com/p", "p", "package p\n\ntype Reader interface{ Read() int }")
	implPkg := goPackageFromSource(t, "example.com/q", "q", "package q\n\ntype Empty struct{}")
	ifaceType := ifacePkg.Types["Reader"].Type
	implType := implPkg.Types["Empty"].Type
	if ifaceType == nil || implType == nil {
		t.Fatal("missing type metadata")
	}
	// Precondition: the concrete type is NOT assignable to the interface by
	// go/types, so success can only come from the structural fallback.
	if gotypes.AssignableTo(implType, ifaceType) {
		t.Fatal("precondition failed: Empty should not implement Reader")
	}

	readMethod := map[string]GoMethod{"Read": {Name: "Read", Signature: GoSignature{Results: []GoValueType{{Kind: GoValueInt, Expr: "int"}}}}}
	expected := &ExternType{DirectGoInterface: true, DirectGoType: ifaceType, DirectGoMethods: readMethod}
	actual := &ExternType{DirectGoType: implType, DirectGoMethods: readMethod}

	if !directGoInterfaceCompatible(expected, actual) {
		t.Fatal("matching method sets should satisfy the interface despite AssignableTo failing")
	}
}
