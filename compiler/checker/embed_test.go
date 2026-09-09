package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func checkEmbedSource(t *testing.T, root string, source string) *checker.Checker {
	t.Helper()
	mainPath := filepath.Join(root, "main.ard")
	result := parse.Parse([]byte(source), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := checker.New(mainPath, result.Program, resolver)
	checked.Check()
	return checked
}

func hasEmbedDiagnostic(checked *checker.Checker, code checker.DiagnosticCode) bool {
	for _, diagnostic := range checked.Diagnostics() {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestEmbedExactFilesResolveFromOwningPackageRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("hello from an embedded file\n")
	if err := os.WriteFile(filepath.Join(root, "assets", "page.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "ui"), 0o755); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(root, "ui", "page.ard")
	source := "use ard/embed\nlet page = embed::text(\"assets/page.txt\")\nlet raw = embed::bytes(\"assets/page.txt\")\n"
	result := parse.Parse([]byte(source), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := checker.New(mainPath, result.Program, resolver)
	checked.Check()
	if checked.HasErrors() {
		t.Fatalf("checker diagnostics: %#v", checked.Diagnostics())
	}

	statements := checked.Module().Program().Statements
	if len(statements) != 2 {
		t.Fatalf("statements = %d, want 2", len(statements))
	}
	page, ok := statements[0].Stmt.(*checker.VariableDef)
	if !ok {
		t.Fatalf("page statement = %T", statements[0].Stmt)
	}
	text, ok := page.Value.(*checker.EmbeddedText)
	if !ok {
		t.Fatalf("page value = %T", page.Value)
	}
	if text.Resource.OwnerPackageIdentity != "app" || text.Resource.LogicalPath != "assets/page.txt" || string(text.Resource.Data) != string(content) {
		t.Fatalf("embedded text resource = %#v", text.Resource)
	}
	if text.Type() != checker.Str {
		t.Fatalf("embedded text type = %s", text.Type())
	}

	raw, ok := statements[1].Stmt.(*checker.VariableDef)
	if !ok {
		t.Fatalf("raw statement = %T", statements[1].Stmt)
	}
	bytes, ok := raw.Value.(*checker.EmbeddedBytes)
	if !ok {
		t.Fatalf("raw value = %T", raw.Value)
	}
	if bytes.Resource.OwnerPackageIdentity != "app" || bytes.Resource.LogicalPath != "assets/page.txt" || string(bytes.Resource.Data) != string(content) {
		t.Fatalf("embedded bytes resource = %#v", bytes.Resource)
	}
	list, ok := bytes.Type().(*checker.List)
	if !ok || list.Of() != checker.Byte {
		t.Fatalf("embedded bytes type = %s", bytes.Type())
	}
}

func TestEmbedTextRejectsInvalidUTF8(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "invalid.bin"), []byte{0xff}, 0o644); err != nil {
		t.Fatal(err)
	}
	checked := checkEmbedSource(t, root, "use ard/embed\nlet value = embed::text(\"invalid.bin\")\n")
	if !hasEmbedDiagnostic(checked, checker.DiagnosticCodeEmbedTextUTF8) {
		t.Fatalf("missing UTF-8 diagnostic: %#v", checked.Diagnostics())
	}
}

func TestEmbedExactFilesRequireStaticLiteralPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checked := checkEmbedSource(t, root, "use ard/embed\nlet path = \"asset.txt\"\nlet value = embed::bytes(path)\n")
	if !hasEmbedDiagnostic(checked, checker.DiagnosticCodeEmbedStaticArgument) {
		t.Fatalf("missing static argument diagnostic: %#v", checked.Diagnostics())
	}
}

func TestEmbedExactFilesRejectMissingFilesAndSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checked := checkEmbedSource(t, root, "use ard/embed\nlet value = embed::bytes(\"missing.bin\")\n")
	if !hasEmbedDiagnostic(checked, checker.DiagnosticCodeEmbedResource) {
		t.Fatalf("missing resource diagnostic: %#v", checked.Diagnostics())
	}

	target := filepath.Join(root, "target.bin")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link.bin")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	checked = checkEmbedSource(t, root, "use ard/embed\nlet value = embed::bytes(\"link.bin\")\n")
	if !hasEmbedDiagnostic(checked, checker.DiagnosticCodeEmbedResource) {
		t.Fatalf("missing symlink diagnostic: %#v", checked.Diagnostics())
	}
}

func TestEmbedConstructorsCannotBeFunctionValues(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	checked := checkEmbedSource(t, root, "use ard/embed\nlet reader = embed::text\n")
	if !hasEmbedDiagnostic(checked, checker.DiagnosticCodeEmbedStaticArgument) {
		t.Fatalf("missing function-value diagnostic: %#v", checked.Diagnostics())
	}
}

func TestEmbedFSExpandsDirectoriesAndAllPatterns(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"public/index.html":           "index",
		"public/app.js":               "app",
		"public/.well-known/info.txt": "hidden",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	source := `use ard/embed
let normal = embed::fs(["public"])
let complete = embed::fs(["all:public"])
fn read(path: Str) [Byte]!Error { normal.read_file(path) }
fn read_text(path: Str) Str!Error { normal.read_text(path) }
fn subset(path: Str) embed::FS!Error { normal.sub(path) }
`
	checked := checkEmbedSource(t, root, source)
	if checked.HasErrors() {
		t.Fatalf("checker diagnostics: %#v", checked.Diagnostics())
	}
	statements := checked.Module().Program().Statements
	normal := statements[0].Stmt.(*checker.VariableDef).Value.(*checker.EmbeddedFSValue)
	complete := statements[1].Stmt.(*checker.VariableDef).Value.(*checker.EmbeddedFSValue)
	paths := func(set checker.EmbeddedFileSet) []string {
		result := make([]string, len(set.Entries))
		for index, entry := range set.Entries {
			result[index] = entry.LogicalPath
		}
		return result
	}
	if got := strings.Join(paths(normal.Set), ","); got != "public/app.js,public/index.html" {
		t.Fatalf("normal embedded paths = %q", got)
	}
	if got := strings.Join(paths(complete.Set), ","); got != "public/.well-known/info.txt,public/app.js,public/index.html" {
		t.Fatalf("complete embedded paths = %q", got)
	}
}

func TestEmbedResourcesResolveFromDependencyPackageRoot(t *testing.T) {
	workspace := t.TempDir()
	appRoot := filepath.Join(workspace, "app")
	depRoot := filepath.Join(workspace, "dep")
	for _, dir := range []string{appRoot, depRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(appRoot, "ard.toml"):  "name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n",
		filepath.Join(appRoot, "asset.txt"): "consumer",
		filepath.Join(depRoot, "ard.toml"):  "name = \"dep\"\nard = \">= 0.1.0\"\n",
		filepath.Join(depRoot, "asset.txt"): "dependency",
		filepath.Join(depRoot, "dep.ard"):   "use ard/embed\nfn value() Str { embed::text(\"asset.txt\") }\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	checked := checkEmbedSource(t, appRoot, "use dep\nfn main() Str { dep::value() }\n")
	if checked.HasErrors() {
		t.Fatalf("checker diagnostics: %#v", checked.Diagnostics())
	}
	var dependency checker.Module
	for path, imported := range checked.Module().Program().Imports {
		if strings.HasSuffix(path, "/dep") {
			dependency = imported
			break
		}
	}
	if dependency == nil {
		t.Fatalf("dependency imports = %#v", checked.Module().Program().Imports)
	}
	function := dependency.Program().Statements[0].Expr.(*checker.FunctionDef)
	resource := function.Body.Stmts[0].Expr.(*checker.EmbeddedText).Resource
	if string(resource.Data) != "dependency" || resource.OwnerPackageIdentity != "dep" {
		t.Fatalf("dependency embedded resource = %#v", resource)
	}
}

func TestEmbedFSIsNotAValidMapKey(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	checked := checkEmbedSource(t, root, "use ard/embed\nlet files = embed::fs([\"asset.txt\"])\nlet table: [embed::FS:Str] = [files: \"value\"]\n")
	if !checked.HasErrors() {
		t.Fatal("embed::FS map key was accepted")
	}
}

func TestEmbedFSRejectsDynamicAndUnmatchedPatterns(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	dynamic := checkEmbedSource(t, root, "use ard/embed\nlet patterns = [\"asset.txt\"]\nlet files = embed::fs(patterns)\n")
	if !hasEmbedDiagnostic(dynamic, checker.DiagnosticCodeEmbedStaticArgument) {
		t.Fatalf("missing dynamic-pattern diagnostic: %#v", dynamic.Diagnostics())
	}
	unmatched := checkEmbedSource(t, root, "use ard/embed\nlet files = embed::fs([\"missing/*\"])\n")
	if !hasEmbedDiagnostic(unmatched, checker.DiagnosticCodeEmbedResource) {
		t.Fatalf("missing unmatched-pattern diagnostic: %#v", unmatched.Diagnostics())
	}
	malformed := checkEmbedSource(t, root, "use ard/embed\nlet files = embed::fs([\"[bad\"])\n")
	if !hasEmbedDiagnostic(malformed, checker.DiagnosticCodeEmbedResource) {
		t.Fatalf("missing malformed-pattern diagnostic: %#v", malformed.Diagnostics())
	}
}
