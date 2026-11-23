package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

// Issue #620: public type aliases of any target kind are usable from another
// module, directly and re-exported, and behave as the type they name. Writes
// through an imported pointer alias reach the caller's value.
func TestRunProgramUsesImportedTypeAliasesOfEveryTargetKind(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"ard.toml": "name = \"app\"\nard = \">= 0.1.0\"\n",
		"lib.ard": `
use go:strings

struct Box<$T> {
  v: $T,
}

fn Box::make(v: Int) Box<Int> {
  Box{v: v}
}

type Inbox = &mut Box<Int>
type View = &Box<Int>
type Plain = Box<Int>
type Opt = Int?
type Res = Int!Str
type Ints = [Int]
type Counts = [Str:Int]
type Pair = (Int, Str)
type Handler = fn(Int) Int
type Builder = &mut strings::Builder

fn bump(inbox: Inbox) {
  inbox.v = inbox.v + 1
}
`,
		"barrel.ard": `
use app/lib

type Inbox = lib::Inbox
`,
		"main.ard": `
use go:strings
use app/barrel
use app/lib

struct Holder {
  inbox: lib::Inbox,
}

fn add(inbox: lib::Inbox, n: Int) lib::Inbox {
  inbox.v = inbox.v + n
  inbox
}

fn reexported(inbox: barrel::Inbox) {
  inbox.v = inbox.v * 10
}

fn read(view: lib::View) Int {
  view.v
}

fn write(builder: lib::Builder, text: Str) {
  builder.WriteString(text)
}

fn main() {
  mut box = lib::Box{v: 1}
  lib::bump(&mut box)
  if box.v != 2 { panic("lib::bump through alias = {box.v}") }

  let returned = add(&mut box, 3)
  returned.v = returned.v + 1
  if box.v != 6 { panic("returned alias = {box.v}") }

  let holder = Holder{inbox: &mut box}
  holder.inbox.v = 7
  if box.v != 7 { panic("struct field alias = {box.v}") }

  let annotated: lib::Inbox = &mut box
  let inboxes: [lib::Inbox] = [annotated]
  for inbox in inboxes {
    inbox.v = 8
  }
  if box.v != 8 { panic("generic arg alias = {box.v}") }

  reexported(&mut box)
  if box.v != 80 { panic("re-exported alias = {box.v}") }
  if read(&box) != 80 { panic("read-only alias") }

  let plain: lib::Plain = lib::Plain::make(5)
  if plain.v != 5 { panic("plain alias static owner") }

  let opt: lib::Opt = Maybe::new(3)
  if opt.or(0) != 3 { panic("maybe alias") }
  let res: lib::Res = Result::ok(4)
  if res.or(0) != 4 { panic("result alias") }
  let ints: lib::Ints = [1, 2]
  if ints.size() != 2 { panic("list alias") }
  let counts: lib::Counts = ["a": 1]
  if counts.size() != 1 { panic("map alias") }
  let pair: lib::Pair = (1, "one")
  let number, word = pair
  if number != 1 or word != "one" { panic("tuple alias") }
  let double: lib::Handler = fn(n: Int) Int { n * 2 }
  if double(4) != 8 { panic("function alias") }

  mut builder = strings::Builder{}
  let alias: lib::Builder = &mut builder
  write(alias, "hi")
  if alias.String() != "hi" { panic("go pointer alias = {alias.String()}") }
}
`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(root, "main.ard")
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
