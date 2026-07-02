package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestGoPackagesResolverResolvesStdlibWithoutGoMod(t *testing.T) {
	resolver := checker.NewGoPackagesResolver(t.TempDir(), nil)
	pkg, err := resolver.ResolveGoPackage("fmt")
	if err != nil {
		t.Fatalf("ResolveGoPackage(fmt): %v", err)
	}
	if pkg.Functions["Println"] == nil {
		t.Fatal("fmt.Println missing from resolved package")
	}
}

func TestGoPackagesResolverResolvesLocalModulePackage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(`package ffi

func Greet(name string) string { return "hello " + name }
`), 0644); err != nil {
		t.Fatal(err)
	}
	resolver := checker.NewGoPackagesResolver(root, nil)
	pkg, err := resolver.ResolveGoPackage("example.com/app/ffi")
	if err != nil {
		t.Fatalf("ResolveGoPackage(local ffi): %v", err)
	}
	fn := pkg.Functions["Greet"]
	if fn == nil {
		t.Fatal("Greet missing from resolved package")
	}
	if got := fn.ReturnType.String(); got != "Str" {
		t.Fatalf("Greet return = %s, want Str", got)
	}
}

func TestGoPackagesResolverUsesBuildTags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte("package ffi\n\nfunc Always() string { return \"always\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "tagged.go"), []byte("//go:build special\n\npackage ffi\n\nfunc Tagged() string { return \"tagged\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	withoutTag := checker.NewGoPackagesResolver(root, nil)
	pkg, err := withoutTag.ResolveGoPackage("example.com/app/ffi")
	if err != nil {
		t.Fatalf("ResolveGoPackage without tags: %v", err)
	}
	if pkg.Functions["Tagged"] != nil {
		t.Fatal("Tagged should not be visible without build tag")
	}
	withTag := checker.NewGoPackagesResolver(root, []string{"special"})
	pkg, err = withTag.ResolveGoPackage("example.com/app/ffi")
	if err != nil {
		t.Fatalf("ResolveGoPackage with tags: %v", err)
	}
	if pkg.Functions["Tagged"] == nil {
		t.Fatal("Tagged should be visible with build tag")
	}
}
