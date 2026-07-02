package checker_test

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestAnyCastTypeChecksValueTarget(t *testing.T) {
	module, diagnostics := checkAnyCastSource(t, `use ard/any

let value: Any = "hello"
let text = any::cast<Str>(value)`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("text").Type.String()
	if got != "Str?" {
		t.Fatalf("text type = %q, want Str?", got)
	}
}

func TestAnyCastTypeChecksMutableTarget(t *testing.T) {
	module, diagnostics := checkAnyCastSource(t, `use ard/any

let value: Any = "hello"
let text = any::cast<mut Str>(value)`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("text").Type.String()
	if got != "(mut Str)?" {
		t.Fatalf("text type = %q, want (mut Str)?", got)
	}
}

func TestAnyCastWorksThroughImportAlias(t *testing.T) {
	module, diagnostics := checkAnyCastSource(t, `use ard/any as box

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

func TestAnyCastRequiresImport(t *testing.T) {
	_, diagnostics := checkAnyCastSource(t, `let value: Any = "hello"
let text = any::cast<Str>(value)`)
	assertAnyCastDiagnostic(t, diagnostics, "Undefined module: any")
}

func TestAnyCastRejectsPreludeAnyAlias(t *testing.T) {
	_, diagnostics := checkAnyCastSource(t, `let value: Any = "hello"
let text = Any::cast<Str>(value)`)
	assertAnyCastDiagnostic(t, diagnostics, "any::cast requires importing ard/any")
}

func TestAnyCastRejectsUnknownNamedArgument(t *testing.T) {
	_, diagnostics := checkAnyCastSource(t, `use ard/any

let value: Any = "hello"
let text = any::cast<Str>(wrong: value)`)
	assertAnyCastDiagnostic(t, diagnostics, "unknown argument: wrong")
}

func TestAnyCastAcceptsNamedValueArgument(t *testing.T) {
	module, diagnostics := checkAnyCastSource(t, `use ard/any

let value: Any = "hello"
let text = any::cast<Str>(value: value)`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("text").Type.String()
	if got != "Str?" {
		t.Fatalf("text type = %q, want Str?", got)
	}
}

func TestAnyCastRequiresExplicitTypeArgument(t *testing.T) {
	_, diagnostics := checkAnyCastSource(t, `use ard/any

let value: Any = "hello"
let text = any::cast(value)`)
	assertAnyCastDiagnostic(t, diagnostics, "any::cast requires exactly one explicit type argument")
}

func TestAnyCastAcceptsBoxableArgument(t *testing.T) {
	module, diagnostics := checkAnyCastSource(t, `use ard/any

let text = any::cast<Str>("hello")`)
	if len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	got := module.Get("text").Type.String()
	if got != "Str?" {
		t.Fatalf("text type = %q, want Str?", got)
	}
}

func checkAnyCastSource(t *testing.T, source string) (checker.Module, []checker.Diagnostic) {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	return c.Module(), c.Diagnostics()
}

func assertAnyCastDiagnostic(t *testing.T, diagnostics []checker.Diagnostic, want string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, want) {
			return
		}
	}
	t.Fatalf("diagnostics %v do not contain %q", diagnostics, want)
}
