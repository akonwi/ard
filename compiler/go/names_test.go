package gotarget

import (
	"testing"

	"github.com/akonwi/ard/air"
)

func TestGoPackageNameFromModulePathSanitizesInvalidNames(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "user.ard", want: "user"},
		{path: "accounts/foo_bar.ard", want: "foo_bar"},
		{path: "foo-bar.ard", want: "foo_bar"},
		{path: "123-api.ard", want: "_123_api"},
		{path: "type.ard", want: "type_"},
		{path: "---.ard", want: "module"},
	}
	for _, tt := range tests {
		if got := goPackageNameFromModulePath(tt.path); got != tt.want {
			t.Fatalf("goPackageNameFromModulePath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestModulePackageHelpersSanitizePaths(t *testing.T) {
	program := &air.Program{Modules: []air.Module{
		{ID: 0, Path: "accounts/foo_bar.ard"},
		{ID: 1, Path: "123-api/type.ard"},
		{ID: 2, Path: "v1.0/foo.ard"},
	}}
	if got := modulePackageName(program, 0); got != "foo_bar" {
		t.Fatalf("modulePackageName = %q, want foo_bar", got)
	}
	if got := modulePackageDir(program, 1); got != "_123_api/type_" {
		t.Fatalf("modulePackageDir = %q, want _123_api/type_", got)
	}
	if got := moduleImportPath(program, 1); got != "generated/_123_api/type_" {
		t.Fatalf("moduleImportPath = %q, want generated/_123_api/type_", got)
	}
	if got := modulePackageDir(program, 2); got != "v1_0/foo" {
		t.Fatalf("modulePackageDir dotted directory = %q, want v1_0/foo", got)
	}
}

func TestNaturalTypeNameUsesVisibilityForArdTypes(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "user.ard"},
		{ID: 2, Kind: air.TypeStruct, Name: "internal_config", ModulePath: "config.ard", Private: true},
		{ID: 3, Kind: air.TypeStruct, Name: "Std", ModulePath: "ard/std"},
	}}
	if got := typeName(program, program.Types[0]); got != "User" {
		t.Fatalf("public user type = %q, want User", got)
	}
	if got := typeName(program, program.Types[1]); got != "internalConfig" {
		t.Fatalf("private user type = %q, want internalConfig", got)
	}
	if got := typeName(program, program.Types[2]); got != "Std" {
		t.Fatalf("stdlib type = %q, want Std", got)
	}
}

func TestNaturalTypeNameFallsBackOnCollisions(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "a.ard"},
		{ID: 2, Kind: air.TypeStruct, Name: "User", ModulePath: "b.ard"},
	}}
	if got := typeName(program, program.Types[0]); got != "A_ard__User" {
		t.Fatalf("first colliding type = %q, want A_ard__User", got)
	}
	if got := typeName(program, program.Types[1]); got != "B_ard__User" {
		t.Fatalf("second colliding type = %q, want B_ard__User", got)
	}
}

func TestNaturalEnumVariantNameUsesEnumVisibility(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeEnum, Name: "Direction", ModulePath: "direction.ard", Variants: []air.VariantInfo{{Name: "Down", Discriminant: 0}}},
		{ID: 2, Kind: air.TypeEnum, Name: "internal_state", ModulePath: "state.ard", Private: true, Variants: []air.VariantInfo{{Name: "Ready", Discriminant: 0}}},
	}}
	if got := enumVariantName(program, program.Types[0], program.Types[0].Variants[0]); got != "DirectionDown" {
		t.Fatalf("public enum variant = %q, want DirectionDown", got)
	}
	if got := enumVariantName(program, program.Types[1], program.Types[1].Variants[0]); got != "internalStateReady" {
		t.Fatalf("private enum variant = %q, want internalStateReady", got)
	}
}

func TestNaturalEnumVariantNameAliasesCollisions(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeStruct, Name: "DirectionDown", ModulePath: "other.ard"},
		{ID: 2, Kind: air.TypeEnum, Name: "Direction", ModulePath: "direction.ard", Variants: []air.VariantInfo{{Name: "Down", Discriminant: 0}}},
		{ID: 3, Kind: air.TypeEnum, Name: "Direction", ModulePath: "direction2.ard", Variants: []air.VariantInfo{{Name: "Down", Discriminant: 0}}},
	}}
	if got := enumVariantName(program, program.Types[1], program.Types[1].Variants[0]); got != "Direction_ard__Direction__Down" {
		t.Fatalf("variant on enum with colliding type name = %q, want legacy name", got)
	}
	// Give the second enum a non-colliding type name but colliding variant name.
	program.Types[2].Name = "DirectionDown"
	program.Types[2].Variants[0].Name = ""
	// Empty variant names keep the legacy spelling.
	if got := enumVariantName(program, program.Types[2], program.Types[2].Variants[0]); got != "Direction2_ard__DirectionDown__variant_0" {
		t.Fatalf("empty variant = %q, want legacy fallback", got)
	}
	program.Types[2].Variants[0].Name = "__"
	if got := enumVariantName(program, program.Types[2], program.Types[2].Variants[0]); got != "Direction2_ard__DirectionDown__variant_0" {
		t.Fatalf("underscore-only variant = %q, want legacy fallback", got)
	}
}

