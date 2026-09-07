package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportCompletionIncludesDeclaredBuildModule(t *testing.T) {
	root := t.TempDir()
	manifest := "name = \"app\"\nard = \">= 0.40.0\"\n\n[build.values]\nversion = { type = \"Str\", default = \"dev\" }\n"
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "main.ard")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	items := importPathCompletionItems("ard/b", file)
	if len(items) != 1 || items[0].Label != "build" {
		t.Fatalf("items = %#v, want build", items)
	}
}

func TestImportCompletionOmitsBuildModuleForDependencyFile(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	dependency := filepath.Join(workspace, "dep")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dependency, 0o755); err != nil {
		t.Fatal(err)
	}
	rootManifest := "name = \"app\"\nard = \">= 0.40.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n\n[build.values]\nversion = { type = \"Str\", default = \"app\" }\n"
	dependencyManifest := "name = \"dep\"\nard = \">= 0.40.0\"\n\n[build.values]\nversion = { type = \"Str\", default = \"dep\" }\n"
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte(rootManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependency, "ard.toml"), []byte(dependencyManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	items := importPathCompletionItemsWithRoot("ard/b", filepath.Join(dependency, "dep.ard"), root)
	for _, item := range items {
		if item.Label == "build" {
			t.Fatalf("items unexpectedly include build: %#v", items)
		}
	}
}

func TestImportCompletionOmitsUndeclaredBuildModule(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.40.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	items := importPathCompletionItems("ard/b", filepath.Join(root, "main.ard"))
	for _, item := range items {
		if item.Label == "build" {
			t.Fatalf("items unexpectedly include build: %#v", items)
		}
	}
}
