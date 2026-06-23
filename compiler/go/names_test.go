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

func TestNaturalTypeNameUsesVisibilityForUserTypes(t *testing.T) {
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
	if got := typeName(program, program.Types[2]); got != "ard_std__Std" {
		t.Fatalf("stdlib type = %q, want legacy artifact name", got)
	}
}

func TestNaturalTypeNameFallsBackOnCollisions(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "a.ard"},
		{ID: 2, Kind: air.TypeStruct, Name: "User", ModulePath: "b.ard"},
	}}
	if got := typeName(program, program.Types[0]); got != "a_ard__User" {
		t.Fatalf("first colliding type = %q, want a_ard__User", got)
	}
	if got := typeName(program, program.Types[1]); got != "b_ard__User" {
		t.Fatalf("second colliding type = %q, want b_ard__User", got)
	}
}

func TestNaturalTypeNameFallsBackOnCrossKindCollisions(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "a.ard"},
		{ID: 2, Kind: air.TypeEnum, Name: "User", ModulePath: "b.ard"},
	}}
	if got := typeName(program, program.Types[0]); got != "a_ard__User" {
		t.Fatalf("struct colliding with enum = %q, want a_ard__User", got)
	}
	if got := typeName(program, program.Types[1]); got != "b_ard__User" {
		t.Fatalf("enum colliding with struct = %q, want b_ard__User", got)
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
