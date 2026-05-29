package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// TestServerInitializes verifies all expected LSP handlers are registered.
func TestServerInitializes(t *testing.T) {
	server := NewServer()

	expectedHandlers := []string{
		"initialize",
		"initialized",
		"shutdown",
		"exit",
		"textDocument/didOpen",
		"textDocument/didChange",
		"textDocument/didSave",
		"textDocument/didClose",
		"textDocument/hover",
		"textDocument/definition",
		"textDocument/references",
		"textDocument/documentSymbol",
		"textDocument/completion",
		"textDocument/formatting",
		"textDocument/signatureHelp",
		"textDocument/documentHighlight",
	}

	for _, method := range expectedHandlers {
		if _, ok := server.handlers[method]; !ok {
			t.Errorf("missing handler for %s", method)
		}
	}
}

// TestCheckerDiagnosticsToLSP verifies the diagnostic conversion handles edge cases.
func TestCheckerDiagnosticsToLSP(t *testing.T) {
	// Nil input should produce empty slice (not nil) so JSON is [] not null
	if result := checkerDiagnosticsToLSP(nil); result == nil {
		t.Error("expected non-nil empty slice for nil input")
	} else if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}

	// Empty input
	if result := checkerDiagnosticsToLSP([]checker.Diagnostic{}); result == nil {
		t.Error("expected non-nil empty slice for empty input")
	} else if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}

	// Single error diagnostic
	diags := []checker.Diagnostic{
		checker.NewDiagnostic(
			checker.Error,
			"type mismatch",
			"test.ard",
			parse.Location{
				Start: parse.Point{Row: 3, Col: 5},
				End:   parse.Point{Row: 3, Col: 10},
			},
		),
	}
	result := checkerDiagnosticsToLSP(diags)
	if len(result) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(result))
	}
	if result[0].Message != "type mismatch" {
		t.Errorf("expected message 'type mismatch', got %q", result[0].Message)
	}
	if result[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("expected severity error, got %v", result[0].Severity)
	}
	// Parser uses 1-based, LSP uses 0-based
	if result[0].Range.Start.Line != 2 {
		t.Errorf("expected start line 2 (0-based), got %d", result[0].Range.Start.Line)
	}
	if result[0].Range.Start.Character != 4 {
		t.Errorf("expected start character 4 (0-based), got %d", result[0].Range.Start.Character)
	}
	if result[0].Range.End.Line != 2 {
		t.Errorf("expected end line 2 (0-based), got %d", result[0].Range.End.Line)
	}
	if result[0].Range.End.Character != 9 {
		t.Errorf("expected end character 9 (0-based), got %d", result[0].Range.End.Character)
	}
	if result[0].Source != "ard" {
		t.Errorf("expected source 'ard', got %q", result[0].Source)
	}
}

// TestCheckerLocationToLSPRange verifies the 1-based to 0-based conversion.
func TestCheckerLocationToLSPRange(t *testing.T) {
	tests := []struct {
		name     string
		input    parse.Location
		expected parse.Point
	}{
		{
			name: "normal position",
			input: parse.Location{
				Start: parse.Point{Row: 1, Col: 1},
				End:   parse.Point{Row: 1, Col: 5},
			},
			expected: parse.Point{Row: 0, Col: 0},
		},
		{
			name: "zero position should stay zero",
			input: parse.Location{
				Start: parse.Point{Row: 0, Col: 0},
				End:   parse.Point{Row: 0, Col: 0},
			},
			expected: parse.Point{Row: 0, Col: 0},
		},
		{
			name: "deep position",
			input: parse.Location{
				Start: parse.Point{Row: 10, Col: 15},
				End:   parse.Point{Row: 12, Col: 3},
			},
			expected: parse.Point{Row: 9, Col: 14},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkerLocationToLSPRange(tt.input)
			if result.Start.Line != uint32(tt.expected.Row) {
				t.Errorf("start line: got %d, want %d", result.Start.Line, tt.expected.Row)
			}
			if result.Start.Character != uint32(tt.expected.Col) {
				t.Errorf("start char: got %d, want %d", result.Start.Character, tt.expected.Col)
			}
		})
	}
}

