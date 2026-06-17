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
	if fn.ExternalBinding != "go:math as math::Floor" || fn.ExternalBindings["go"] != "go:math as math::Floor" {
		t.Fatalf("external binding = %q / %#v", fn.ExternalBinding, fn.ExternalBindings)
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
	if duration.ExternalBinding != "go:time as time::Duration" {
		t.Fatalf("duration binding = %q", duration.ExternalBinding)
	}
}

func TestDirectGoEnumLikeConstantsResolveAsClosedEnum(t *testing.T) {
	result := parse.Parse([]byte(`use go:git.sr.ht/~rockorager/vaxis as vaxis
extern fn next(status: vaxis::AnimationStatus) vaxis::AnimationStatus = vaxis::Next
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
	pkg.Functions = map[string]GoFunction{"Next": {Name: "Next", Signature: GoSignature{Params: []GoValueType{status}, Results: []GoValueType{status}}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
	fn := c.Module().Program().Statements[0].Expr.(*ExternalFunctionDef)
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

func TestDirectGoExternSignatureAcceptsErrorToVoidResultAdapter(t *testing.T) {
	result := parse.Parse([]byte(`use go:os
extern fn chdir(dir: Str) Void!Str = os::Chdir`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"os": {ImportPath: "os", Name: "os", Functions: map[string]GoFunction{"Chdir": {Name: "Chdir", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueError, "error")}}}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoExternSignatureAcceptsValueErrorToResultAdapter(t *testing.T) {
	result := parse.Parse([]byte(`use go:strconv
extern fn atoi(value: Str) Int!Str = strconv::Atoi`), "main.ard")
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

func TestDirectGoExternSignatureAcceptsValueBoolToMaybeAdapter(t *testing.T) {
	result := parse.Parse([]byte(`use go:os
extern fn lookup_env(key: Str) Str? = os::LookupEnv`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"os": {ImportPath: "os", Name: "os", Functions: map[string]GoFunction{"LookupEnv": {Name: "LookupEnv", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueString, "string"), goParam(GoValueBool, "bool")}}}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoExternSignatureRejectsNamedBoolMaybeAdapter(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/lookup as lookup
extern fn lookup_value(key: Str) Str? = lookup::Value`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/lookup": {ImportPath: "example.com/lookup", Name: "lookup", Functions: map[string]GoFunction{"Value": {Name: "Value", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueString, "string"), goNamed(GoValueBool, "lookup.Found", "example.com/lookup", "Found")}}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected unsupported named bool adapter diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "no supported adapter matches") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternSignatureRejectsUnsupportedMultipleReturnAdapters(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/unsupported as unsupported
extern fn pair(value: Str) Int = unsupported::Pair`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"example.com/unsupported": {ImportPath: "example.com/unsupported", Name: "unsupported", Functions: map[string]GoFunction{"Pair": {Name: "Pair", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string")}, Results: []GoValueType{goParam(GoValueInt, "int"), goParam(GoValueString, "string")}}}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected unsupported multiple-return adapter diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "no supported adapter matches") {
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

func TestDirectGoExternSignatureAcceptsMutableDirectGoPointerReceiver(t *testing.T) {
	result := parse.Parse([]byte(`use go:database/sql as sql
extern fn ping(db: mut sql::DB) Void!Str = sql::DB::Ping`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	db := goNamed(GoValueOther, "sql.DB", "database/sql", "DB")
	ptrDB := GoValueType{Kind: GoValuePointer, Expr: "*sql.DB", Elem: &db}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"database/sql": {ImportPath: "database/sql", Name: "sql", Types: map[string]GoType{
			"DB": {Name: "DB", Methods: map[string]GoMethod{"Ping": {Name: "Ping", Signature: GoSignature{Receiver: &ptrDB, Results: []GoValueType{goParam(GoValueError, "error")}}}}},
		}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoExternSignatureAcceptsMutableDirectGoPointerReturn(t *testing.T) {
	result := parse.Parse([]byte(`use go:os
extern fn create_temp(dir: Str, pattern: Str) (mut os::File)!Str = os::CreateTemp`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	file := goNamed(GoValueOther, "os.File", "os", "File")
	ptrFile := GoValueType{Kind: GoValuePointer, Expr: "*os.File", Elem: &file}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"os": {ImportPath: "os", Name: "os", Functions: map[string]GoFunction{
			"CreateTemp": {Name: "CreateTemp", Signature: GoSignature{Params: []GoValueType{goParam(GoValueString, "string"), goParam(GoValueString, "string")}, Results: []GoValueType{ptrFile, goParam(GoValueError, "error")}}},
		}, Types: map[string]GoType{"File": {Name: "File"}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestDirectGoExternSignatureRejectsMutableEnumPointerReturn(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/status as status
extern fn current() (mut status::State)!Str = status::Current`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	state := goNamed(GoValueInt, "status.State", "example.com/status", "State")
	ptrState := GoValueType{Kind: GoValuePointer, Expr: "*status.State", Elem: &state}
	pkg := fakeEnumLikePackage("example.com/status", "status", state, []GoConstant{
		goEnumConstant("StateReady", state, 0),
		goEnumConstant("StateDone", state, 1),
	})
	pkg.Functions = map[string]GoFunction{"Current": {Name: "Current", Signature: GoSignature{Results: []GoValueType{ptrState, goParam(GoValueError, "error")}}}}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{pkg.ImportPath: pkg}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected enum pointer diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "pointer to enum-like type") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternSignatureRejectsPlainValueForPointerReceiver(t *testing.T) {
	result := parse.Parse([]byte(`use go:database/sql as sql
extern fn ping(db: sql::DB) Void!Str = sql::DB::Ping`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	db := goNamed(GoValueOther, "sql.DB", "database/sql", "DB")
	ptrDB := GoValueType{Kind: GoValuePointer, Expr: "*sql.DB", Elem: &db}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"database/sql": {ImportPath: "database/sql", Name: "sql", Types: map[string]GoType{
			"DB": {Name: "DB", Methods: map[string]GoMethod{"Ping": {Name: "Ping", Signature: GoSignature{Receiver: &ptrDB, Results: []GoValueType{goParam(GoValueError, "error")}}}}},
		}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected pointer diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "requires Ard type mut sql::DB") {
		t.Fatalf("diagnostic = %q", got)
	}
}

func TestDirectGoExternSignatureRejectsNamedScalarReturnsUntilReturnCoercionExists(t *testing.T) {
	result := parse.Parse([]byte(`use go:time
extern fn since(value: time::Time) Int = time::Since`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	timeType := goNamed(GoValueOther, "time.Time", "time", "Time")
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"time": {ImportPath: "time", Name: "time", Functions: map[string]GoFunction{
			"Since": {Name: "Since", Signature: GoSignature{Params: []GoValueType{timeType}, Results: []GoValueType{goNamed(GoValueInt, "time.Duration", "time", "Duration")}}},
		}, Types: map[string]GoType{"Time": {Name: "Time"}, "Duration": {Name: "Duration"}}},
	}}})
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected named scalar return diagnostic")
	}
	if got := c.Diagnostics()[0].Message; !strings.Contains(got, "Ard type Int is not compatible with Go named type time.Duration") {
		t.Fatalf("diagnostic = %q", got)
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
