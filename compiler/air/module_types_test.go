package air

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// Module.Types is part of the AIR contract external consumers read: it lists
// the nominal declarations (structs, enums, unions, generic struct
// definitions) each module owns, whether the module is a lowering root or an
// import. Generic instantiations and type aliases are not declarations with
// their own AIR identity and are not listed.

func writeModuleTypesProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"ard.toml": "name = \"spike\"\nard = \">= 0.1.0\"",
		"shared.ard": `
			struct Meta { id: Int }
		`,
		"api.ard": `
			use spike/shared

			enum Color { red, green, blue }

			enum Unused { a, b }

			struct Point { x: Int, y: Int }

			type Shape = Point | Str

			struct ApiError { code: Int, message: Str }

			struct Envelope { meta: shared::Meta, color: Color }

			fn find(id: Int) Point? {
				Maybe::new(Point{x: id, y: id})
			}
		`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const moduleTypesMain = `
	use spike/api

	enum Mode { on, off }

	struct Config { mode: Mode }

	struct Box { value: $T }

	type Value = Int | Str

	type Settings = Config

	fn boxed() Box<Int> { Box{value: 1} }

	fn main() {
		let _ = api::find(1)
	}
`

func checkModuleTypesProject(t *testing.T) checker.Module {
	t.Helper()
	root := writeModuleTypesProject(t)
	result := parse.Parse([]byte(moduleTypesMain), filepath.Join(root, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	resolver, err := checker.NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New("main.ard", result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	return c.Module()
}

func lowerModuleTypesRoot(t *testing.T, root checker.Module) *Program {
	t.Helper()
	program, err := LowerModulesWithOptions([]checker.Module{root}, LowerOptions{})
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}

func importedModule(t *testing.T, module checker.Module, path string) checker.Module {
	t.Helper()
	for _, imported := range module.Program().Imports {
		if imported.Path() == path {
			return imported
		}
	}
	t.Fatalf("module %s does not import %s", module.Path(), path)
	return nil
}

func moduleTypeNames(t *testing.T, program *Program, path string) []string {
	t.Helper()
	for _, module := range program.Modules {
		if module.Path != path {
			continue
		}
		names := make([]string, 0, len(module.Types))
		for _, id := range module.Types {
			names = append(names, program.Types[id-1].Name)
		}
		sort.Strings(names)
		return names
	}
	t.Fatalf("module %q not lowered", path)
	return nil
}

func assertModuleTypeNames(t *testing.T, program *Program, path string, want ...string) {
	t.Helper()
	got := moduleTypeNames(t, program, path)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Fatalf("module %s types = %v, want %v", path, got, want)
	}
}

// assertModuleTypesOwnDeclarations checks the invariant behind Module.Types:
// every non-instantiated struct, enum, and union in Program.Types is listed
// exactly once, by the module that declares it.
func assertModuleTypesOwnDeclarations(t *testing.T, program *Program) {
	t.Helper()
	listedBy := map[TypeID]string{}
	for _, module := range program.Modules {
		for _, id := range module.Types {
			info := program.Types[id-1]
			if previous, ok := listedBy[id]; ok {
				t.Fatalf("type %s listed by both %s and %s", info.Name, previous, module.Path)
			}
			listedBy[id] = module.Path
			switch info.Kind {
			case TypeStruct, TypeEnum, TypeUnion:
			default:
				t.Fatalf("module %s lists non-declaration %s (kind %v)", module.Path, info.Name, info.Kind)
			}
			if info.Generic != NoType {
				t.Fatalf("module %s lists generic instantiation %s", module.Path, info.Name)
			}
			if info.ModulePath != module.Path {
				t.Fatalf("module %s lists %s declared in %s", module.Path, info.Name, info.ModulePath)
			}
		}
	}
	for _, info := range program.Types {
		switch info.Kind {
		case TypeStruct, TypeEnum, TypeUnion:
		default:
			continue
		}
		if info.Generic != NoType || info.ModulePath == "" {
			continue
		}
		if _, ok := listedBy[info.ID]; !ok {
			t.Fatalf("%s::%s is in Program.Types but not in its module's Types", info.ModulePath, info.Name)
		}
	}
}

func TestModuleTypesListsRootModuleDeclarationsWithImports(t *testing.T) {
	program := lowerModuleTypesRoot(t, checkModuleTypesProject(t))
	// Box<Int> is an instantiation and Settings an alias; neither is listed.
	assertModuleTypeNames(t, program, "main.ard", "Box", "Config", "Mode", "Value")
	assertModuleTypesOwnDeclarations(t, program)
}

func TestModuleTypesListsImportedModuleDeclarationsIncludingEnums(t *testing.T) {
	program := lowerModuleTypesRoot(t, checkModuleTypesProject(t))
	assertModuleTypeNames(t, program, "spike/api", "ApiError", "Color", "Envelope", "Point", "Shape", "Unused")
	assertModuleTypesOwnDeclarations(t, program)
}

func TestModuleTypesListsImportedModuleLoweredAsRoot(t *testing.T) {
	api := importedModule(t, checkModuleTypesProject(t), "spike/api")
	program := lowerModuleTypesRoot(t, api)
	assertModuleTypeNames(t, program, "spike/api", "ApiError", "Color", "Envelope", "Point", "Shape", "Unused")
	assertModuleTypesOwnDeclarations(t, program)
}

func TestModuleTypesListsTypesReachedOnlyThroughAnotherModule(t *testing.T) {
	// Nothing references spike/shared directly; Meta is interned only through
	// api's Envelope field, yet it is still a declaration of spike/shared.
	program := lowerModuleTypesRoot(t, checkModuleTypesProject(t))
	assertModuleTypeNames(t, program, "spike/shared", "Meta")
	assertModuleTypesOwnDeclarations(t, program)
}

const moduleTypesMethodsSource = `
	struct Box { value: $T }

	impl Box {
		fn get() $T { self.value }
	}

	struct Counter { count: Int }

	impl Counter {
		fn mut bump() { self.count = self.count + 1 }
	}

	fn bump_twice(counter: &mut Counter) {
		counter.bump()
		counter.bump()
	}

	fn main() {
		let box = Box{value: 1}
		let _ = box.get()
	}
`

func TestModuleTypesExcludesMethodReceiverReferencesAndInstantiations(t *testing.T) {
	// Calling methods interns the receiver's mutable reference (&mut Counter)
	// and the instantiation (Box<Int>); neither is a declaration.
	program := lowerProjectSource(t, writeModuleTypesProject(t), moduleTypesMethodsSource)
	assertModuleTypeNames(t, program, "main.ard", "Box", "Counter")
	assertModuleTypesOwnDeclarations(t, program)
}

func TestModuleTypesListsGenericDefinitionsOfSingleFileRoot(t *testing.T) {
	// Without a module path, declarations carry an empty ModulePath; the root
	// still lists everything it declares, generic definitions included.
	program := lowerSource(t, moduleTypesMethodsSource)
	assertModuleTypeNames(t, program, "test.ard", "Box", "Counter")
}
