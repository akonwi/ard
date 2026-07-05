package lsp

import (
	"context"
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
	refs := srv.referencesFromSpans(context.Background(), docURI, protocol.Position{Line: 0, Character: 5}, true)
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

	if edit := srv.renameFromSpans(context.Background(), docURI, protocol.Position{Line: 2, Character: 11}, "1 bad name!"); edit != nil {
		t.Fatal("invalid identifier accepted")
	}
	if edit := srv.renameFromSpans(context.Background(), docURI, protocol.Position{Line: 2, Character: 11}, "count"); edit != nil {
		t.Fatal("no-op rename produced edits")
	}
	edit := srv.renameFromSpans(context.Background(), docURI, protocol.Position{Line: 2, Character: 11}, "total")
	if edit == nil || len(edit.Changes[docURI]) != 2 {
		t.Fatalf("expected 2 edits for local rename, got %#v", edit)
	}
}

// TestRenameFromSpansNominalSameFile renames a function and its call sites
// within one file, with the declaration edit covering only the identifier.
func TestRenameFromSpansNominalSameFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ard")
	source := "fn helper() Int {\n  1\n}\n\nfn main() {\n  let x = helper()\n}\n"
	os.WriteFile(path, []byte(source), 0o644)
	srv := NewServer()
	docURI := uri.File(path)
	srv.cache.Open(docURI, "ard", 1, source)

	edit := srv.renameFromSpans(context.Background(), docURI, protocol.Position{Line: 5, Character: 11}, "renamed")
	if edit == nil {
		t.Fatal("expected rename edit")
	}
	edits := edit.Changes[docURI]
	if len(edits) != 2 {
		t.Fatalf("expected declaration + call edits, got %#v", edits)
	}
	if edits[0].Range.Start.Line != 0 || edits[0].Range.Start.Character != 3 {
		t.Fatalf("declaration edit should target the identifier, got %#v", edits[0].Range)
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
	items := srv.completionFromSpans(context.Background(), docURI, source, protocol.Position{Line: 3, Character: 6})

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
	items := srv.completionFromSpans(context.Background(), docURI, source, protocol.Position{Line: 15, Character: 8})

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

// TestRenameFromSpansCrossFile verifies nominal renames edit every file that
// references the entity.
func TestRenameFromSpansCrossFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"proj\"\nard = \">= 0.1.0\"\n"), 0o644)
	mathSource := "fn inc(value: Int) Int {\n  value + 1\n}\n"
	mathPath := filepath.Join(root, "math.ard")
	os.WriteFile(mathPath, []byte(mathSource), 0o644)
	mainSource := "use proj/math\n\nfn main() Int {\n  math::inc(1)\n}\n"
	mainPath := filepath.Join(root, "main.ard")
	os.WriteFile(mainPath, []byte(mainSource), 0o644)

	srv := NewServer()
	docURI := uri.File(mainPath)
	srv.cache.Open(docURI, "ard", 1, mainSource)

	edit := srv.renameFromSpans(context.Background(), docURI, protocol.Position{Line: 3, Character: 9}, "bump")
	if edit == nil {
		t.Fatal("expected cross-file rename edit")
	}
	mathEdits := edit.Changes[uri.File(mathPath)]
	mainEdits := edit.Changes[uri.File(mainPath)]
	if len(mathEdits) != 1 {
		t.Fatalf("expected 1 edit in math.ard (the declaration), got %#v", edit.Changes)
	}
	if len(mainEdits) != 1 {
		t.Fatalf("expected 1 edit in main.ard (the call), got %#v", edit.Changes)
	}
	if mathEdits[0].NewText != "bump" || mainEdits[0].NewText != "bump" {
		t.Fatal("wrong replacement text")
	}
	// The declaration edit must cover exactly the identifier.
	if mathEdits[0].Range.Start.Character != 3 || mathEdits[0].Range.End.Character != 6 {
		t.Fatalf("declaration edit range imprecise: %#v", mathEdits[0].Range)
	}
}

// TestRangeHoldsIdentifierGuard exercises the rename verification guard
// directly: only ranges holding exactly the identifier verify, so any
// unverifiable range aborts the whole rename (no partial edits).
func TestRangeHoldsIdentifierGuard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ard")
	source := "fn helper() Int {\n  1\n}\n"
	os.WriteFile(path, []byte(source), 0o644)
	srv := NewServer()
	docURI := uri.File(path)
	srv.cache.Open(docURI, "ard", 1, source)

	mk := func(line, start, end uint32) protocol.Location {
		return protocol.Location{
			URI: protocol.DocumentURI(docURI),
			Range: protocol.Range{
				Start: protocol.Position{Line: line, Character: start},
				End:   protocol.Position{Line: line, Character: end},
			},
		}
	}
	if !srv.rangeHoldsIdentifier(mk(0, 3, 9), "helper") {
		t.Fatal("exact identifier range failed verification")
	}
	if srv.rangeHoldsIdentifier(mk(0, 0, 9), "helper") {
		t.Fatal("range including the fn keyword must not verify")
	}
	if srv.rangeHoldsIdentifier(mk(0, 3, 8), "helper") {
		t.Fatal("truncated range must not verify")
	}
	if srv.rangeHoldsIdentifier(protocol.Location{
		URI: protocol.DocumentURI(docURI),
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 3},
			End:   protocol.Position{Line: 2, Character: 1},
		},
	}, "helper") {
		t.Fatal("multi-line range must not verify")
	}
}

// TestSignatureHelpFromSpans covers the span-based signature path: function,
// method, and builtin calls with active-parameter tracking.
func TestSignatureHelpFromSpans(t *testing.T) {
	dir := t.TempDir()
	source := `struct Board {
  cells: [Str],
}
impl Board {
  fn play(player: Str, pos: Int) Bool {
    true
  }
}

fn configure(width: Int, height: Int, title: Str) {}

fn main(board: Board) {
  configure(80, 24, "demo")
  board.play("x", 0)
}
`
	path := filepath.Join(dir, "test.ard")
	os.WriteFile(path, []byte(source), 0o644)
	srv := NewServer()
	docURI := uri.File(path)
	srv.cache.Open(docURI, "ard", 1, source)

	t.Run("function second arg", func(t *testing.T) {
		help := srv.signatureHelpFromSpans(context.Background(), docURI, source, protocol.Position{Line: 12, Character: 17})
		if help == nil || len(help.Signatures) == 0 {
			t.Fatal("no signature help")
		}
		if want := "fn configure(width: Int, height: Int, title: Str)"; help.Signatures[0].Label != want {
			t.Fatalf("label = %q, want %q", help.Signatures[0].Label, want)
		}
		if help.ActiveParameter != 1 {
			t.Fatalf("active parameter = %d, want 1", help.ActiveParameter)
		}
	})
	t.Run("method first arg", func(t *testing.T) {
		help := srv.signatureHelpFromSpans(context.Background(), docURI, source, protocol.Position{Line: 13, Character: 14})
		if help == nil || len(help.Signatures) == 0 {
			t.Fatal("no signature help")
		}
		if want := "fn Board.play(player: Str, pos: Int) Bool"; help.Signatures[0].Label != want {
			t.Fatalf("label = %q, want %q", help.Signatures[0].Label, want)
		}
		if help.ActiveParameter != 0 {
			t.Fatalf("active parameter = %d, want 0", help.ActiveParameter)
		}
	})
}
