package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestReferencesModuleValueFromDefinition covers TargetValue kind derivation
// when the query starts at the module-level value's definition.
func TestReferencesModuleValueFromDefinition(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"proj\"\nard = \">= 0.1.0\"\n"), 0o644)
	valuesSource := "let api_name = \"ranger\"\n"
	valuesPath := filepath.Join(root, "values.ard")
	os.WriteFile(valuesPath, []byte(valuesSource), 0o644)
	routesSource := "use proj/values\n\nfn main() Str {\n  values::api_name\n}\n"
	routesPath := filepath.Join(root, "routes.ard")
	os.WriteFile(routesPath, []byte(routesSource), 0o644)

	srv := NewServer()
	docURI := uri.File(valuesPath)
	srv.cache.Open(docURI, "ard", 1, valuesSource)
	refs := srv.referencesFromSpans(docURI, protocol.Position{Line: 0, Character: 5}, true)
	if len(refs) != 2 {
		t.Fatalf("expected def + cross-file use, got %d: %#v", len(refs), refs)
	}
}

// TestRenameFromSpansRejectsInvalidNames guards newName validation.
func TestRenameFromSpansRejectsInvalidNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ard")
	source := "fn main() {\n  let count = 1\n  let x = count\n}\n"
	os.WriteFile(path, []byte(source), 0o644)
	srv := NewServer()
	docURI := uri.File(path)
	srv.cache.Open(docURI, "ard", 1, source)

	if edit := srv.renameFromSpans(docURI, protocol.Position{Line: 2, Character: 11}, "1 bad name!"); edit != nil {
		t.Fatal("invalid identifier accepted")
	}
	if edit := srv.renameFromSpans(docURI, protocol.Position{Line: 2, Character: 11}, "count"); edit != nil {
		t.Fatal("no-op rename produced edits")
	}
	edit := srv.renameFromSpans(docURI, protocol.Position{Line: 2, Character: 11}, "total")
	if edit == nil || len(edit.Changes[docURI]) != 2 {
		t.Fatalf("expected 2 edits for local rename, got %#v", edit)
	}
}

// TestRenameFromSpansDefersNominalEntitiesToLegacy: nominal renames need
// cross-file edits, which the span path does not produce yet.
func TestRenameFromSpansDefersNominalEntitiesToLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ard")
	source := "fn helper() Int {\n  1\n}\n\nfn main() {\n  let x = helper()\n}\n"
	os.WriteFile(path, []byte(source), 0o644)
	srv := NewServer()
	docURI := uri.File(path)
	srv.cache.Open(docURI, "ard", 1, source)

	if edit := srv.renameFromSpans(docURI, protocol.Position{Line: 5, Character: 11}, "renamed"); edit != nil {
		t.Fatal("nominal entity rename should defer to the legacy cross-file path")
	}
}

// TestCompletionImportedStructMethods guards cross-module method completion:
// imported structs keep methods in the defining module's program.
func TestCompletionImportedStructMethods(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"proj\"\nard = \">= 0.1.0\"\n"), 0o644)
	os.WriteFile(filepath.Join(root, "boxes.ard"), []byte("struct Box {\n  item: Int,\n}\n\nimpl Box {\n  fn get() Int {\n    self.item\n  }\n}\n"), 0o644)
	source := "use proj/boxes\n\nfn main(box: boxes::Box) {\n  box.\n}\n"
	path := filepath.Join(root, "main.ard")
	os.WriteFile(path, []byte(source), 0o644)

	srv := NewServer()
	docURI := uri.File(path)
	srv.cache.Open(docURI, "ard", 1, source)
	items := srv.completionFromSpans(docURI, source, protocol.Position{Line: 3, Character: 6})

	var haveField, haveMethod bool
	for _, item := range items {
		if item.Label == "item" {
			haveField = true
		}
		if item.Label == "get" {
			haveMethod = true
		}
	}
	if !haveField {
		t.Fatalf("field 'item' missing from completions: %#v", items)
	}
	if !haveMethod {
		t.Fatalf("method 'get' missing from completions (cross-module side table): %#v", items)
	}
}

// TestCompletionStructWithTraitImpl guards trait-impl methods on structs.
func TestCompletionStructWithTraitImpl(t *testing.T) {
	dir := t.TempDir()
	source := `trait Render {
  fn describe() Str
}

struct Board {
  cells: [Str],
}

impl Render for Board {
  fn describe() Str {
    "board"
  }
}

fn main(board: Board) {
  board.
}
`
	path := filepath.Join(dir, "test.ard")
	os.WriteFile(path, []byte(source), 0o644)
	srv := NewServer()
	docURI := uri.File(path)
	srv.cache.Open(docURI, "ard", 1, source)
	items := srv.completionFromSpans(docURI, source, protocol.Position{Line: 15, Character: 8})

	var haveDescribe bool
	for _, item := range items {
		if item.Label == "describe" {
			haveDescribe = true
		}
	}
	if !haveDescribe {
		t.Fatalf("trait-impl method 'describe' missing: %#v", items)
	}
}
