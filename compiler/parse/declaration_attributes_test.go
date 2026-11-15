package parse

import "testing"

func TestTopLevelDeclarationAttributes(t *testing.T) {
	result := Parse([]byte(`use ard/io
#platform(unix)
use go:syscall
use go:os

#platform(linux, darwin_arm64)
fn watched() Int { 1 }

#platform(windows)

private fn hidden() {}

#platform(unix)
fn Point::new() Int { 1 }

#platform(unix)
let size: Int = 1

#platform(unix)
type Fd = Int

#platform(unix)
struct S {}

#platform(unix)
enum E { a }

#platform(unix)
trait T {}

#platform(unix)
impl S {}

#platform(unix)
impl T for S {}

#platform(unix)
test fn works() {}
`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	program := result.Program
	if len(program.Imports) != 3 {
		t.Fatalf("imports = %d, want 3", len(program.Imports))
	}
	if len(program.Imports[0].Attributes) != 0 || len(program.Imports[2].Attributes) != 0 {
		t.Fatalf("unattributed imports carry attributes: %#v", program.Imports)
	}
	syscallImport := program.Imports[1]
	if len(syscallImport.Attributes) != 1 || syscallImport.Attributes[0].Name.Name != "platform" {
		t.Fatalf("syscall import attributes = %#v", syscallImport.Attributes)
	}
	declarations := 0
	for _, statement := range program.Statements {
		if _, ok := statement.(*Comment); ok {
			continue
		}
		declarations++
		attributes := DeclarationAttributes(statement)
		if len(attributes) != 1 || attributes[0].Name.Name != "platform" {
			t.Fatalf("%T attributes = %#v", statement, attributes)
		}
	}
	if declarations != 11 {
		t.Fatalf("declarations = %d, want 11", declarations)
	}
	fn := program.Statements[0].(*FunctionDeclaration)
	arguments := fn.Attributes[0].Arguments
	if len(arguments) != 2 || arguments[0].Value.Text != "linux" || arguments[1].Value.Kind != AttributeSymbol || arguments[1].Value.Text != "darwin_arm64" {
		t.Fatalf("arguments = %#v", arguments)
	}
	if fn.Attributes[0].Location.Start.Row != 6 || fn.Location.Start.Row != 7 {
		t.Fatalf("attribute row = %d, declaration row = %d", fn.Attributes[0].Location.Start.Row, fn.Location.Start.Row)
	}
	if got := DeclarationStart(fn); got.Row != 6 {
		t.Fatalf("DeclarationStart = %v, want row 6", got)
	}
}

func TestAttributedImportAfterPlainImportsKeepsImportPhase(t *testing.T) {
	result := Parse([]byte("use go:os\n\n#platform(unix)\nuse go:syscall\n\nfn main() {}\n"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	if len(result.Program.Imports) != 2 || len(result.Program.Imports[1].Attributes) != 1 {
		t.Fatalf("imports = %#v", result.Program.Imports)
	}
	if len(result.Program.Statements) != 1 {
		t.Fatalf("statements = %#v", result.Program.Statements)
	}
}

func TestDeclarationAttributeErrors(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		message string
	}{
		{"not a declaration", "fn main() {}\n#platform(unix)\nmain()\n", "Attributes must be followed by a declaration"},
		{"end of file", "fn main() {}\n#platform(unix)\n", "Expected declaration after attribute"},
		{"same line", "#platform(unix) fn main() {}\n", "Expected new line after attribute"},
		{"comment between", "#platform(unix)\n// note\nfn main() {}\n", "Comments must come before attributes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.source), "test.ard")
			if len(result.Errors) == 0 || result.Errors[0].Message != tt.message {
				t.Fatalf("errors = %#v, want %q", result.Errors, tt.message)
			}
		})
	}
}
