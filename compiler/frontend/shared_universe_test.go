package frontend

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadModuleReportsWarningsWithoutFailing(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"warnings\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "main.ard"), []byte("let value: Void? = Maybe::new()\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStderr := os.Stderr
	os.Stderr = writer
	loaded, loadErr := LoadModule(filepath.Join(projectDir, "main.ard"))
	os.Stderr = originalStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, readErr := io.ReadAll(reader)
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
	if loadErr != nil {
		t.Fatalf("LoadModule returned an error for a warning: %v", loadErr)
	}
	if loaded == nil || loaded.Module == nil {
		t.Fatal("LoadModule returned no module")
	}
	if got := string(output); !strings.Contains(got, "warning: Redundant nullable Void") {
		t.Fatalf("diagnostic output = %q, want redundant nullable Void warning", got)
	}
}

// TestLoadModuleSharesOneGoTypeUniverse pins ADR 0044: all `use go:` imports
// across the whole Ard program — including imports that only appear in
// transitively imported Ard modules — load in one go/packages session.
//
// The fixture is the case cross-universe translation cannot resolve: the
// interface's package (ffi/api) and the implementer's package (ffi/impl)
// share a vocabulary package (ffi/shared) but do not import each other, so
// no single per-path load universe contains both declarations. Only a shared
// universe proves that *impl.Client satisfies api.Handler.
func TestLoadModuleSharesOneGoTypeUniverse(t *testing.T) {
	projectDir := t.TempDir()
	writeFile := func(path, contents string) {
		t.Helper()
		full := filepath.Join(projectDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile("ard.toml", "name = \"universe\"\nard = \">= 0.1.0\"\n")
	writeFile("go.mod", "module universe\n\ngo 1.27\n")
	writeFile("ffi/shared/shared.go", `package shared

type Record struct {
	Value int
}
`)
	writeFile("ffi/api/api.go", `package api

import "universe/ffi/shared"

type Handler interface {
	Handle(rec shared.Record) int
}

func Dispatch(h Handler) int {
	return h.Handle(shared.Record{Value: 21})
}
`)
	writeFile("ffi/impl/impl.go", `package impl

import "universe/ffi/shared"

type Client struct{}

func (c *Client) Handle(rec shared.Record) int {
	return rec.Value * 2
}

func New() *Client {
	return &Client{}
}
`)
	// The api import lives in a transitively imported Ard module, so the
	// pre-scan must walk the Ard import graph, not just the entry module.
	writeFile("clients.ard", `use go:universe/ffi/api

fn dispatch(h: api::Handler) Int {
  api::Dispatch(h)
}
`)
	writeFile("main.ard", `use go:universe/ffi/impl
use universe/clients

fn main() {
  let client = impl::New()
  if not clients::dispatch(client) == 42 {
    panic("dispatch mismatch")
  }
}
`)

	loaded, err := LoadModule(filepath.Join(projectDir, "main.ard"))
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	if loaded.Module == nil {
		t.Fatal("LoadModule returned no module")
	}
}
