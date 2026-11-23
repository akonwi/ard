package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

// Issue #620: an imported pointer alias resolves, so its use site links to
// the alias declaration and values typed with it hover as the pointer type.
func TestImportedPointerAliasDefinitionAndHover(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	libPath := filepath.Join(root, "lib.ard")
	if err := os.WriteFile(libPath, []byte(`struct Box<$T> {
  v: $T,
}

type Inbox = &mut Box<Int>
`), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/lib

fn bump(inbox: lib::Inbox) {
  inbox.v = 1
}
`
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	loc := requireDefinition(t, source, filePath, 2, 21)
	assertDefinitionStart(t, loc, libPath, 4, 5)

	srv, docURI := spanServer(t, source, filePath)
	info := srv.hoverFromSpans(context.Background(), docURI, protocol.Position{Line: 3, Character: 3})
	if info == nil || !strings.Contains(info.content, "&mut Box<Int>") {
		t.Fatalf("hover = %#v, want the alias's pointer type", info)
	}
}

// Imported type aliases are type names, whatever they name, so module member
// completion offers them as types rather than values or functions.
func TestModuleMemberCompletionOffersTypeAliasesAsTypes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "lib.ard"), []byte(`struct Box<$T> {
  v: $T,
}

type Inbox = &mut Box<Int>
type Handler = fn(Int) Str
type Name = Str
type Plain = Box<Int>
let greeting = "hi"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use test_project/lib

fn main() {
  lib::
}
`
	filePath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	srv, docURI := spanServer(t, source, filePath)
	items := srv.completionFromSpans(context.Background(), docURI, source, protocol.Position{Line: 3, Character: 7})
	for _, tc := range []struct {
		label  string
		kind   protocol.CompletionItemKind
		detail string
	}{
		{"Inbox", protocol.CompletionItemKindClass, "&mut Box<Int>"},
		{"Handler", protocol.CompletionItemKindClass, "fn(Int) Str"},
		{"Name", protocol.CompletionItemKindClass, "Str"},
		{"Plain", protocol.CompletionItemKindStruct, ""},
		{"Box", protocol.CompletionItemKindStruct, ""},
		{"greeting", protocol.CompletionItemKindValue, "Str"},
	} {
		item, ok := completionItemByLabel(items, tc.label)
		if !ok {
			t.Fatalf("completion %q not found in %#v", tc.label, items)
		}
		if item.Kind != tc.kind || tc.detail != "" && item.Detail != tc.detail {
			t.Errorf("completion %q = kind %v detail %q, want kind %v detail %q", tc.label, item.Kind, item.Detail, tc.kind, tc.detail)
		}
	}
}
