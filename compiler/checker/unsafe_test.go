package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

func TestUnsafeCatchValidationDoesNotSpecialCaseUserResultModules(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"ardmodtest\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "my"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "my", "result.ard"), []byte(`fn err() Int!Str {
  Result::ok(5)
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(tempDir, "main.ard")
	result := parse.Parse([]byte(`use ardmodtest/my/result as result

fn inner() Int!Str {
  Result::err("inner")
}

fn bad() Str!Str {
  unsafe {
    let value = try inner() -> _ { result::err() }
    value.to_str()
  }
}`), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, result.Program, resolver)
	c.Check()
	want := []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected Str!Str, got Int!Str"}}
	if diff := cmp.Diff(want, c.Diagnostics(), compareOptions); diff != "" {
		t.Fatalf("Diagnostics mismatch (-want +got):\n%s", diff)
	}
}
