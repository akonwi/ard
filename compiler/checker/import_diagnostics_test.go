package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// Issue #350 contract: every imported Ard module — project-owned and
// dependencies alike — is re-validated by the importing compiler, and its
// body diagnostics are reported with spans attributed to the module's own
// file. The embedded standard library is the single exception: it is gated
// at compiler build time by TestEmbeddedStdLibModulesCheckCleanly instead.
const invalidModuleBody = `fn broken(a: [Int]) [Int] {
  mut res = a
  res.push(1)
  res
}
`

func checkImporter(t *testing.T, projectDir string, source string) []checker.Diagnostic {
	t.Helper()
	resolver, err := checker.NewModuleResolver(projectDir)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	result := parse.Parse([]byte(source), filepath.Join(projectDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New(filepath.Join(projectDir, "main.ard"), result.Program, resolver)
	c.Check()
	return c.Diagnostics()
}

func requireModuleBodyDiagnostic(t *testing.T, diagnostics []checker.Diagnostic, moduleFile string) {
	t.Helper()
	if len(diagnostics) == 0 {
		t.Fatal("expected imported module diagnostics, got none")
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == checker.DiagnosticCodeValueInteriorMutation {
			if filepath.Base(diagnostic.Primary.Span.FilePath) != moduleFile {
				t.Fatalf("diagnostic attributed to %q, want %q", diagnostic.Primary.Span.FilePath, moduleFile)
			}
			return
		}
	}
	t.Fatalf("no interior-mutation diagnostic from the imported module, got: %v", diagnostics)
}

func TestProjectModuleBodyDiagnosticsSurfaceAtImport(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "lib", "util.ard"), []byte(invalidModuleBody), 0o644); err != nil {
		t.Fatal(err)
	}

	diagnostics := checkImporter(t, projectDir, "use app/lib/util\n\nfn main() {\n  let out = util::broken([1])\n}\n")
	requireModuleBodyDiagnostic(t, diagnostics, "util.ard")
}

func TestWarningOnlyImportedModuleRemainsUsable(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "util.ard"), []byte(`let signal: Void? = Maybe::new()

fn answer() Int { 42 }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	diagnostics := checkImporter(t, projectDir, "use app/util\n\nfn main() Int { util::answer() }\n")
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one imported-module warning", diagnostics)
	}
	if diagnostic := diagnostics[0]; diagnostic.Kind != checker.Warn || diagnostic.Code != checker.DiagnosticCodeRedundantNullableVoid {
		t.Fatalf("diagnostic = %#v, want redundant nullable Void warning", diagnostic)
	}
}

func TestPathDependencyBodyDiagnosticsSurfaceAtImport(t *testing.T) {
	workspace := t.TempDir()
	app := filepath.Join(workspace, "app")
	dep := filepath.Join(workspace, "dep-src")
	for _, dir := range []string{app, dep} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dep, "ard.toml"), []byte("name = \"dep\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dep, "dep.ard"), []byte(invalidModuleBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep-src\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diagnostics := checkImporter(t, app, "use dep\n\nfn main() {\n  let out = dep::broken([1])\n}\n")
	requireModuleBodyDiagnostic(t, diagnostics, "dep.ard")
}
