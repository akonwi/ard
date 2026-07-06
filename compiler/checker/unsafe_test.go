package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

func TestUnsafeCatchValidationDoesNotSpecialCaseUserResultModules(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"ardmodtest\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "my"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "my", "result.ard"), []byte(`fn err() Int!Str {
  Result::ok(5)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(tempDir, "main.ard")
	result := parse.Parse([]byte(`use ardmodtest/my/result as result

fn inner() Int!Str {
  Result::err("inner")
}

fn bad() Str!Str {
  unsafe {
    let value = try inner() -> _ { result::err() }
    value.to_str()
  }
}`), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, result.Program, resolver)
	c.Check()
	want := []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected Str!Str, got Int!Str"}}
	if diff := cmp.Diff(want, c.Diagnostics(), compareOptions); diff != "" {
		t.Fatalf("Diagnostics mismatch (-want +got):\n%s", diff)
	}
}

func TestUnsafeCastTypeChecksValueTarget(t *testing.T) {
	module, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe

let value: Any = "hello"
let text = unsafe::cast<Str>(value)`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("text").Type.String()
	if got != "Str?" {
		t.Fatalf("text type = %q, want Str?", got)
	}
}

func TestUnsafeCastTypeChecksMutableTarget(t *testing.T) {
	module, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe

let value: Any = "hello"
let text = unsafe::cast<mut Str>(value)`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("text").Type.String()
	if got != "(mut Str)?" {
		t.Fatalf("text type = %q, want (mut Str)?", got)
	}
}

func TestUnsafeCastWorksThroughImportAlias(t *testing.T) {
	module, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe as box

let value: Any = "hello"
let text = box::cast<Str>(value)`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("text").Type.String()
	if got != "Str?" {
		t.Fatalf("text type = %q, want Str?", got)
	}
}

func TestUnsafeCastRequiresImport(t *testing.T) {
	_, diagnostics := checkUnsafeCastSource(t, `let value: Any = "hello"
let text = unsafe::cast<Str>(value)`)
	assertUnsafeCastDiagnostic(t, diagnostics, "Undefined module: unsafe")
}

func TestUnsafeCastRejectsPreludeAnyAlias(t *testing.T) {
	_, diagnostics := checkUnsafeCastSource(t, `let value: Any = "hello"
let text = Any::cast<Str>(value)`)
	assertUnsafeCastDiagnostic(t, diagnostics, "Undefined module: Any")
}

func TestUnsafeCastRejectsUnknownNamedArgument(t *testing.T) {
	_, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe

let value: Any = "hello"
let text = unsafe::cast<Str>(wrong: value)`)
	assertUnsafeCastDiagnostic(t, diagnostics, "unknown argument: wrong")
}

func TestUnsafeCastAcceptsNamedValueArgument(t *testing.T) {
	module, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe

let value: Any = "hello"
let text = unsafe::cast<Str>(value: value)`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("text").Type.String()
	if got != "Str?" {
		t.Fatalf("text type = %q, want Str?", got)
	}
}

func TestUnsafeCastRequiresExplicitTypeArgument(t *testing.T) {
	_, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe

let value: Any = "hello"
let text = unsafe::cast(value)`)
	assertUnsafeCastDiagnostic(t, diagnostics, "unsafe::cast requires exactly one explicit type argument")
}

func TestUnsafeCastAcceptsBoxableArgument(t *testing.T) {
	module, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe

let text = unsafe::cast<Str>("hello")`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("text").Type.String()
	if got != "Str?" {
		t.Fatalf("text type = %q, want Str?", got)
	}
}

func TestUnsafeIsNilWorksThroughImportAlias(t *testing.T) {
	module, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe as u

let nil = u::is_nil("hello")`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("nil").Type.String()
	if got != "Bool" {
		t.Fatalf("nil type = %q, want Bool", got)
	}
}

func TestUnsafeIsNilRequiresImport(t *testing.T) {
	_, diagnostics := checkUnsafeCastSource(t, `let nil = unsafe::is_nil("hello")`)
	assertUnsafeCastDiagnostic(t, diagnostics, "Undefined module: unsafe")
}

func TestUnsafeIsNilTypeChecks(t *testing.T) {
	module, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe

let value: Any = "hello"
let nil = unsafe::is_nil(value)`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("nil").Type.String()
	if got != "Bool" {
		t.Fatalf("nil type = %q, want Bool", got)
	}
}

func TestUnsafeIsNilAcceptsNamedValueArgument(t *testing.T) {
	module, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe

let nil = unsafe::is_nil(value: "hello")`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("nil").Type.String()
	if got != "Bool" {
		t.Fatalf("nil type = %q, want Bool", got)
	}
}

func TestUnsafeIsNilRejectsTypeArgument(t *testing.T) {
	_, diagnostics := checkUnsafeCastSource(t, `use ard/unsafe

let nil = unsafe::is_nil<Str>("hello")`)
	assertUnsafeCastDiagnostic(t, diagnostics, "unsafe::is_nil does not accept type arguments")
}

func checkUnsafeCastSource(t *testing.T, source string) (checker.Module, []checker.Diagnostic) {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	return c.Module(), c.Diagnostics()
}

func assertUnsafeCastDiagnostic(t *testing.T, diagnostics []checker.Diagnostic, want string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, want) {
			return
		}
	}
	t.Fatalf("diagnostics %v do not contain %q", diagnostics, want)
}
