package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestExternType(t *testing.T) {
	run(t, []test{
		{
			name: "extern type can be declared",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
			}, "\n"),
		},
		{
			name: "private extern type can be declared",
			input: strings.Join([]string{
				`private extern type ConnectionPtr`,
			}, "\n"),
		},
		{
			name: "extern type can be used in extern fn signatures",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern fn connect(cs: Str) ConnectionPtr = "SqlConnect"`,
				`extern fn close(db: ConnectionPtr) Void = "SqlClose"`,
			}, "\n"),
		},
		{
			name: "extern type can be used as struct field",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`struct Database {`,
				`  ptr: ConnectionPtr,`,
				`  path: Str,`,
				`}`,
			}, "\n"),
		},
		{
			name: "extern type can be wrapped in Maybe",
			input: strings.Join([]string{
				`extern type RawRequest`,
				`extern fn get_req(r: RawRequest?) Str = "GetReq"`,
			}, "\n"),
		},
		{
			name: "extern type can be wrapped in Result",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern fn connect(cs: Str) ConnectionPtr!Str = "SqlConnect"`,
			}, "\n"),
		},
		{
			name: "extern type can be used in lists",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern fn get_all() [ConnectionPtr] = "GetAll"`,
			}, "\n"),
		},
		{
			name: "extern type cannot be compared with ==",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern fn connect(cs: Str) ConnectionPtr = "SqlConnect"`,
				`let a = connect("a")`,
				`let b = connect("b")`,
				`let same = a == b`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Invalid: ConnectionPtr == ConnectionPtr"},
			},
		},
		{
			name: "duplicate extern type declaration",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern type ConnectionPtr`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Duplicate declaration: ConnectionPtr"},
			},
		},
		{
			name: "different extern types are not interchangeable",
			input: strings.Join([]string{
				`extern type ConnectionPtr`,
				`extern type TransactionPtr`,
				`extern fn connect(cs: Str) ConnectionPtr = "SqlConnect"`,
				`extern fn close(db: TransactionPtr) Void = "SqlClose"`,
				`let conn = connect("x")`,
				`close(conn)`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected TransactionPtr, got ConnectionPtr"},
			},
		},
	})
}

func TestImportedExternTypeIsVisible(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "helpers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "helpers", "promise.ard"), []byte("extern type Promise\nextern fn resolved() Promise = \"Resolved\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	result := parse.Parse([]byte("use demo/helpers/promise as promise\nextern fn keep(p: promise::Promise) promise::Promise = \"Keep\"\n"), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected parse error: %s", result.Errors[0].Message)
	}

	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		messages := make([]string, 0, len(c.Diagnostics()))
		for _, d := range c.Diagnostics() {
			messages = append(messages, d.String())
		}
		t.Fatalf("unexpected diagnostics:\n%s", strings.Join(messages, "\n"))
	}
}
