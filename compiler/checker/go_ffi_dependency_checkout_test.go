package checker_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
)

// TestGoPackagesResolverResolvesDependencyFFIFromCheckout pins issue #353: a
// git dependency's Go FFI must resolve from the same locked checkout as its Ard
// source, not from whatever version the consuming project's go.mod happens to
// pin. Otherwise bumping a dependency (advancing ard.lock) while go.mod lags
// makes the checker validate the dependency's new Ard source against an old FFI
// package, producing spurious "Undefined Go function" errors.
func TestGoPackagesResolverResolvesDependencyFFIFromCheckout(t *testing.T) {
	// stale mirrors the version the consumer's go.mod points at: it lacks the
	// newly added FFI function.
	stale := t.TempDir()
	writeGoModule(t, stale, "example.com/dep", `package ffi

func Old() string { return "old" }
`)
	// checkout mirrors the ard.lock git checkout: the current dependency source,
	// including the new FFI function its Ard source now calls.
	checkout := t.TempDir()
	writeGoModule(t, checkout, "example.com/dep", `package ffi

func Old() string { return "old" }
func NewFn() string { return "new" }
`)

	consumer := t.TempDir()
	goMod := fmt.Sprintf("module consumer\n\ngo 1.21\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => %s\n", stale)
	if err := os.WriteFile(filepath.Join(consumer, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	// Baseline (the bug): resolving through the consumer's module graph sees the
	// stale FFI, so the new function is missing.
	baseline := checker.NewGoPackagesResolver(consumer, nil)
	if err := baseline.Prime([]string{"example.com/dep/ffi"}); err != nil {
		t.Fatalf("baseline Prime: %v", err)
	}
	basePkg, err := baseline.ResolveGoPackage("example.com/dep/ffi")
	if err != nil {
		t.Fatalf("baseline resolve: %v", err)
	}
	if basePkg.Functions["NewFn"] != nil {
		t.Fatal("precondition failed: stale go.mod version unexpectedly already has NewFn")
	}

	// The fix: told where the dependency's locked checkout lives, the resolver
	// sources its FFI from there, matching the dependency's Ard source commit.
	resolver := checker.NewGoPackagesResolver(consumer, nil)
	resolver.DependencyModuleRoots = map[string]string{"example.com/dep": checkout}
	if err := resolver.Prime([]string{"example.com/dep/ffi"}); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	pkg, err := resolver.ResolveGoPackage("example.com/dep/ffi")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if pkg.Functions["NewFn"] == nil {
		t.Fatal("dependency FFI did not resolve from the locked checkout: NewFn missing")
	}
	if pkg.Functions["Old"] == nil {
		t.Fatal("dependency FFI lost an existing function after checkout redirection")
	}
}

// TestGoPackagesResolverResolvesPathDependencyFFIWithoutConsumerGoModule pins
// issue #437: a path dependency's Go module must be available to the checker
// even when the consumer is outside that module and has no go.mod of its own.
// Otherwise nested consumers pass accidentally through Go's parent-module
// lookup while equivalent sibling projects fail to resolve the same FFI.
func TestGoPackagesResolverResolvesPathDependencyFFIWithoutConsumerGoModule(t *testing.T) {
	root := t.TempDir()
	dependency := filepath.Join(root, "dependency")
	consumer := filepath.Join(root, "consumer")
	if err := os.MkdirAll(dependency, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(consumer, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoModule(t, dependency, "example.com/dep", `package ffi

func Value() string { return "ok" }
`)

	project := &checker.ProjectInfo{Dependencies: map[string]checker.DependencyInfo{
		"dep": {
			Alias:      "dep",
			SourcePath: dependency,
			RootPath:   dependency,
		},
	}}
	resolver := checker.NewGoPackagesResolver(consumer, nil)
	resolver.DependencyModuleRoots = checker.DependencyGoModuleRoots(project)
	if err := resolver.Prime([]string{"example.com/dep/ffi"}); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	pkg, err := resolver.ResolveGoPackage("example.com/dep/ffi")
	if err != nil {
		t.Fatalf("resolve path dependency FFI: %v", err)
	}
	if pkg.Functions["Value"] == nil {
		t.Fatal("path dependency FFI did not expose Value")
	}
}

func writeGoModule(t *testing.T, root, modulePath, ffiSource string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(fmt.Sprintf("module %s\n\ngo 1.21\n", modulePath)), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(root, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte(ffiSource), 0o644); err != nil {
		t.Fatal(err)
	}
}
