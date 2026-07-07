package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestGoPackagesResolverRejectsProjectLocalPackageOutsideFFI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	internalDir := filepath.Join(root, "internal")
	if err := os.MkdirAll(internalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(internalDir, "internal.go"), []byte("package internal\n\nfunc Secret() string { return \"secret\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	resolver := checker.NewGoPackagesResolver(root, nil)
	if err := resolver.Prime([]string{"example.com/app/internal"}); err != nil {
		t.Fatalf("Prime(%s): %v", "example.com/app/internal", err)
	}
	_, err := resolver.ResolveGoPackage("example.com/app/internal")
	if err == nil {
		t.Fatal("expected project-local package outside ffi to be rejected")
	}
	if !strings.Contains(err.Error(), "outside the FFI boundary") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGoPackagesResolverReportsMalformedGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("not a go mod"), 0644); err != nil {
		t.Fatal(err)
	}
	resolver := checker.NewGoPackagesResolver(root, nil)
	if err := resolver.Prime([]string{"fmt"}); err != nil {
		t.Fatalf("Prime(%s): %v", "fmt", err)
	}
	_, err := resolver.ResolveGoPackage("fmt")
	if err == nil {
		t.Fatal("expected malformed go.mod error")
	}
	if !strings.Contains(err.Error(), "read go.mod") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGoPackagesResolverAllowsProjectLocalPackageUnderFFI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi", "http")
	if err := os.MkdirAll(ffiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "http.go"), []byte("package http\n\nfunc Serve() string { return \"ok\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	resolver := checker.NewGoPackagesResolver(root, nil)
	if err := resolver.Prime([]string{"example.com/app/ffi/http"}); err != nil {
		t.Fatalf("Prime(%s): %v", "example.com/app/ffi/http", err)
	}
	pkg, err := resolver.ResolveGoPackage("example.com/app/ffi/http")
	if err != nil {
		t.Fatalf("ResolveGoPackage(ffi/http): %v", err)
	}
	if pkg.Functions["Serve"] == nil {
		t.Fatal("Serve missing from resolved package")
	}
}
