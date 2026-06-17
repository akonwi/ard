package checker

import (
	"fmt"
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

func goParam(kind GoValueKind, expr string) GoValueType {
	return GoValueType{Kind: kind, Expr: expr}
}

func goNamed(kind GoValueKind, expr string, importPath string, name string) GoValueType {
	return GoValueType{Kind: kind, Expr: expr, Named: true, ImportPath: importPath, Package: importPath, Name: name}
}

func TestDirectGoExternBindingValidatesImportedFunction(t *testing.T) {
	result := parse.Parse([]byte(`use go:math
extern fn floor(value: Float) Float = math::Floor`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"math": {ImportPath: "math", Name: "math", Functions: map[string]GoFunction{"Floor": {Name: "Floor", Signature: GoSignature{Params: []GoValueType{goParam(GoValueFloat, "float64")}, Results: []GoValueType{goParam(GoValueFloat, "float64")}}}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
	fn, ok := c.Module().Program().Statements[0].Expr.(*ExternalFunctionDef)
	if !ok {
		t.Fatalf("expected external function, got %#v", c.Module().Program().Statements[0].Expr)
	}
	if fn.ExternalBinding != "go:math::Floor" || fn.ExternalBindings["go"] != "go:math::Floor" {
		t.Fatalf("external binding = %q / %#v", fn.ExternalBinding, fn.ExternalBindings)
	}
}

func TestDirectGoTypeReferenceDoesNotRequireExternType(t *testing.T) {
	result := parse.Parse([]byte(`use go:time
extern fn sleep(duration: time::Duration) Void = time::Sleep`), "main.ard")
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
	fn := c.Module().Program().Statements[0].Expr.(*ExternalFunctionDef)
	duration, ok := fn.Parameters[0].Type.(*ExternType)
	if !ok {
		t.Fatalf("param type = %#v, want ExternType", fn.Parameters[0].Type)
	}
	if duration.ExternalBinding != "go:time::Duration" {
		t.Fatalf("duration binding = %q", duration.ExternalBinding)
	}
}

func TestDirectGoExternBindingRequiresImportedAlias(t *testing.T) {
	result := parse.Parse([]byte(`extern fn floor(value: Float) Float = math::Floor`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected missing Go import alias diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, `Unknown Go import alias "math"`) {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternBindingValidatesMissingFunction(t *testing.T) {
	result := parse.Parse([]byte(`use go:math
extern fn floor(value: Float) Float = math::Missing`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"math": {ImportPath: "math", Name: "math", Functions: map[string]GoFunction{"Floor": {Name: "Floor"}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected missing Go function diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, `Go package "math" has no exported function "Missing"`) {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternBindingValidatesMethods(t *testing.T) {
	result := parse.Parse([]byte(`use go:time
extern fn stringify(value: time::Time) Str = time::Time::String`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	receiver := goNamed(GoValueOther, "time.Time", "time", "Time")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"time": {
			ImportPath: "time",
			Name:       "time",
			Types: map[string]GoType{
				"Time": {Name: "Time", Methods: map[string]GoMethod{"String": {Name: "String", Signature: GoSignature{Receiver: &receiver, Results: []GoValueType{goParam(GoValueString, "string")}}}}},
			},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoExternSignatureRejectsParameterArityMismatch(t *testing.T) {
	result := parse.Parse([]byte(`use go:math
extern fn floor(left: Float, right: Float) Float = math::Floor`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"math": {ImportPath: "math", Name: "math", Functions: map[string]GoFunction{"Floor": {Name: "Floor", Signature: GoSignature{Params: []GoValueType{goParam(GoValueFloat, "float64")}, Results: []GoValueType{goParam(GoValueFloat, "float64")}}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected arity diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "expects 1 parameter(s)") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternSignatureRejectsParameterTypeMismatch(t *testing.T) {
	result := parse.Parse([]byte(`use go:math
extern fn floor(value: Str) Float = math::Floor`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"math": {ImportPath: "math", Name: "math", Functions: map[string]GoFunction{"Floor": {Name: "Floor", Signature: GoSignature{Params: []GoValueType{goParam(GoValueFloat, "float64")}, Results: []GoValueType{goParam(GoValueFloat, "float64")}}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected parameter type diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "parameter 1") || !strings.Contains(got, "Ard type Str is not compatible with Go type float64") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternSignatureRejectsReturnTypeMismatch(t *testing.T) {
	result := parse.Parse([]byte(`use go:math
extern fn floor(value: Float) Str = math::Floor`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"math": {ImportPath: "math", Name: "math", Functions: map[string]GoFunction{"Floor": {Name: "Floor", Signature: GoSignature{Params: []GoValueType{goParam(GoValueFloat, "float64")}, Results: []GoValueType{goParam(GoValueFloat, "float64")}}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected return type diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "return for math.Floor") || !strings.Contains(got, "Ard type Str is not compatible with Go type float64") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternSignatureRejectsErrorAdapterShapeForNow(t *testing.T) {
	result := parse.Parse([]byte(`use go:os
extern fn chdir(dir: Str) Void!Str = os::Chdir`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"os": {ImportPath: "os", Name: "os", Functions: map[string]GoFunction{"Chdir": {Name: "Chdir", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueError, "error")}}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected error adapter diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Go return error requires an adapter") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternSignatureRejectsMultipleReturnAdaptersForNow(t *testing.T) {
	result := parse.Parse([]byte(`use go:strconv
extern fn atoi(value: Str) Int!Str = strconv::Atoi`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"strconv": {ImportPath: "strconv", Name: "strconv", Functions: map[string]GoFunction{"Atoi": {Name: "Atoi", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueInt, "int"), goParam(GoValueError, "error")}}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected multiple-return adapter diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "multiple-return direct Go adapters are not supported yet") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternSignatureAcceptsListAndMapTypes(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/collections as collections
extern fn split(value: Str) [Str] = collections::Split
extern fn counts(value: [Str]) [Str:Int] = collections::Counts`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	str := goParam(GoValueString, "string")
	intType := goParam(GoValueInt, "int")
	strSlice := GoValueType{Kind: GoValueSlice, Expr: "[]string", Elem: &str}
	strIntMap := GoValueType{Kind: GoValueMap, Expr: "map[string]int", Key: &str, Value: &intType}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/collections": {ImportPath: "example.com/collections", Name: "collections", Functions: map[string]GoFunction{
			"Split":  {Name: "Split", Signature: GoSignature{Params: []GoValueType{str}, Results: []GoValueType{strSlice}}},
			"Counts": {Name: "Counts", Signature: GoSignature{Params: []GoValueType{strSlice}, Results: []GoValueType{strIntMap}}},
		}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoExternSignatureRejectsPointerTypesForNow(t *testing.T) {
	result := parse.Parse([]byte(`use go:database/sql as sql
extern fn ping(db: sql::DB) Void = sql::DB::Ping`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	db := goNamed(GoValueOther, "sql.DB", "database/sql", "DB")
	ptrDB := GoValueType{Kind: GoValuePointer, Expr: "*sql.DB", Elem: &db}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"database/sql": {ImportPath: "database/sql", Name: "sql", Types: map[string]GoType{
			"DB": {Name: "DB", Methods: map[string]GoMethod{"Ping": {Name: "Ping", Signature: GoSignature{Receiver: &ptrDB}}}},
		}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected pointer diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "direct Go pointer bindings are not supported yet") {
		t.Fatalf("diagnostic = %q", got)
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
}
