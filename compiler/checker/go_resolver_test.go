package checker_test

import (
	"fmt"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

type recordingGoResolver struct {
	paths []string
}

func (r *recordingGoResolver) ResolveGoPackage(path string) (*checker.GoPackage, error) {
	r.paths = append(r.paths, path)
	if path != "example.com/ffi" {
		return nil, fmt.Errorf("unexpected path %s", path)
	}
	pointType := &checker.ForeignType{
		Target:       "go",
		Namespace:    path,
		Qualifier:    "ffi",
		Name:         "Point",
		Struct:       true,
		Fields:       map[string]checker.Type{"X": checker.Int},
		FieldsLoaded: true,
		Methods: map[string]*checker.FunctionDef{
			"Label": {Name: "Label", ReturnType: checker.Str},
		},
		MethodsLoaded: true,
	}
	counterType := &checker.ForeignType{
		Target:        "go",
		Namespace:     path,
		Qualifier:     "ffi",
		Name:          "Counter",
		MethodsLoaded: true,
	}
	counterType.LoadMethods = func(pointer bool) (map[string]*checker.FunctionDef, map[string]string) {
		if !pointer {
			return map[string]*checker.FunctionDef{}, map[string]string{}
		}
		return map[string]*checker.FunctionDef{"Reset": {Name: "Reset", ReturnType: checker.Void}}, map[string]string{}
	}
	return &checker.GoPackage{
		Path:      path,
		TypesName: "ffi",
		Functions: map[string]*checker.FunctionDef{
			"Print": {
				Name:       "Print",
				Parameters: []checker.Parameter{{Name: "value", Type: checker.Str}},
				ReturnType: checker.Void,
			},
			"NewPoint": {
				Name:       "NewPoint",
				ReturnType: pointType,
			},
			"NewCounter": {
				Name:       "NewCounter",
				ReturnType: counterType,
			},
		},
		Types:                map[string]checker.Type{},
		Constants:            map[string]checker.Type{},
		Variables:            map[string]checker.Type{},
		UnsupportedConstants: map[string]string{"UnsupportedConst": "named Go types with underlying chan int are not supported yet"},
		UnsupportedVariables: map[string]string{"UnsupportedVar": "named Go types with underlying chan int are not supported yet"},
		UnsupportedFunctions: map[string]string{},
	}, nil
}

func checkWithRecordingGoResolver(t *testing.T, source string) *recordingGoResolver {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %#v", result.Errors)
	}
	resolver := &recordingGoResolver{}
	c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %#v", c.Diagnostics())
	}
	return resolver
}

func TestCheckerUsesConfiguredGoResolver(t *testing.T) {
	resolver := checkWithRecordingGoResolver(t, `use go:example.com/ffi as ffi

fn main() {
  ffi::Print("hello")
}`)
	if got, want := fmt.Sprint(resolver.paths), "[example.com/ffi]"; got != want {
		t.Fatalf("resolved paths = %s, want %s", got, want)
	}
}

func TestConfiguredGoResolverProvidesForeignFieldsAndMethods(t *testing.T) {
	checkWithRecordingGoResolver(t, `use go:example.com/ffi as ffi

fn main() Str {
  let point = ffi::NewPoint()
  let _ = point.X
  point.Label()
}`)
}

func TestConfiguredGoResolverProvidesPointerForeignMethods(t *testing.T) {
	checkWithRecordingGoResolver(t, `use go:example.com/ffi as ffi

fn main() {
  mut counter = ffi::NewCounter()
  counter.Reset()
}`)
}

func TestConfiguredGoResolverReportsUnsupportedVariables(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/ffi as ffi

let _ = ffi::UnsupportedVar`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %#v", result.Errors)
	}
	resolver := &recordingGoResolver{}
	c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
	c.Check()
	want := []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported Go variable ffi::UnsupportedVar: named Go types with underlying chan int are not supported yet"}}
	if diff := cmp.Diff(want, c.Diagnostics(), compareOptions); diff != "" {
		t.Fatalf("Diagnostics mismatch (-want +got):\n%s", diff)
	}
}

func TestConfiguredGoResolverReportsUnsupportedConstantAssignment(t *testing.T) {
	result := parse.Parse([]byte(`use go:example.com/ffi as ffi

fn main() {
  ffi::UnsupportedConst = 1
}`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %#v", result.Errors)
	}
	resolver := &recordingGoResolver{}
	c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
	c.Check()
	want := []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported Go constant ffi::UnsupportedConst: named Go types with underlying chan int are not supported yet"}}
	if diff := cmp.Diff(want, c.Diagnostics(), compareOptions); diff != "" {
		t.Fatalf("Diagnostics mismatch (-want +got):\n%s", diff)
	}
}
