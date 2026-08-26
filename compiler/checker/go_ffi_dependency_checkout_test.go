package checker_test

import (
	"archive/zip"
	"fmt"
	gotypes "go/types"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

func TestGoPackagesResolverRetriesIncompleteDependencySumsPrivately(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("HOME", cacheRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("LOCALAPPDATA", cacheRoot)
	proxy := t.TempDir()
	writeGoProxyModule(t, proxy, "example.com/transitive", "v1.0.0", map[string]string{
		"go.mod":   "module example.com/transitive\n\ngo 1.27\n",
		"value.go": "package transitive\n\ntype Value string\n",
	})
	proxyPath := filepath.ToSlash(proxy)
	if !strings.HasPrefix(proxyPath, "/") {
		proxyPath = "/" + proxyPath
	}
	t.Setenv("GOPROXY", (&url.URL{Scheme: "file", Path: proxyPath}).String())
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "")
	t.Setenv("GONOSUMDB", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOPACKAGESDRIVER", "off")
	moduleCache := t.TempDir()
	t.Setenv("GOMODCACHE", moduleCache)
	t.Cleanup(func() {
		_ = filepath.Walk(moduleCache, func(path string, _ os.FileInfo, _ error) error {
			_ = os.Chmod(path, 0o700)
			return nil
		})
	})

	dependency := t.TempDir()
	dependencyMod := "module example.com/dep\n\ngo 1.27\n\nrequire example.com/transitive v1.0.0\n"
	if err := os.WriteFile(filepath.Join(dependency, "go.mod"), []byte(dependencyMod), 0o644); err != nil {
		t.Fatal(err)
	}
	modelDir := filepath.Join(dependency, "ffi", "model")
	producerDir := filepath.Join(dependency, "ffi", "producer")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(producerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modelSource := `package model

import "example.com/transitive"

type Item struct { Value transitive.Value }
`
	if err := os.WriteFile(filepath.Join(modelDir, "model.go"), []byte(modelSource), 0o644); err != nil {
		t.Fatal(err)
	}
	producerSource := `package producer

import "example.com/dep/ffi/model"

func Make() model.Item { return model.Item{} }
`
	if err := os.WriteFile(filepath.Join(producerDir, "producer.go"), []byte(producerSource), 0o644); err != nil {
		t.Fatal(err)
	}

	consumer := t.TempDir()
	consumerMod := "module example.com/app\n\ngo 1.27\n"
	if err := os.WriteFile(filepath.Join(consumer, "go.mod"), []byte(consumerMod), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver := checker.NewGoPackagesResolver(consumer, nil)
	resolver.DependencyModuleRoots = map[string]string{"example.com/dep": dependency}
	if err := resolver.Prime([]string{"example.com/dep/ffi/producer", "example.com/dep/ffi/model"}); err != nil {
		t.Fatalf("Prime: %v", err)
	}
	producer, err := resolver.ResolveGoPackage("example.com/dep/ffi/producer")
	if err != nil {
		t.Fatalf("resolve producer with incomplete sums: %v", err)
	}
	model, err := resolver.ResolveGoPackage("example.com/dep/ffi/model")
	if err != nil {
		t.Fatalf("resolve model with incomplete sums: %v", err)
	}
	makeFn := producer.Functions["Make"]
	if makeFn == nil {
		t.Fatal("dependency FFI did not expose Make after private module retry")
	}
	returned, ok := makeFn.ReturnType.(*checker.ForeignType)
	if !ok {
		t.Fatalf("Make return = %T, want *checker.ForeignType", producer.Functions["Make"].ReturnType)
	}
	declared, ok := model.Types["Item"].(*checker.ForeignType)
	if !ok {
		t.Fatalf("model.Item = %T, want *checker.ForeignType", model.Types["Item"])
	}
	if !gotypes.Identical(returned.GoType, declared.GoType) {
		t.Fatalf("retry split the shared Go type universe: %v != %v", returned.GoType, declared.GoType)
	}
	if _, err := os.Stat(filepath.Join(consumer, "go.sum")); !os.IsNotExist(err) {
		t.Fatalf("consumer go.sum was created or stat failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dependency, "go.sum")); !os.IsNotExist(err) {
		t.Fatalf("dependency go.sum was created or stat failed: %v", err)
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

func writeGoProxyModule(t *testing.T, proxyRoot, modulePath, version string, files map[string]string) {
	t.Helper()
	versionDir := filepath.Join(proxyRoot, filepath.FromSlash(modulePath), "@v")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "list"), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, version+".info"), []byte(fmt.Sprintf("{\"Version\":%q,\"Time\":\"2026-01-01T00:00:00Z\"}\n", version)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, version+".mod"), []byte(files["go.mod"]), 0o644); err != nil {
		t.Fatal(err)
	}
	archive, err := os.Create(filepath.Join(versionDir, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(archive)
	prefix := modulePath + "@" + version + "/"
	for name, content := range files {
		entry, err := zipWriter.Create(prefix + name)
		if err != nil {
			_ = archive.Close()
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			_ = archive.Close()
			t.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		_ = archive.Close()
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
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
