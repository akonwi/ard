package gotarget

import (
	"testing"

	"github.com/akonwi/ard/air"
)

func TestNamePlanTracksTypesSynthesizedDuringLowering(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{{ID: 0, Path: "app/main.ard"}},
		Types:   []air.TypeInfo{{ID: 1, Kind: air.TypeInt, Name: "Int"}},
	}
	lowerer := &lowerer{program: program, useModulePackages: true, currentModule: 0}
	lowerer.functionModules = lowerer.collectFunctionEmitModules()
	lowerer.namePlan = newNamePlan(lowerer)

	maybeID := lowerer.findMaybeTypeByElem(1)
	maybeType := program.Types[maybeID-1]
	alias := lowerer.typeName(maybeType)
	if !lowerer.importAliasCollidesWithTopLevel(alias) {
		t.Fatalf("synthesized type name %q was not added to the module collision set", alias)
	}
}

func TestNamePlanMatchesExistingNamesAndImportCollisions(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "app/alpha.ard", Types: []air.TypeID{1, 2}, Globals: []air.GlobalID{0}},
			{ID: 1, Path: "app/beta.ard", Types: []air.TypeID{3}, Globals: []air.GlobalID{1}},
		},
		Types: []air.TypeInfo{
			{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "app/alpha.ard"},
			{ID: 2, Kind: air.TypeEnum, Name: "Direction", ModulePath: "app/alpha.ard", Variants: []air.VariantInfo{{Name: "Down", Discriminant: 0}, {Name: "Down", Discriminant: 1}}},
			{ID: 3, Kind: air.TypeStruct, Name: "User", ModulePath: "app/beta.ard"},
		},
		Traits: []air.Trait{{ID: 0, Name: "Renderable", ModulePath: "app/alpha.ard"}},
		Functions: []air.Function{
			{ID: 0, Module: 0, Name: "load_user"},
			{ID: 1, Module: 0, Name: "renderable"},
			{ID: 2, Module: 1, Name: "load_user"},
		},
		Globals: []air.Global{
			{ID: 0, Module: 0, Name: "default_user"},
			{ID: 1, Module: 1, Name: "default_user"},
		},
	}

	lowerer := &lowerer{program: program, useModulePackages: true}
	lowerer.functionModules = lowerer.collectFunctionEmitModules()

	moduleAliases := map[air.ModuleID]map[string]bool{}
	aliases := []string{"User", "Direction", "DirectionDown", "DirectionDown_1", "Renderable", "Renderable_1", "LoadUser", "LoadUser_1", "DefaultUser", "fmt", "unused"}
	for _, module := range program.Modules {
		moduleAliases[module.ID] = map[string]bool{}
		for _, alias := range aliases {
			moduleAliases[module.ID][alias] = lowerer.importAliasCollidesWithModuleTopLevel(alias, module.ID)
		}
	}
	programAliases := map[string]bool{}
	for _, alias := range aliases {
		programAliases[alias] = lowerer.importAliasCollidesWithProgramTopLevel(alias)
	}

	plan := newNamePlan(lowerer)
	for _, typ := range program.Types {
		if got, want := plan.typeName(typ), typeName(program, typ); got != want {
			t.Errorf("type %d name = %q, want %q", typ.ID, got, want)
		}
		for _, variant := range typ.Variants {
			if got, want := plan.enumVariantName(typ, variant), enumVariantName(program, typ, variant); got != want {
				t.Errorf("type %d variant %q name = %q, want %q", typ.ID, variant.Name, got, want)
			}
		}
	}
	for _, trait := range program.Traits {
		if got, want := plan.traitName(trait), lowerer.traitInterfaceTypeName(trait); got != want {
			t.Errorf("trait %d name = %q, want %q", trait.ID, got, want)
		}
	}
	for _, fn := range program.Functions {
		if got, want := plan.functionName(fn), functionName(program, fn); got != want {
			t.Errorf("function %d name = %q, want %q", fn.ID, got, want)
		}
	}
	for _, global := range program.Globals {
		if got, want := plan.globalName(global), globalName(program, global); got != want {
			t.Errorf("global %d name = %q, want %q", global.ID, got, want)
		}
	}

	lowerer.namePlan = plan
	for _, module := range program.Modules {
		lowerer.currentModule = module.ID
		for _, alias := range aliases {
			if got, want := lowerer.importAliasCollidesWithTopLevel(alias), moduleAliases[module.ID][alias]; got != want {
				t.Errorf("module %d alias %q collision = %v, want %v", module.ID, alias, got, want)
			}
		}
	}
	lowerer.useModulePackages = false
	for _, alias := range aliases {
		if got, want := lowerer.importAliasCollidesWithTopLevel(alias), programAliases[alias]; got != want {
			t.Errorf("program alias %q collision = %v, want %v", alias, got, want)
		}
	}
}