func TestNaturalEnumVariantNameAliasesDuplicateVariantNames(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{{ID: 1, Kind: air.TypeEnum, Name: "Direction", ModulePath: "direction.ard", Variants: []air.VariantInfo{{Name: "Down", Discriminant: 0}, {Name: "Down", Discriminant: 1}}}}}
	if got := enumVariantName(program, program.Types[0], program.Types[0].Variants[0]); got != "DirectionDown" {
		t.Fatalf("first duplicate variant = %q, want DirectionDown", got)
	}
	if got := enumVariantName(program, program.Types[0], program.Types[0].Variants[1]); got != "DirectionDown_1" {
		t.Fatalf("second duplicate variant = %q, want DirectionDown_1", got)
	}
}

func TestNaturalEnumVariantNameAliasesValueCollisions(t *testing.T) {
	program := &air.Program{
		Types:     []air.TypeInfo{{ID: 1, Kind: air.TypeEnum, Name: "Direction", ModulePath: "direction.ard", Variants: []air.VariantInfo{{Name: "Down", Discriminant: 0}}}},
		Functions: []air.Function{{ID: 0, Module: 0, Name: "DirectionDown"}},
	}
	if got := enumVariantName(program, program.Types[0], program.Types[0].Variants[0]); got != "DirectionDown_1" {
		t.Fatalf("variant colliding with function = %q, want DirectionDown_1", got)
	}
}

func TestNaturalFunctionAndGlobalNamesUseVisibility(t *testing.T) {
	program := &air.Program{
		Functions: []air.Function{
			{ID: 0, Module: 0, Name: "make_user"},
			{ID: 1, Module: 0, Name: "format_name", Private: true},
		},
		Globals: []air.Global{
			{ID: 0, Module: 0, Name: "default_name"},
			{ID: 1, Module: 0, Name: "cache_key", Private: true},
		},
	}
	if got := functionName(program, program.Functions[0]); got != "MakeUser" {
		t.Fatalf("public function = %q, want MakeUser", got)
	}
	if got := functionName(program, program.Functions[1]); got != "formatName" {
		t.Fatalf("private function = %q, want formatName", got)
	}
	if got := globalName(program, program.Globals[0]); got != "DefaultName" {
		t.Fatalf("public global = %q, want DefaultName", got)
	}
	if got := globalName(program, program.Globals[1]); got != "cacheKey" {
		t.Fatalf("private global = %q, want cacheKey", got)
	}
}

func TestNaturalFunctionNameFallsBackForSyntheticFunctions(t *testing.T) {
	program := &air.Program{Functions: []air.Function{
		{ID: 0, Module: 0, Name: "main", IsScript: true},
		{ID: 1, Module: 0, Name: "User.ToString.to_str", Receiver: 1},
		{ID: 2, Module: 0, Name: "anon_func_2"},
	}}
	if got := functionName(program, program.Functions[0]); got != "ArdScript_0" {
		t.Fatalf("script function = %q, want ArdScript_0", got)
	}
	if got := functionName(program, program.Functions[1]); got != "Module_0__User_ToString_to_str" {
		t.Fatalf("method helper function = %q, want Module_0__User_ToString_to_str", got)
	}
	if got := functionName(program, program.Functions[2]); got != "module_0__anon_func_2" {
		t.Fatalf("closure helper function = %q, want module_0__anon_func_2", got)
	}
}

func TestNaturalTopLevelNamesAliasSpecialGoNames(t *testing.T) {
	program := &air.Program{
		Types: []air.TypeInfo{{ID: 1, Kind: air.TypeStruct, Name: "len", ModulePath: "types.ard", Private: true}},
		Functions: []air.Function{
			{ID: 0, Module: 0, Name: "main", Private: true},
			{ID: 1, Module: 0, Name: "len", Private: true},
		},
		Globals: []air.Global{{ID: 0, Module: 0, Name: "main", Private: true}},
	}
	if got := typeName(program, program.Types[0]); got != "types_ard__len" {
		t.Fatalf("type with special Go name alias = %q, want types_ard__len", got)
	}
	if got := functionName(program, program.Functions[0]); got != "main_1" {
		t.Fatalf("private main function alias = %q, want main_1", got)
	}
	if got := functionName(program, program.Functions[1]); got != "len_1" {
		t.Fatalf("private len function alias = %q, want len_1", got)
	}
	if got := globalName(program, program.Globals[0]); got != "main_2" {
		t.Fatalf("private main global alias = %q, want main_2", got)
	}
}

