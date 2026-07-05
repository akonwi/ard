package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// spanHover runs hover through the snapshot/span path with a real temp file.
func spanHover(t *testing.T, source string, line, char uint32) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ard")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	docURI := uri.File(path)
	s.cache.Open(docURI, "ard", 1, source)
	info := s.hoverFromSpans(context.Background(), docURI, protocol.Position{Line: line, Character: char})
	if info == nil {
		return ""
	}
	return info.content
}

func requireSpanHover(t *testing.T, source string, line, char uint32, want string) {
	t.Helper()
	content := spanHover(t, source, line, char)
	if !strings.Contains(content, want) {
		t.Errorf("hover at %d:%d = %q, want contains %q", line, char, content, want)
	}
}

// Ported parity scenario: TestHoverImportedStaticResultMethod (previously
// failing on the legacy path). Result-method hovers render full signatures.
func TestSpanHoverResultMethod(t *testing.T) {
	source := `fn read() Str!Str {
  Result::ok("x")
}

fn main() {
  let input = read().or("")
}
`
	requireSpanHover(t, source, 5, 23, "fn Str!Str.or(default: Str) Str")
}

// Ported parity scenario: TestHoverTraitMethodReference. The original used
// the removed Str::ToString prelude trait; a user trait covers the same
// behavior: trait-typed receiver method hovers render full signatures.
func TestSpanHoverTraitMethod(t *testing.T) {
	source := `trait Render {
  fn describe() Str
}

fn render(value: Render) Str {
  value.describe()
}
`
	requireSpanHover(t, source, 5, 9, "fn Render.describe() Str")
}

// Ported parity scenarios from TestHoverInstanceMethodSignatures.
func TestSpanHoverInstanceMethodSignatures(t *testing.T) {
	source := `struct Board {
  cells: [Str],
}
impl Board {
  fn mut play(player: Str, pos: Int) {
    self.cells.set(pos, player)
  }
  fn can_play(pos: Int) Bool {
    self.cells.at(pos).expect("bounds").is_empty()
  }
}
fn main() {
  mut board = Board{cells: []}
  board.can_play(0)
  let maybe_cell: Str? = board.cells.at(0)
  let cell = maybe_cell.or("")
}
`
	t.Run("list mutating method", func(t *testing.T) {
		requireSpanHover(t, source, 5, 15, "fn mut [Str].set(index: Int, value: Str)")
	})
	t.Run("list at returns Maybe", func(t *testing.T) {
		requireSpanHover(t, source, 8, 15, "fn [Str].at(index: Int) Str?")
	})
	t.Run("string method after chain", func(t *testing.T) {
		requireSpanHover(t, source, 8, 40, "fn Str.is_empty() Bool")
	})
	t.Run("struct method", func(t *testing.T) {
		requireSpanHover(t, source, 13, 9, "fn Board.can_play(pos: Int) Bool")
	})
	t.Run("maybe method", func(t *testing.T) {
		requireSpanHover(t, source, 15, 25, "fn Str?.or(default: Str) Str")
	})
}

// Ported parity scenarios: expression and variable hovers.
func TestSpanHoverExpressions(t *testing.T) {
	source := `struct Board {
  cells: [Str],
}
fn main() {
  let msg = "hello"
  mut board = Board{cells: []}
  let n = 1 + 2
  let eq = 1 > 2
}
`
	t.Run("string literal", func(t *testing.T) {
		requireSpanHover(t, source, 4, 13, "Str")
	})
	t.Run("struct variable", func(t *testing.T) {
		requireSpanHover(t, source, 5, 7, "Board")
	})
	t.Run("field access hover shows owner and type", func(t *testing.T) {
		source := `struct Board {
  cells: [Str],
}
impl Board {
  fn size() Int {
    self.cells.size()
  }
}
`
		requireSpanHover(t, source, 5, 10, "Board.cells: [Str]")
	})
}

// TestSpanHoverFunctionDeclaration renders the signature when hovering the
// declaration name itself.
func TestSpanHoverFunctionDeclaration(t *testing.T) {
	source := `fn greet(name: Str) Str {
  let msg = "Hello"
  msg
}
`
	requireSpanHover(t, source, 0, 3, "fn greet(name: Str) Str")
}
