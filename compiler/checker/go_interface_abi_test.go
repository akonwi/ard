package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

// writeGoInterfaceABIPackage provides Go interfaces whose methods take a
// struct by value and by pointer, to pin the mut-parameter ABI rules.
func writeGoInterfaceABIPackage(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := `package ffi

type Payload struct {
	N int
}

type ValueTaker interface {
	Take(p Payload)
}

type PointerTaker interface {
	Take(p *Payload)
}

type Variadic interface {
	Count(values ...int) int
}

type EmptyResult interface {
	Ready() (struct{}, error)
}

type EmptyMaybe interface {
	Lookup() (struct{}, bool)
}

type Namer interface {
	Name() string
}

type URLer interface {
	URL() string
}

type EmbeddedNamer interface {
	Namer
}
`
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestGoInterfaceMutParameterABI pins how `mut` parameters interact with Go
// interface method ABIs after mutability became type syntax:
//
//   - a Go method expecting value Payload rejects `mut ffi::Payload` (the
//     pointer form changes the ABI) — reported as a type mismatch;
//   - a Go method expecting *Payload accepts `mut ffi::Payload` (mutation
//     flows through the pointer the ABI already carries);
//   - native mut parameters still trip the dedicated ABI diagnostic.
func TestGoInterfaceMutParameterABI(t *testing.T) {
	root := t.TempDir()
	writeGoInterfaceABIPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)

	tests := []struct {
		name        string
		input       string
		diagnostics []checker.Diagnostic
	}{
		{
			name: "value-taking Go method rejects mut foreign struct param",
			input: `use go:example.com/app/ffi

struct Impl {}

impl ffi::ValueTaker for Impl {
  fn take(p: mut ffi::Payload) {
  }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected ffi::Payload, got mut ffi::Payload"}},
		},
		{
			name: "pointer-taking Go method accepts mut foreign struct param",
			input: `use go:example.com/app/ffi

struct Impl {}

impl ffi::PointerTaker for Impl {
  fn take(p: mut ffi::Payload) {
    p.N = 1
  }
}`,
		},
		{
			name: "variadic Go interface implementation is rejected",
			input: `use go:example.com/app/ffi

struct Impl {}

impl ffi::Variadic for Impl {
  fn count(value: Int) Int { value }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported Go interface method ffi::Variadic.Count: variadic Go interface methods cannot be implemented by fixed-arity Ard declarations"}},
		},
		{
			name: "empty Result Go interface implementation is rejected",
			input: `use go:example.com/app/ffi

struct Impl {}

impl ffi::EmptyResult for Impl {
  fn ready() Void!Str { Result::ok(()) }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported Go interface method ffi::EmptyResult.Ready: Go interface methods returning (struct{}, error) require an unsupported empty-success ABI adapter"}},
		},
		{
			name: "empty Maybe Go interface implementation is rejected",
			input: `use go:example.com/app/ffi

struct Impl {}

impl ffi::EmptyMaybe for Impl {
  fn lookup() Void? { Maybe::new(()) }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Unsupported Go interface method ffi::EmptyMaybe.Lookup: Go interface methods returning (struct{}, bool) require an unsupported empty-success ABI adapter"}},
		},
		{
			name: "generated field rejects required Go method selector collision",
			input: `use go:example.com/app/ffi

struct User { name: Str }

impl ffi::Namer for User {
  fn name() Str { self.name }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Ard property 'User.name' lowers to Go field 'Name', which conflicts with Go interface method 'Name'"}},
		},
		{
			name: "colliding implementation is not recorded as interface conformance",
			input: `use go:example.com/app/ffi

struct User { name: Str }

impl ffi::Namer for User {
  fn name() Str { self.name }
}

fn consume(value: ffi::Namer) {}
fn main() { consume(User{name: "Ada"}) }`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Ard property 'User.name' lowers to Go field 'Name', which conflicts with Go interface method 'Name'"},
				{Kind: checker.Error, Message: "Type mismatch: Expected ffi::Namer, got User"},
			},
		},
		{
			name: "acronym field rejects exact Go method selector collision",
			input: `use go:example.com/app/ffi

struct Resource { u_r_l: Str }

impl ffi::URLer for Resource {
  fn u_r_l() Str { self.u_r_l }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Ard property 'Resource.u_r_l' lowers to Go field 'URL', which conflicts with Go interface method 'URL'"}},
		},
		{
			name: "differently cased field does not collide with acronym method",
			input: `use go:example.com/app/ffi

struct Resource { url: Str }

impl ffi::URLer for Resource {
  fn u_r_l() Str { self.url }
}`,
		},
		{
			name: "embedded Go interface method rejects field collision",
			input: `use go:example.com/app/ffi

struct User { name: Str }

impl ffi::EmbeddedNamer for User {
  fn name() Str { self.name }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Ard property 'User.name' lowers to Go field 'Name', which conflicts with Go interface method 'Name'"}},
		},
		{
			name: "private struct field still occupies exported Go selector",
			input: `use go:example.com/app/ffi

private struct user { name: Str }

impl ffi::Namer for user {
  fn name() Str { self.name }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Ard property 'user.name' lowers to Go field 'Name', which conflicts with Go interface method 'Name'"}},
		},
		{
			name: "generic struct field rejects method selector collision",
			input: `use go:example.com/app/ffi

struct Record { name: $T }

impl ffi::Namer for Record {
  fn name() Str { "record" }
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Ard property 'Record.name' lowers to Go field 'Name', which conflicts with Go interface method 'Name'"}},
		},
		{
			name: "native mut parameter still trips the ABI diagnostic",
			input: `use go:example.com/app/ffi

struct Impl {}

impl ffi::ValueTaker for Impl {
  fn take(p: mut Int) {
  }
}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected ffi::Payload, got mut Int"},
				{Kind: checker.Error, Message: "Go interface method 'take' parameter 'p' cannot be mutable because it would change the Go ABI"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse error: %s", result.Errors[0].Message)
			}
			c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
			c.Check()
			if len(tt.diagnostics) > 0 || c.HasErrors() {
				if diff := cmp.Diff(tt.diagnostics, c.Diagnostics(), compareOptions); diff != "" {
					t.Fatalf("diagnostics mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestGoMethodFieldCollisionHasStructuredDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeGoInterfaceABIPackage(t, root)
	resolver := checker.NewGoPackagesResolver(root, nil)
	result := parse.Parse([]byte(`use go:example.com/app/ffi

struct User { name: Str }

impl ffi::Namer for User {
  fn name() Str { self.name }
}`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	var fieldLocation parse.Location
	var methodLocation parse.Location
	for _, stmt := range result.Program.Statements {
		switch stmt := stmt.(type) {
		case *parse.StructDefinition:
			fieldLocation = stmt.Fields[0].Name.GetLocation()
		case *parse.TraitImplementation:
			methodLocation = stmt.Methods[0].GetLocation()
		}
	}

	c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %#v, want one", c.Diagnostics())
	}
	diagnostic := c.Diagnostics()[0]
	if diagnostic.Code != checker.DiagnosticCodeGoMethodFieldCollision || diagnostic.Title != "Ard property conflicts with Go interface method" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if diagnostic.Primary.Span.Location != methodLocation {
		t.Fatalf("primary location = %#v, want %#v", diagnostic.Primary.Span.Location, methodLocation)
	}
	if len(diagnostic.Secondary) != 1 || diagnostic.Secondary[0].Span.Location != fieldLocation {
		t.Fatalf("secondary labels = %#v, want field at %#v", diagnostic.Secondary, fieldLocation)
	}
}