func TestNaturalTopLevelNamesFallBackOnCollisions(t *testing.T) {
	program := &air.Program{
		Types:     []air.TypeInfo{{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "types.ard"}},
		Traits:    []air.Trait{{ID: 0, Name: "Renderable", ModulePath: "traits.ard"}},
		Functions: []air.Function{{ID: 0, Module: 0, Name: "user"}, {ID: 1, Module: 0, Name: "renderable"}},
		Globals:   []air.Global{{ID: 0, Module: 0, Name: "user"}},
	}
	if got := typeName(program, program.Types[0]); got != "User" {
		t.Fatalf("type name should take precedence over function/global collisions = %q, want User", got)
	}
	if got := functionName(program, program.Functions[0]); got != "User_1" {
		t.Fatalf("function colliding with type/global = %q, want User_1", got)
	}
	if got := functionName(program, program.Functions[1]); got != "Renderable_1" {
		t.Fatalf("function colliding with trait = %q, want Renderable_1", got)
	}
	if got := globalName(program, program.Globals[0]); got != "User_2" {
		t.Fatalf("global colliding with type/function = %q, want User_2", got)
	}
}

func TestNaturalTypeNameFallsBackOnCrossKindCollisions(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "a.ard"},
		{ID: 2, Kind: air.TypeEnum, Name: "User", ModulePath: "b.ard"},
	}}
	if got := typeName(program, program.Types[0]); got != "A_ard__User" {
		t.Fatalf("struct colliding with enum = %q, want A_ard__User", got)
	}
	if got := typeName(program, program.Types[1]); got != "B_ard__User" {
		t.Fatalf("enum colliding with struct = %q, want B_ard__User", got)
	}
}

func TestUnionNamesUseExportedNaturalNamesWithAliases(t *testing.T) {
	typ := air.TypeInfo{Kind: air.TypeUnion, Name: "value", Members: []air.UnionMember{
		{Type: 1, Tag: 0, Name: "FooBar"},
		{Type: 2, Tag: 1, Name: "foo_bar"},
		{Type: 3, Tag: 2, Name: "ArdTag"},
	}}
	program := &air.Program{Types: []air.TypeInfo{typ}}
	if got := typeName(program, typ); got != "Value" {
		t.Fatalf("union type name = %q, want Value", got)
	}
	if got := unionMemberFieldName(typ, typ.Members[0]); got != "FooBar" {
		t.Fatalf("first union member field = %q, want FooBar", got)
	}
	if got := unionMemberFieldName(typ, typ.Members[1]); got != "FooBar1" {
		t.Fatalf("aliased union member field = %q, want FooBar1", got)
	}
	if got := unionTagFieldName(typ); got != "ArdTag1" {
		t.Fatalf("union tag field = %q, want ArdTag1", got)
	}
}

func TestGoFieldNamesAreAlwaysExported(t *testing.T) {
	l := &lowerer{}
	publicStruct := air.TypeInfo{Kind: air.TypeStruct, Name: "Error", ModulePath: "ard/decode"}
	if got := l.goFieldName(publicStruct, "expected_value"); got != "ExpectedValue" {
		t.Fatalf("public field = %q, want ExpectedValue", got)
	}
	// Fields are always exported so every struct is serializable, even private ones.
	privateStruct := air.TypeInfo{Kind: air.TypeStruct, Name: "internal", ModulePath: "ard/decode", Private: true}
	if got := l.goFieldName(privateStruct, "secret_key"); got != "SecretKey" {
		t.Fatalf("private struct field = %q, want SecretKey (always exported)", got)
	}
}

func TestNaturalGoIdentifierUsesVisibility(t *testing.T) {
	tests := []struct {
		raw      string
		exported bool
		want     string
	}{
		{raw: "make_user", exported: true, want: "MakeUser"},
		{raw: "format_name", exported: false, want: "formatName"},
		{raw: "User", exported: true, want: "User"},
		{raw: "InternalConfig", exported: false, want: "internalConfig"},
		{raw: "type", exported: false, want: "type_"},
	}
	for _, tt := range tests {
		if got := naturalGoIdentifier(tt.raw, tt.exported); got != tt.want {
			t.Fatalf("naturalGoIdentifier(%q, %v) = %q, want %q", tt.raw, tt.exported, got, tt.want)
		}
	}
}
