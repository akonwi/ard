package checker_test

import (
	"os"
	"path/filepath"
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
	if text.Resource.OwnerPackageIdentity != "root" || text.Resource.LogicalPath != "assets/page.txt" || string(text.Resource.Data) != string(content) {
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
	if bytes.Resource.OwnerPackageIdentity != "root" || bytes.Resource.LogicalPath != "assets/page.txt" || string(bytes.Resource.Data) != string(content) {
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
