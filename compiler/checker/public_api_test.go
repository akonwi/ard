package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestPublicStructFieldCannotExposePrivateType(t *testing.T) {
	assertPrivateTypeExposure(t, `
private struct Hidden {
  value: Int,
}

struct Public {
  hidden: Hidden?,
}
`, "public field `hidden` exposes private type `Hidden`")
}

func TestPublicAPICannotExposeNestedPrivateTypes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "function return",
			source: `
private struct Hidden {}
fn expose() [Hidden] { [] }
`,
			want: "function `expose` return type exposes private type `Hidden`",
		},
		{
			name: "function parameter callback",
			source: `
private struct Hidden {}
fn consume(callback: fn(Hidden) Int) { callback(Hidden{}) }
`,
			want: "function `consume` parameter `callback` exposes private type `Hidden`",
		},
		{
			name: "generic argument",
			source: `
private struct Hidden {}
struct Box<$T> { value: $T }
fn expose() Box<Hidden> { Box<Hidden>{value: Hidden{}} }
`,
			want: "function `expose` return type exposes private type `Hidden`",
		},
		{
			name: "inferred global",
			source: `
private struct Hidden {}
let state = Hidden{}
`,
			want: "public variable `state` exposes private type `Hidden`",
		},
		{
			name: "union member",
			source: `
private struct Hidden {}
type Public = Int | Hidden
`,
			want: "public union `Public` exposes private type `Hidden`",
		},
		{
			name: "trait method",
			source: `
private struct Hidden {}
trait Public {
  fn reveal() Hidden
}
`,
			want: "public trait method `Public.reveal` return type exposes private type `Hidden`",
		},
		{
			name: "struct method",
			source: `
private struct Hidden {}
struct Public {}
impl Public {
  fn reveal() Hidden { Hidden{} }
}
`,
			want: "public method `Public.reveal` return type exposes private type `Hidden`",
		},
		{
			name: "public alias",
			source: `
private struct Hidden {}
type Public = Hidden?
`,
			want: "public type alias `Public` exposes private type `Hidden`",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertPrivateTypeExposure(t, test.source, test.want)
		})
	}
}

func TestPublicAPIDoesNotTreatValuesAsNominalDeclarations(t *testing.T) {
	result := parse.Parse([]byte(`
private struct Hidden {}
struct Public { hidden: Hidden }
let value = Public{hidden: Hidden{}}
`), "model.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("model.ard", result.Program, nil)
	c.Check()
	count := 0
	for _, diagnostic := range c.Diagnostics() {
		if diagnostic.Code == checker.DiagnosticCodePrivateTypeExposure {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("private type exposure diagnostics = %d, want 1: %v", count, c.Diagnostics())
	}
}

func TestPublicGenericDeclarationMayExposeItsTypeParameter(t *testing.T) {
	result := parse.Parse([]byte(`
struct Box<$T> { value: $T }
fn wrap(value: $T) Box<$T> { Box<$T>{value: value} }
`), "model.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("model.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestImportedModuleRejectsPrivateTypeInPublicField(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"privatefield\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "model.ard"), []byte(`
private struct Hidden { value: Int }
struct Public { hidden: Hidden? }
fn make() Public { Public{hidden: Hidden{value: 1}} }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte("use privatefield/model\nfn main() { let value = model::make() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainSource, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse(mainSource, mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, result.Program, resolver)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected imported module private type exposure diagnostic")
	}
	for _, diagnostic := range c.Diagnostics() {
		if strings.Contains(diagnostic.Message, "public field `hidden` exposes private type `Hidden`") {
			return
		}
	}
	t.Fatalf("diagnostics = %v", c.Diagnostics())
}

func TestPrivateTypesMayRemainInsideModuleImplementation(t *testing.T) {
	result := parse.Parse([]byte(`
private struct Hidden {}
private fn make_hidden() Hidden { Hidden{} }
struct Public { value: Int }
fn make_public() Public { Public{value: 1} }
`), "model.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("model.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func assertPrivateTypeExposure(t *testing.T, source string, want string) {
	t.Helper()
	result := parse.Parse([]byte(source), "model.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	c := checker.New("model.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("checker succeeded; expected private type exposure diagnostic")
	}
	for _, diagnostic := range c.Diagnostics() {
		if strings.Contains(diagnostic.Message, want) {
			return
		}
	}
	messages := make([]string, len(c.Diagnostics()))
	for i, diagnostic := range c.Diagnostics() {
		messages[i] = diagnostic.Message
	}
	t.Fatalf("diagnostics = %q, want one containing %q", messages, want)
}
