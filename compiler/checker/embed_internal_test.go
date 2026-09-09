package checker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedExactFileSnapshotIsCachedPerResolver(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "asset.txt")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := resolver.resolveEmbeddedExactFile("app/main", "asset.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := resolver.resolveEmbeddedExactFile("app/main", "asset.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Data) != "first" || string(second.Data) != "first" {
		t.Fatalf("cached snapshots = %q, %q", first.Data, second.Data)
	}
}

func TestEmbeddedExactFileEnforcesProgramFileLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxEmbeddedProgramFileCount; i++ {
		resolver.embeddedResources[string(rune(i))+"\x00"] = EmbeddedResource{}
	}
	if _, err := resolver.resolveEmbeddedExactFile("app/main", "asset.txt"); err == nil {
		t.Fatal("expected program file limit error")
	}
}

func TestEmbeddedExactFileAllowsReservedNameWhenItIsAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vendor"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := resolver.resolveEmbeddedExactFile("app/main", "vendor")
	if err != nil {
		t.Fatal(err)
	}
	if string(resource.Data) != "file" {
		t.Fatalf("resource data = %q", resource.Data)
	}
}
