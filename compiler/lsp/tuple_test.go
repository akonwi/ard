package lsp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestHoverOnTupleDestructuring(t *testing.T) {
	source := `fn pair() (Int, Str) {
  (1, "a")
}

fn main() {
  let (number, text) = pair()
  let copy = text
  let whole = pair()
}
`

	tests := []struct {
		name string
		line uint32
		char uint32
		want string
	}{
		{name: "first destructured name", line: 5, char: 8, want: "Int"},
		{name: "second destructured name", line: 5, char: 16, want: "Str"},
		{name: "destructured name use", line: 6, char: 14, want: "Str"},
		{name: "tuple-typed binding", line: 7, char: 7, want: "(Int, Str)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := spanHoverAt(t, source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

func TestSpanHoverStructWithTupleField(t *testing.T) {
	source := `struct Span {
  bounds: (Int, Int),
  label: (Str, Bool)?,
}
`
	content := spanHover(t, source, 0, 8)
	want := "struct Span {\n  bounds: (Int, Int),\n  label: (Str, Bool)?,\n}"
	if !strings.Contains(content, want) {
		t.Fatalf("struct hover = %q, want contains %q", content, want)
	}
}

// Destructured names are ordinary local bindings: navigation and rename treat
// each one independently of the tuple it came from.
func TestNavigationOnTupleDestructuring(t *testing.T) {
	source := `fn pair() (Int, Str) {
  (1, "a")
}

fn main() Str {
  let (number, text) = pair()
  let copy = number
  text
}
`
	filePath := filepath.Join(t.TempDir(), "tuple_nav.ard")

	t.Run("definition of a destructured name", func(t *testing.T) {
		loc := requireDefinition(t, source, filePath, 7, 3)
		assertDefinitionStart(t, loc, filePath, 5, 15)
	})

	t.Run("references to a destructured name", func(t *testing.T) {
		srv, docURI := spanServer(t, source, filePath)
		refs, err := srv.referencesFromSpans(context.Background(), docURI, protocol.Position{Line: 5, Character: 8}, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) != 2 {
			t.Fatalf("references = %#v, want the binding and one use", refs)
		}
	})

	t.Run("rename a destructured name", func(t *testing.T) {
		srv, docURI := spanServer(t, source, filePath)
		edit, err := srv.renameFromSpans(context.Background(), docURI, protocol.Position{Line: 7, Character: 3}, "label")
		if err != nil {
			t.Fatal(err)
		}
		if edit == nil {
			t.Fatal("rename returned no edit")
		}
		edits := edit.Changes[docURI]
		if len(edits) != 2 {
			t.Fatalf("rename edits = %#v, want the binding and one use", edits)
		}
		for _, textEdit := range edits {
			if textEdit.NewText != "label" {
				t.Fatalf("rename edit = %#v, want new text %q", textEdit, "label")
			}
		}
	})
}
