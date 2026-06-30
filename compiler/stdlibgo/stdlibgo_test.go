package stdlibgo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializedDirWritesCompleteModule(t *testing.T) {
	dir, err := MaterializedDir()
	if err != nil {
		t.Fatalf("MaterializedDir: %v", err)
	}
	for _, rel := range []string{"go.mod", "go.sum", "runtime/maybe.go"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected materialized %s: %v", rel, err)
		}
	}
	marker, err := os.ReadFile(filepath.Join(dir, markerName))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if string(marker) != ContentHash() {
		t.Fatalf("marker %q != content hash %q", marker, ContentHash())
	}
}

func TestMaterializedDirIsIdempotent(t *testing.T) {
	first, err := MaterializedDir()
	if err != nil {
		t.Fatalf("MaterializedDir: %v", err)
	}
	second, err := MaterializedDir()
	if err != nil {
		t.Fatalf("MaterializedDir again: %v", err)
	}
	if first != second {
		t.Fatalf("expected stable dir, got %q then %q", first, second)
	}
}

func TestIsBundledImportPath(t *testing.T) {
	bundled := []string{
		"github.com/akonwi/ard/runtime",
	}
	for _, p := range bundled {
		if !IsBundledImportPath(p) {
			t.Errorf("expected %q to be bundled", p)
		}
	}
	notBundled := []string{
		"math",
		"github.com/akonwi/ard",
		"github.com/akonwi/ard/checker",
		"github.com/akonwi/ard/std_lib/ffi/sub",
	}
	for _, p := range notBundled {
		if IsBundledImportPath(p) {
			t.Errorf("expected %q to not be bundled", p)
		}
	}
}
