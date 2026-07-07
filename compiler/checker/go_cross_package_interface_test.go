package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/google/go-cmp/cmp"
)

// TestGoInterfaceSatisfactionAcrossPackageLoads pins that Go interface
// assignability holds when the interface and the implementing type come from
// different imported packages. Per ADR 0044 the checker primes the resolver
// with the whole Go import set in one go/packages session, so named types
// referenced in interface method signatures (e.g. http.ResponseWriter) share
// one go/types identity and plain assignability holds.
func TestGoInterfaceSatisfactionAcrossPackageLoads(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		diagnostics []checker.Diagnostic
	}{
		{
			name: "pointer implementer from a different package satisfies the interface",
			// *httputil.ReverseProxy implements http.Handler across package
			// boundaries; identity holds because both packages load in the
			// same primed session.
			input: `use go:net/http
use go:net/url
use go:net/http/httputil

fn main() Void!Str {
  let target = try url::Parse("http://localhost:9")
  let proxy = httputil::NewSingleHostReverseProxy(target)
  http::ListenAndServe(":0", proxy)
  Result::ok(())
}`,
		},
		{
			name: "value form without pointer-receiver method set is still rejected",
			// ReverseProxy's ServeHTTP has a pointer receiver, so the
			// immutable value form must not satisfy http.Handler.
			input: `use go:net/http
use go:net/http/httputil

fn main() {
  let proxy = httputil::ReverseProxy{}
  http::ListenAndServe(":0", proxy)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected http::Handler, got httputil::ReverseProxy"}},
		},
		{
			name: "non-implementing pointer type from a different package is still rejected",
			input: `use go:net/http
use go:strings

fn main() {
  let reader = strings::NewReader("body")
  http::ListenAndServe(":0", reader)
}`,
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "Type mismatch: Expected http::Handler, got mut strings::Reader"}},
		},
		{
			name: "same named type across packages is assignable",
			input: `use go:net/http
use go:net/http/httptest

fn main() {
  mut proxy = http::NewServeMux()
  let server = httptest::NewServer(proxy)
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := checker.NewGoPackagesResolver(t.TempDir(), nil)
			runCrossPackageCase(t, resolver, tt.input, tt.diagnostics)
		})
	}
}

// TestGoInterfaceSatisfactionSymmetricDirection pins interface satisfaction
// when the interface's package imports the implementer's vocabulary but the
// two top-level packages do not import each other.
func TestGoInterfaceSatisfactionSymmetricDirection(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module wrappermod\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "ffi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ffi", "ffi.go"), []byte(`package ffi

import "net/http"

type Wrapper interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

func Accept(w Wrapper) {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver := checker.NewGoPackagesResolver(root, nil)
	input := `use go:net/http
use go:wrappermod/ffi

fn main() {
  let mux = http::NewServeMux()
  ffi::Accept(mux)
}`
	runCrossPackageCase(t, resolver, input, nil)
}

func runCrossPackageCase(t *testing.T, resolver *checker.GoPackagesResolver, input string, diagnostics []checker.Diagnostic) {
	t.Helper()
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
	c.Check()
	if len(diagnostics) > 0 || c.HasErrors() {
		if diff := cmp.Diff(diagnostics, c.Diagnostics(), compareOptions); diff != "" {
			t.Fatalf("diagnostics mismatch (-want +got):\n%s", diff)
		}
	}
}
