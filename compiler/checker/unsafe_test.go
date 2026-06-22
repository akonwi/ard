package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

func TestUnsafeBlocks(t *testing.T) {
	run(t, []test{
		{
			name: "unsafe block types as result",
			input: `fn safe() Int!Str {
  unsafe {
    42
  }
}`,
		},
		{
			name: "unsafe block rejects break",
			input: `fn bad() Int!Str {
  unsafe {
    break
    1
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "break is not allowed inside unsafe blocks"},
			},
		},
		{
			name: "unsafe block allows break in nested closures",
			input: `fn ok() Void!Str {
  unsafe {
    let stop = fn() {
      while true {
        break
      }
    }
    ()
  }
}`,
		},
		{
			name: "unsafe block rejects incompatible try catch ok result",
			input: `fn inner() Int!Str {
  Result::err("inner")
}

fn bad() Str!Str {
  unsafe {
    let value = try inner() -> err { Result::ok(5) }
    value.to_str()
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str!Str, got Int!$Err"},
			},
		},
		{
			name: "unsafe block preserves local Result ok alias in try catch",
			input: `fn inner() Int!Str {
  Result::err("inner")
}

fn good() Int!Str {
  unsafe {
    let value = try inner() -> _ {
      let r = Result::ok(5)
      r
    }
    value
  }
}`,
		},
		{
			name: "unsafe block allows direct Go extern aliases in try catch ok result",
			input: `use go:time

extern type Time1 = "go:time::Time"
extern type Time2 = "go:time as t::Time"
extern fn now1() Time1 = time::Now
extern fn now2() Time2 = time::Now

fn inner() Int!Str {
  Result::err("inner")
}

fn good() Time1!Str {
  unsafe {
    let value = try inner() -> err { Result::ok(now2()) }
    now1()
  }
}`,
		},
		{
			name: "unsafe block rejects generic incompatible try catch ok result",
			input: `fn inner() Int!Str {
  Result::err("inner")
}

fn bad<$T>(x: $T) Str!Str {
  unsafe {
    let value = try inner() -> err { Result::ok(x) }
    value.to_str()
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str!Str, got $T!$Err"},
			},
		},
		{
			name: "unsafe block rejects generic incompatible try catch err result",
			input: `fn inner() Int!Str {
  Result::err("inner")
}

fn bad<$E>(e: $E) Str!Str {
  unsafe {
    let value = try inner() -> err { Result::err(e) }
    value.to_str()
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str!Str, got $Val!$E"},
			},
		},
		{
			name: "unsafe block rejects incompatible try catch err after non-final expression",
			input: `fn inner() Int!Str {
  Result::err("inner")
}

fn bad<$E>(e: $E) Str!Str {
  unsafe {
    let value = try inner() -> err {
      Result::ok("ignored")
      Result::err(e)
    }
    value.to_str()
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str!Str, got $Val!$E"},
			},
		},
		{
			name: "unsafe block rejects generic compound incompatible try catch ok result",
			input: `fn inner() Int!Str {
  Result::err("inner")
}

fn bad<$T>(x: $T) [$T]!Str {
  unsafe {
    let value = try inner() -> err { Result::ok([1]) }
    [x]
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected [$T]!Str, got [Int]!$Err"},
			},
		},
		{
			name: "unsafe block rejects nested incompatible try catch ok result",
			input: `fn inner() Int!Str {
  Result::err("inner")
}

fn bad() Str!Str {
  unsafe {
    let value = (try inner() -> err { Result::ok(5) }) + 1
    value.to_str()
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str!Str, got Int!$Err"},
			},
		},
	})
}

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
