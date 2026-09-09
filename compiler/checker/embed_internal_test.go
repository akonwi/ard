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
	resolver.embeddedProgramFileCount = MaxEmbeddedProgramFileCount
	resource, err := resolver.resolveEmbeddedExactFile("app/main", "asset.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := resolver.accountEmbeddedExact(resource); err == nil {
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

func TestEmbeddedExactAccountingDeduplicatesIdenticalBlobs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.txt", "second.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("same"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	resolver, err := NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.txt", "second.txt"} {
		resource, err := resolver.resolveEmbeddedExactFile("app/main", name)
		if err != nil {
			t.Fatal(err)
		}
		if err := resolver.accountEmbeddedExact(resource); err != nil {
			t.Fatal(err)
		}
	}
	if resolver.embeddedProgramFileCount != 1 || resolver.embeddedProgramBytes != len("same") {
		t.Fatalf("embedded accounting = %d files, %d bytes", resolver.embeddedProgramFileCount, resolver.embeddedProgramBytes)
	}
}
