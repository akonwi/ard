package checker_test

import (
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

func TestDeferDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		diagnostics []checker.Diagnostic
	}{
		{
			name: "defer is allowed in script bodies",
			input: `fn cleanup() {}

defer cleanup()`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "defer is rejected in module initializer expressions",
			input: `fn cleanup() {}

let global = {
  defer cleanup()
  1
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "defer can only be used inside a function, method, closure, or script body"}},
		},
		{
			name: "try is rejected in deferred block",
			input: `fn close() Void!Str { Result::ok(()) }

fn main() Void!Str {
  defer {
    try close()
  }
  Result::ok(())
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "try is not allowed inside deferred work; handle the Result or Maybe explicitly"}},
		},
		{
			name: "try is allowed in closure defined inside deferred block",
			input: `fn close() Void!Str { Result::ok(()) }

fn main() Void!Str {
  defer {
    let attempt = fn() Void!Str {
      try close()
      Result::ok(())
    }
    attempt()
  }
  Result::ok(())
}`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "empty deferred block is rejected with a source diagnostic",
			input: `fn main() {
  defer {}
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "deferred block has no statements"}},
		},
		{
			name: "comment-only deferred block is rejected with a source diagnostic",
			input: `fn main() {
  defer {
    // cleanup will go here
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "deferred block has no statements"}},
		},
		{
			name: "try is rejected in deferred call expression",
			input: `fn close() Void!Str { Result::ok(()) }
fn cleanup(v: Void) {}

fn main() Void!Str {
  defer cleanup(try close())
  Result::ok(())
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "try is not allowed inside deferred work; handle the Result or Maybe explicitly"}},
		},
		{
			name: "defer is rejected inside unsafe block",
			input: `fn cleanup() {}

fn main() {
  let value = unsafe {
    defer cleanup()
    1
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "defer is not allowed inside unsafe blocks; move it outside the unsafe block"}},
		},
		{
			name: "defer block is a break boundary",
			input: `fn cleanup() {}

fn main() {
  while true {
    defer {
      break
    }
    break
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "break can only be used inside a loop"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %#v", result.Errors)
			}
			c := checker.New("test.ard", result.Program, nil)
			c.Check()
			if diff := cmp.Diff(tt.diagnostics, c.Diagnostics(), compareOptions); diff != "" {
				t.Fatalf("diagnostics mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
