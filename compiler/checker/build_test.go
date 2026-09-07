package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/manifest"
	"github.com/akonwi/ard/parse"
)

func TestResolveBuildValues(t *testing.T) {
	config := manifest.BuildConfig{Values: map[string]manifest.BuildValue{
		"version": {Type: manifest.BuildValueStr, Default: "dev", Release: true},
		"number":  {Type: manifest.BuildValueInt, Default: int64(0)},
		"enabled": {Type: manifest.BuildValueBool, Default: false},
	}}

	values, err := resolveBuildValues(config, BuildOptions{Release: true, Overrides: []BuildOverride{
		{Name: "version", Value: "dev"},
		{Name: "number", Value: "42"},
		{Name: "enabled", Value: "true"},
	}})
	if err != nil {
		t.Fatalf("resolveBuildValues: %v", err)
	}
	if values["version"].Value != "dev" || values["number"].Value != 42 || values["enabled"].Value != true {
		t.Fatalf("values = %#v", values)
	}
}

func TestResolveBuildValuesRejectsInvalidOverrides(t *testing.T) {
	config := manifest.BuildConfig{Values: map[string]manifest.BuildValue{
		"version": {Type: manifest.BuildValueStr, Default: "dev", Release: true},
		"number":  {Type: manifest.BuildValueInt, Default: int64(0)},
		"enabled": {Type: manifest.BuildValueBool, Default: false},
	}}
	tests := []struct {
		name    string
		options BuildOptions
		wantErr string
	}{
		{name: "missing release value", options: BuildOptions{Release: true}, wantErr: `requires explicit --define values for: "version"`},
		{name: "unknown", options: BuildOptions{Overrides: []BuildOverride{{Name: "missing", Value: "x"}}}, wantErr: `unknown build value "missing"`},
		{name: "duplicate", options: BuildOptions{Overrides: []BuildOverride{{Name: "version", Value: "a"}, {Name: "version", Value: "b"}}}, wantErr: `defined more than once`},
		{name: "bad int", options: BuildOptions{Overrides: []BuildOverride{{Name: "number", Value: "0x2a"}}}, wantErr: `decimal Int`},
		{name: "bad bool", options: BuildOptions{Overrides: []BuildOverride{{Name: "enabled", Value: "TRUE"}}}, wantErr: `must be Bool`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveBuildValues(config, tt.options)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestBuildModuleImportExposesTypedImmutableValues(t *testing.T) {
	root := t.TempDir()
	manifestSource := `name = "app"
ard = ">= 0.40.0"

[build.values]
version = { type = "Str", default = "dev" }
number = { type = "Int", default = 7 }
enabled = { type = "Bool", default = true }
`
	writeBuildInfoTestFile(t, root, "ard.toml", manifestSource)
	source := "use ard/build\nlet version = build::version\nlet number = build::number\nlet enabled = build::enabled\n"
	writeBuildInfoTestFile(t, root, "main.ard", source)

	module, diagnostics := checkBuildInfoTestModule(t, root, "main.ard", source, BuildOptions{})
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	imported := module.Program().Imports[BuildModulePath]
	if imported == nil {
		t.Fatalf("missing build import: %#v", module.Program().Imports)
	}
	for name, want := range map[string]Type{"version": Str, "number": Int, "enabled": Bool} {
		if got := imported.Get(name).Type; got != want {
			t.Errorf("%s type = %v, want %v", name, got, want)
		}
	}
	names := make([]string, 0, len(imported.Program().Statements))
	for _, statement := range imported.Program().Statements {
		definition, ok := statement.Stmt.(*VariableDef)
		if !ok || definition.Mutable {
			t.Fatalf("build statement = %#v, want immutable variable", statement)
		}
		names = append(names, definition.Name)
	}
	if got, want := fmt.Sprint(names), "[enabled number version]"; got != want {
		t.Fatalf("build statement order = %s, want %s", got, want)
	}
}

func TestBuildModuleImportRequiresDeclarations(t *testing.T) {
	root := t.TempDir()
	writeBuildInfoTestFile(t, root, "ard.toml", "name = \"app\"\nard = \">= 0.40.0\"\n")
	source := "use ard/build\n"
	_, diagnostics := checkBuildInfoTestModule(t, root, "main.ard", source, BuildOptions{})
	if got := fmt.Sprint(diagnostics); !strings.Contains(got, "requires declarations in [build.values]") {
		t.Fatalf("diagnostics = %s", got)
	}
}

func TestBuildModuleImportIsRejectedInDependency(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	dependency := filepath.Join(workspace, "dep")
	writeBuildInfoTestFile(t, root, "ard.toml", "name = \"app\"\nard = \">= 0.40.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n\n[build.values]\nversion = { type = \"Str\", default = \"app\" }\n")
	writeBuildInfoTestFile(t, dependency, "ard.toml", "name = \"dep\"\nard = \">= 0.40.0\"\n\n[build.values]\nversion = { type = \"Str\", default = \"dep\" }\n")
	writeBuildInfoTestFile(t, dependency, "dep.ard", "use ard/build\nfn version() Str { build::version }\n")
	source := "use dep\nlet version = dep::version()\n"
	writeBuildInfoTestFile(t, root, "main.ard", source)

	_, diagnostics := checkBuildInfoTestModule(t, root, "main.ard", source, BuildOptions{})
	if got := fmt.Sprint(diagnostics); !strings.Contains(got, "available only to modules in the root application package") {
		t.Fatalf("diagnostics = %s", got)
	}
}

func TestBuildModuleDirectDependencyCheckFailsClosed(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "app")
	dependency := filepath.Join(workspace, "dep")
	writeBuildInfoTestFile(t, root, "ard.toml", "name = \"app\"\nard = \">= 0.40.0\"\n\n[dependencies]\ndep = { path = \"../dep\" }\n\n[build.values]\nversion = { type = \"Str\", default = \"app\" }\n")
	writeBuildInfoTestFile(t, dependency, "ard.toml", "name = \"dep\"\nard = \">= 0.40.0\"\n")
	source := "use ard/build\n"
	dependencyFile := filepath.Join(dependency, "dep.ard")
	writeBuildInfoTestFile(t, dependency, "dep.ard", source)
	parsed := parse.Parse([]byte(source), dependencyFile)
	resolver, err := NewModuleResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := New(dependencyFile, parsed.Program, resolver)
	checked.Check()
	if got := fmt.Sprint(checked.Diagnostics()); !strings.Contains(got, "available only to modules in the root application package") {
		t.Fatalf("diagnostics = %s", got)
	}
}

func checkBuildInfoTestModule(t *testing.T, root, rel, source string, options BuildOptions) (Module, []Diagnostic) {
	t.Helper()
	parsed := parse.Parse([]byte(source), filepath.Join(root, rel))
	if len(parsed.Errors) != 0 {
		t.Fatalf("parse errors = %#v", parsed.Errors)
	}
	resolver, err := NewModuleResolverWithOptions(root, options)
	if err != nil {
		t.Fatalf("NewModuleResolverWithOptions: %v", err)
	}
	checker := New(rel, parsed.Program, resolver)
	checker.Check()
	return checker.Module(), checker.Diagnostics()
}

func writeBuildInfoTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
