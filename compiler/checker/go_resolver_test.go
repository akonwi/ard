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

func TestDirectGoExternBindingValidatesImportedFunction(t *testing.T) {
	result := parse.Parse([]byte(`use go:math
extern fn floor(value: Float) Float = math::Floor`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"math": {ImportPath: "math", Name: "math", Functions: map[string]GoFunction{"Floor": {Name: "Floor"}}},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
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
	result := parse.Parse([]byte(`use go:database/sql as sql
extern fn close(db: Dynamic) Void!Str = sql::DB::Close`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := New("main.ard", result.Program, nil, CheckOptions{GoResolver: fakeGoResolver{packages: map[string]*GoPackage{
		"database/sql": {
			ImportPath: "database/sql",
			Name:       "sql",
			Types: map[string]GoType{
				"DB": {Name: "DB", Methods: map[string]GoMethod{"Close": {Name: "Close"}}},
			},
		},
	}}})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
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