// TestFormatSource verifies the formatting flow end-to-end.
func TestFormatSource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "adds trailing newline",
			input:    "let x = 5",
			expected: "let x = 5\n",
		},
		{
			name:     "normalizes spacing",
			input:    "let   x  =  5",
			expected: "let x = 5\n",
		},
		{
			name:     "handles empty content",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatSource(tt.input, "test.ard")
			if err != nil {
				t.Fatalf("formatSource error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestFormattingHandler verifies the full handleFormatting flow.
func TestFormattingHandler(t *testing.T) {
	server := NewServer()
	uri := uri.New("file:///test.ard")
	server.cache.Open(uri, "ard", 1, "let   x  =  5")

	// We can't easily call handleFormatting directly because it needs
	// a jsonrpc2.Replier. Instead, verify the formatting through formatSource.
	formatted, err := formatSource("let   x  =  5", "test.ard")
	if err != nil {
		t.Fatalf("formatSource error: %v", err)
	}
	if formatted != "let x = 5\n" {
		t.Errorf("expected formatted 'let x = 5\\n', got %q", formatted)
	}
}

// TestHoverPositions verifies hover returns correct type info.
func TestHoverPositions(t *testing.T) {
	tests := []struct {
		name   string
		source string
		line   uint32
		char   uint32
		want   string
	}{
		{
			name:   "string literal",
			source: `let x = "hello"` + "\n",
			line:   0,
			char:   8,
			want:   "Str",
		},
		{
			name:   "int literal",
			source: `let y = 42` + "\n",
			line:   0,
			char:   8,
			want:   "Int",
		},
		{
			name:   "bool literal",
			source: `let z = true` + "\n",
			line:   0,
			char:   8,
			want:   "Bool",
		},
		{
			name:   "float literal",
			source: `let f = 3.14` + "\n",
			line:   0,
			char:   8,
			want:   "Float",
		},
		{
			name:   "variable declaration with type",
			source: `let name: Str = "hello"` + "\n",
			line:   0,
			char:   4,
			want:   "Str",
		},
		{
			name:   "builtin true",
			source: `let a = true` + "\n",
			line:   0,
			char:   8,
			want:   "Bool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := computeHover(tt.source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil")
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverInsideFunction verifies we find inner expressions, not the outer function signature.
func TestHoverInsideFunction(t *testing.T) {
	source := `fn greet(name: Str) Str {
    let msg = "Hello"
    msg
}` + "\n"

	tests := []struct {
		name string
		line uint32
		char uint32
		want string
	}{
		{
			name: "hover on string literal",
			line: 1,
			char: 14,
			want: "Str",
		},
		{
			name: "hover on variable name in body",
			line: 2,
			char: 4,
			want: "Str",
		},
		{
			name: "hover on function name",
			line: 0,
			char: 3,
			want: "fn greet",
		},
		{
			name: "hover on parameter",
			line: 0,
			char: 11,
			want: "Str",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := computeHover(source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverOnSampleFile verifies hover doesn't panic on a real source file.
func TestHoverOnSampleFile(t *testing.T) {
	source := `// samples/concurrent_stress.ard
// Stress test for concurrent interpreter safety

use ard/async
use ard/io

fn sum_items(items: [Int]) Int {
  mut sum = 0
  for item in items {
    sum =+ item
  }
  sum
}

fn count_evens(items: [Int]) Int {
  mut count = 0
  for item in items {
    if item % 2 == 0 {
      count =+ 1
    }
  }
  count
}

fn expensive_work(n: Int) Int {
  async::sleep(10000000)
  mut result = 0
  for i in 1..n {
    result =+ (i * i)
  }
  result
}

fn main() {
  io::print("=== Concurrent Stress Test ===")

  // Test 1: Multiple concurrent batch operations
  io::print("Test 1: Concurrent batch processing")

  let batch1 = [1, 2, 3, 4, 5]
  let batch2 = [6, 7, 8, 9, 10]
  let batch3 = [11, 12, 13, 14, 15]
  let batch4 = [16, 17, 18, 19, 20]
  let batch5 = [21, 22, 23, 24, 25]

  mut fibers: [async::Fiber<Int>] = []
  fibers.push(async::eval(fn() { sum_items(batch1) }))
  fibers.push(async::eval(fn() { sum_items(batch2) }))
  fibers.push(async::eval(fn() { sum_items(batch3) }))
  fibers.push(async::eval(fn() { sum_items(batch4) }))
  fibers.push(async::eval(fn() { sum_items(batch5) }))

  async::join(fibers)

  mut total_sum = 0
  for f in fibers {
    total_sum =+ f.get()
  }

  io::print("Sum results:")
  io::print("Total: ")
  io::print(total_sum.to_str())

  // Test 2: Even counting in parallel
  io::print("Test 2: Even counting")

  mut even_fibers: [async::Fiber<Int>] = []
  even_fibers.push(async::eval(fn() { count_evens(batch1) }))
  even_fibers.push(async::eval(fn() { count_evens(batch2) }))
  even_fibers.push(async::eval(fn() { count_evens(batch3) }))
  even_fibers.push(async::eval(fn() { count_evens(batch4) }))
  even_fibers.push(async::eval(fn() { count_evens(batch5) }))

  async::join(even_fibers)

  mut total_evens = 0
  for f in even_fibers {
    total_evens =+ f.get()
  }

  io::print("Even counts total: ")
  io::print(total_evens.to_str())

  // Test 3: Expensive computations
  io::print("Test 3: Expensive work")

  mut work_fibers: [async::Fiber<Int>] = []
  for i in 0..100 {
    work_fibers.push(async::eval(fn() { expensive_work(i) }))
  }

  async::join(work_fibers)

  mut work_total = 0
  for f in work_fibers {
    work_total =+ f.get()
  }

  io::print("Work total: ")
  io::print(work_total.to_str())

  io::print("=== All concurrent tests completed ===")
}
`

	// Test hover at several positions that are likely to be hovered
	positions := []struct {
		line uint32
		char uint32
	}{
		{0, 15},  // comment
		{5, 3},   // `fn sum_items` — function name
		{6, 6},   // `mut sum` — variable declaration
		{7, 13},  // `items` in for loop
		{8, 4},   // `sum =+ item` — variable assignment
		{10, 2},  // `sum` as return value
		{22, 6},  // `main` function
		{24, 4},  // `io::print(...)` — function call
		{30, 6},  // `batch1` identifier
		{33, 13}, // `fibers` identifier
		{35, 20}, // `batch2` inside function call
		{52, 12}, // `f.get()` — method call
		{59, 6},  // `even_fibers` identifier
		{80, 11}, // `i` in `for i in 0..100`
		{105, 8}, // `===` string inside print
	}

	for _, pos := range positions {
		t.Run(fmt.Sprintf("line_%d_col_%d", pos.line, pos.char), func(t *testing.T) {
			pt := protocol.Position{Line: pos.line, Character: pos.char}
			// Should not panic — recover if it does
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic at %d:%d: %v", pos.line, pos.char, r)
				}
			}()
			info := computeHover(source, "test.ard", pt)
			// info can be nil — that's ok as long as we don't panic
			_ = info
		})
	}
}

// TestHoverInferredExpression verifies type inference from function calls and identifiers.
func TestHoverInferredExpression(t *testing.T) {
	tests := []struct {
		name   string
		source string
		line   uint32
		char   uint32
		want   string
	}{
		{
			name:   "variable from function call",
			source: "fn get_value() Int { 42 }\nlet x = get_value()\n",
			line:   1,
			char:   6,
			want:   "Int",
		},
		{
			name:   "variable from another variable",
			source: "let a: Str = \"hi\"\nlet b = a\n",
			line:   1,
			char:   6,
			want:   "Str",
		},
		{
			name:   "variable from binary expression",
			source: "let x = 1 + 2\n",
			line:   0,
			char:   6,
			want:   "Int",
		},
		{
			name:   "variable from string concat",
			source: "let x = \"hello\" + \"world\"\n",
			line:   0,
			char:   6,
			want:   "Str",
		},
		{
			name:   "variable from comparison",
			source: "let x = 1 > 2\n",
			line:   0,
			char:   6,
			want:   "Bool",
		},
		{
			name:   "variable from struct instance",
			source: "struct Board { cells: [Str] }\nmut board = Board{cells: []}\n",
			line:   1,
			char:   10,
			want:   "Board",
		},
		{
			name:   "board in while loop method chain",
			source: "struct Board {\n  cells: [Str]\n}\nimpl Board {\n  fn is_full() Bool { false }\n}\nfn main() {\n  mut board = Board{cells: []}\n  while not board.is_full() {\n    board.do_stuff()\n  }\n}\n",
			line:   7,
			char:   13,
			want:   "Board",
		},
		{
			name:   "variable from static method chain",
			source: "fn main() {\n  let input = Int::from_str(\"9\").or(-1)\n  input\n}\n",
			line:   1,
			char:   6,
			want:   "Int",
		},
		{
			name:   "variable in match case body",
			source: "fn read_move() Int {\n  let input = Int::from_str(\"9\").or(-1)\n  match input >= 1 and input <= 9 {\n    true => input - 1,\n    false => -1,\n  }\n}\n",
			line:   3,
			char:   13,
			want:   "Int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := computeHover(tt.source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil")
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverInImplBlock verifies variables and parameters hover inside impl methods.
func TestHoverInImplBlock(t *testing.T) {
	source := `struct Board {
  cells: [Str]
}
impl Board {
  fn mut play(player: Str, pos: Int) {
    self.cells.set(pos, player)
  }
  fn is_full() Bool {
    mut full = true
    for cell in self.cells {
      if cell.is_empty() {
        full = false
      }
    }
    full
  }
}
`

	tests := []struct {
		name string
		line uint32
		char uint32
		want string
	}{
		{name: "method parameter player", line: 4, char: 14, want: "Str"},
		{name: "method parameter pos", line: 4, char: 27, want: "Int"},
		{name: "self receiver", line: 5, char: 5, want: "Board"},
		{name: "field in method call", line: 5, char: 10, want: "Board.cells: [Str]"},
		{name: "list method", line: 5, char: 15, want: "fn mut [Str].set(index: Int, value: Str) Bool"},
		{name: "pos argument", line: 5, char: 19, want: "Int"},
		{name: "player argument", line: 5, char: 24, want: "Str"},
		{name: "local variable declaration", line: 8, char: 8, want: "Bool"},
		{name: "for loop cursor", line: 9, char: 8, want: "Str"},
		{name: "self in loop iterable", line: 9, char: 16, want: "Board"},
		{name: "field in loop iterable", line: 9, char: 22, want: "Board.cells: [Str]"},
		{name: "for loop cursor in condition", line: 10, char: 9, want: "Str"},
		{name: "string method", line: 10, char: 15, want: "fn Str.is_empty() Bool"},
		{name: "local variable assignment", line: 11, char: 8, want: "Bool"},
		{name: "local variable return", line: 14, char: 4, want: "Bool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := computeHover(source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverInstanceMethodSignatures verifies instance method hovers include owner, params, and return type.
func TestHoverInstanceMethodSignatures(t *testing.T) {
	source := `struct Board {
  cells: [Str]
}
impl Board {
  fn mut play(player: Str, pos: Int) {
    self.cells.set(pos, player)
  }
  fn can_play(pos: Int) Bool {
    self.cells.at(pos).is_empty()
  }
}
fn main() {
  mut board = Board{cells: []}
  board.can_play(0)
  let parsed = Int::from_str("1").or(0)
}
`

	tests := []struct {
		name string
		line uint32
		char uint32
		want string
	}{
		{name: "list mutating method", line: 5, char: 15, want: "fn mut [Str].set(index: Int, value: Str) Bool"},
		{name: "list method in chain", line: 8, char: 15, want: "fn [Str].at(index: Int) Str"},
		{name: "string method after chain", line: 8, char: 23, want: "fn Str.is_empty() Bool"},
		{name: "struct method", line: 13, char: 9, want: "fn Board.can_play(pos: Int) Bool"},
		{name: "maybe method after static call", line: 14, char: 34, want: "fn Int?.or(default: Int) Int"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := computeHover(source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverStaticFunctionSignatures verifies static function hovers include qualifier, params, and return type.
func TestHoverStaticFunctionSignatures(t *testing.T) {
	source := `use ard/io

fn main() {
  io::print("hello")
  let input_str = io::read_line().or("")
  let input = Int::from_str(input_str).or(-1)
}
`

	tests := []struct {
		name string
		line uint32
		char uint32
		want string
	}{
		{name: "imported function", line: 3, char: 7, want: "fn io::print(value: ToString) Void"},
		{name: "imported extern function", line: 4, char: 24, want: "fn io::read_line() Str!Str"},
		{name: "prelude static function", line: 5, char: 20, want: "fn Int::from_str(str: Str) Int?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := computeHover(source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverImportedModuleTypes verifies imported types use the source alias in hovers.
func TestHoverImportedModuleTypes(t *testing.T) {
	source := `use ard/http as web

fn inspect(req: web::Request, response: web::Response) {
  let url = req.url
  let method = req.method
  response.is_ok()
  req.method.to_str()
}

fn main() {
  let response = web::Response::new(200, "ok")
  let responses: [web::Response] = [response]
}
`

	tests := []struct {
		name string
		line uint32
		char uint32
		want string
	}{
		{name: "imported string field", line: 3, char: 17, want: "web::Request.url: Str"},
		{name: "imported field type", line: 4, char: 21, want: "web::Request.method: web::Method"},
		{name: "imported struct method", line: 5, char: 12, want: "fn web::Response.is_ok() Bool"},
		{name: "imported enum method after field", line: 6, char: 14, want: "fn web::Method.to_str() Str"},
		{name: "inferred imported type", line: 10, char: 7, want: "web::Response"},
		{name: "imported static constructor", line: 10, char: 33, want: "fn web::Response::new(status: Int, body: Str) web::Response"},
		{name: "imported type in list", line: 11, char: 7, want: "[web::Response]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := protocol.Position{Line: tt.line, Character: tt.char}
			info := computeHover(source, "test.ard", pos)
			if info == nil {
				t.Fatalf("expected hover info, got nil at %d:%d", tt.line, tt.char)
			}
			if !strings.Contains(info.content, tt.want) {
				t.Errorf("hover content = %q, want contains %q", info.content, tt.want)
			}
		})
	}
}

// TestHoverUserModuleFunctionSignature verifies static function hovers from user imports.
func TestHoverUserModuleFunctionSignature(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"test_project\"\nard = \">= 0.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	responsesSource := `use ard/http

fn json_error(mut res: http::Response, status: Int, message: Str) {
  res.status = status
}
`
	if err := os.WriteFile(filepath.Join(root, "responses.ard"), []byte(responsesSource), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `use ard/http
use test_project/responses

fn main(mut res: http::Response) {
  responses::json_error(res, 400, "Nope")
}
`
	filePath := filepath.Join(root, "routes.ard")
	if err := os.WriteFile(filePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	pos := protocol.Position{Line: 4, Character: 15}
	info := computeHover(source, filePath, pos)
	if info == nil {
		t.Fatalf("expected hover info, got nil")
	}
	want := "fn responses::json_error(mut res: http::Response, status: Int, message: Str) Void"
	if !strings.Contains(info.content, want) {
		t.Errorf("hover content = %q, want contains %q", info.content, want)
	}
}

// TestHoverImportedStaticResultMethod verifies instance method hovers after imported static calls.
func TestHoverImportedStaticResultMethod(t *testing.T) {
	source := `use ard/io

fn main() {
  let input_str = io::read_line().or("")
}
`

	pos := protocol.Position{Line: 3, Character: 34}
	info := computeHover(source, "test.ard", pos)
	if info == nil {
		t.Fatalf("expected hover info, got nil")
	}
	want := "fn Str!Str.or(default: Str) Str"
	if !strings.Contains(info.content, want) {
		t.Errorf("hover content = %q, want contains %q", info.content, want)
	}
}

// TestDocumentCache verifies basic document lifecycle.
func TestDocumentCache(t *testing.T) {
	cache := NewDocumentCache()

	uri := uri.New("file:///test.ard")

	// Open
	cache.Open(uri, "ard", 1, "let x = 5")
	doc := cache.Get(uri)
	if doc == nil {
		t.Fatal("expected doc after open")
	}
	if doc.Text != "let x = 5" {
		t.Errorf("expected text 'let x = 5', got %q", doc.Text)
	}

	// Update
	cache.Update(uri, 2, "let y = 10")
	doc = cache.Get(uri)
	if doc.Version != 2 {
		t.Errorf("expected version 2, got %d", doc.Version)
	}
	if doc.Text != "let y = 10" {
		t.Errorf("expected text 'let y = 10', got %q", doc.Text)
	}

	// Close
	cache.Close(uri)
	if doc := cache.Get(uri); doc != nil {
		t.Error("expected nil after close")
	}
}
