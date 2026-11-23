package checker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/compiler/checker"
	"github.com/akonwi/ard/compiler/parse"
)

// crossModuleAliasLib declares a public alias for every kind of alias target,
// plus private aliases that must stay inside the module.
const crossModuleAliasLib = `use go:strings

struct Box<$T> {
  v: $T,
}

type Union = Str | Int
type Plain = Box<Int>
type Ptr = &mut Box<Int>
type Ref = &Box<Int>
type Opt = Box<Int>?
type Res = Int!Str
type Ints = [Int]
type Counts = [Str:Int]
type Pair = (Int, Str)
type Handler = fn(Int) Str
type Name = Str
type Builder = strings::Builder
type BuilderPtr = &mut strings::Builder
private type HiddenPlain = Box<Int>
private type HiddenPtr = &mut Box<Int>
private type HiddenHandler = fn() Int
`

func checkCrossModuleAliasMain(t *testing.T, mainSource string) []checker.Diagnostic {
	t.Helper()
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "lib.ard"), []byte(crossModuleAliasLib), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	result := parse.Parse([]byte(mainSource), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, result.Program, resolver)
	c.Check()
	return c.Diagnostics()
}

// Issue #620: a public type alias is exported whatever its target kind is,
// and can be named from another module in every type position.
func TestPublicTypeAliasesOfEveryTargetKindResolveAcrossModules(t *testing.T) {
	aliases := []string{"Union", "Plain", "Ptr", "Ref", "Opt", "Res", "Ints", "Counts", "Pair", "Handler", "Name", "Builder", "BuilderPtr"}
	for _, alias := range aliases {
		t.Run(alias, func(t *testing.T) {
			name := "lib::" + alias
			source := strings.ReplaceAll(`use demo/lib

struct Holder {
  field: ALIAS,
}

fn param(x: ALIAS) {}

fn ret(x: ALIAS) ALIAS {
  x
}

fn annotated(x: ALIAS) {
  let y: ALIAS = x
  let held = Holder{field: y}
  let items: [ALIAS] = [held.field]
  let maybe: ALIAS? = Maybe::new(items.at(0))
  let result: ALIAS!Str = Result::ok(x)
  let keyed: [Str:ALIAS] = ["k": x]
}
`, "ALIAS", name)
			if diagnostics := checkCrossModuleAliasMain(t, source); len(diagnostics) > 0 {
				t.Fatalf("unexpected diagnostics:\n%s", diagnosticsString(diagnostics))
			}
		})
	}
}

// Issue #620: an imported pointer alias is the pointer type it names, so it
// keeps both the pointer's mutability and its pointee.
func TestImportedPointerAliasResolvesToItsPointerType(t *testing.T) {
	valid := `use demo/lib

fn bump(box: lib::Ptr) {
  box.v = box.v + 1
}

fn read(box: lib::Ref) Int {
  box.v
}

fn main() {
  mut box = lib::Box{v: 1}
  bump(&mut box)
  let ptr: lib::Ptr = &mut box
  let reference: lib::Ref = &box
  let total = read(ptr) + read(reference)
}
`
	if diagnostics := checkCrossModuleAliasMain(t, valid); len(diagnostics) > 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diagnosticsString(diagnostics))
	}

	for _, tc := range []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "value where a mutable pointer is expected",
			source: "use demo/lib\n\nfn bump(box: lib::Ptr) {}\n\nfn main() {\n  bump(lib::Box{v: 1})\n}\n",
			want:   "Expected &mut Box<Int>, got Box<Int>",
		},
		{
			name:   "read-only pointer where a mutable pointer is expected",
			source: "use demo/lib\n\nfn bump(box: lib::Ptr) {}\n\nfn main() {\n  let box = lib::Box{v: 1}\n  bump(&box)\n}\n",
			want:   "Expected &mut Box<Int>, got &Box<Int>",
		},
		{
			name:   "write through a read-only pointer alias",
			source: "use demo/lib\n\nfn write(box: lib::Ref) {\n  box.v = 2\n}\n",
			want:   "Cannot write through read-only pointer &Box<Int>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diagnostics := checkCrossModuleAliasMain(t, tc.source)
			if len(diagnostics) == 0 {
				t.Fatalf("expected a diagnostic mentioning %q", tc.want)
			}
			if got := diagnosticsString(diagnostics); !strings.Contains(got, tc.want) {
				t.Fatalf("diagnostics = %s, want mention of %q", got, tc.want)
			}
		})
	}
}

// A `private type` alias belongs to its module whatever its target is, even
// when the target itself is a public type.
func TestPrivateTypeAliasesAreNotExported(t *testing.T) {
	for _, alias := range []string{"HiddenPlain", "HiddenPtr", "HiddenHandler"} {
		t.Run(alias, func(t *testing.T) {
			name := "lib::" + alias
			diagnostics := checkCrossModuleAliasMain(t, "use demo/lib\n\nfn use_it(x: "+name+") {}\n")
			found := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == checker.DiagnosticCodeUndefinedType && diagnostic.Message == "Unrecognized type: "+name {
					found = true
				}
			}
			if !found {
				t.Fatalf("diagnostics = %s, want %s to be unresolved", diagnosticsString(diagnostics), name)
			}
		})
	}
}
