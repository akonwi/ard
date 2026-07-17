package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestGoTargetMapsEmptyStructChannelsToVoid(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"emptysignal\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module emptysignal\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDir := filepath.Join(projectDir, "ffi", "fixture")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "fixture.go"), []byte(`package fixture

var emptyCalled bool

type EmptyAlias = struct{}

func Empty() struct{} { emptyCalled = true; return struct{}{} }
func AliasedEmpty() EmptyAlias { return EmptyAlias{} }
func Consume(value struct{}) {}
func ConsumeAlias(value EmptyAlias) {}
func EmptyCalled() bool { return emptyCalled }
func NewSignal() chan struct{} { return make(chan struct{}, 1) }
func SendOnly(ch chan struct{}) chan<- struct{} { return ch }
func ReceiveOnly(ch chan struct{}) <-chan struct{} { return ch }
func GenericSignal[T any](value T) <-chan struct{} {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return ch
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`use go:emptysignal/ffi/fixture

fn main() {
  fixture::Empty()
  fixture::Consume(())
  fixture::ConsumeAlias(fixture::AliasedEmpty())
  if not fixture::EmptyCalled() { panic("empty return was not evaluated") }

  let ch: Chan<Void> = fixture::NewSignal()
  let tx: Sender<Void> = fixture::SendOnly(ch)
  let rx: Receiver<Void> = fixture::ReceiveOnly(ch)
  tx.send(())
  rx.recv().expect("signal")
  tx.close()
  fixture::GenericSignal("generic").recv().expect("generic signal")
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
