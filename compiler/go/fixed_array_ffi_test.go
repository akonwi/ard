package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestGoTargetGoFixedArrayReturn(t *testing.T) {
	program := lowerParitySource(t, `use go:crypto/sha256

fn main() Bool {
  mut bytes = "hello".bytes()
  let digest: [Byte; 32] = sha256::Sum256(mut bytes)
  digest.size() == 32 and digest.at(0).expect("digest") == 44
}`)
	if got := runGoTargetParityJSON(t, program); got != "true" {
		t.Fatalf("got %s, want true", got)
	}
}

func TestGoTargetNamedGoFixedArrayKeepsIdentity(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"arrayffi\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module arrayffi\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDir := filepath.Join(projectDir, "ffi", "fixture")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "fixture.go"), []byte(`package fixture

type Digest [3]byte

type Widget struct { Name string }
type Widgets [3]*Widget

func MakeDigest() Digest { return Digest{7, 8, 9} }
func UseDigest(d Digest) byte { return d[1] }
func (d Digest) First() byte { return d[0] }
func MakeWidgets() Widgets { return Widgets{&Widget{Name: "a"}, nil, &Widget{Name: "c"}} }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use ard/unsafe
use go:arrayffi/ffi/fixture

fn main() Bool {
  let digest = fixture::MakeDigest()
  let plain: [Byte; 3] = digest
  let literal: fixture::Digest = [7, 8, 9]
  let widgets = fixture::MakeWidgets()
  digest.size() == 3 and digest.at(2).expect("bounds") == 9 and digest.First() == 7 and fixture::UseDigest(digest) == 8 and fixture::UseDigest(literal) == 8 and fixture::UseDigest(plain) == 8 and plain.at(0).expect("plain") == 7 and widgets.at(1).is_some() and unsafe::is_nil(widgets.at(1).expect("in bounds nil")) and widgets.at(3).is_none()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
