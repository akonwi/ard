package checker_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
)

// importPathWorkspace lays out an app that depends on a package named
// "widgets" under the alias "ui". widgets in turn depends on "helper", which
// the app does not declare, so helper is only a transitive dependency.
func importPathWorkspace(t *testing.T) (app string, resolver *checker.ModuleResolver) {
	t.Helper()
	workspace := t.TempDir()
	app = filepath.Join(workspace, "app")
	widgets := filepath.Join(workspace, "widgets")
	helper := filepath.Join(workspace, "helper")
	files := map[string]string{
		filepath.Join(helper, "ard.toml"):           "name = \"helper\"\nard = \">= 0.1.0\"\n",
		filepath.Join(helper, "helper.ard"):         "fn value() Int { 42 }\n",
		filepath.Join(widgets, "ard.toml"):          "name = \"widgets\"\nard = \">= 0.1.0\"\n\n[dependencies]\nhelper = { path = \"../helper\" }\n",
		filepath.Join(widgets, "widgets.ard"):       "fn version() Int { 1 }\n",
		filepath.Join(widgets, "shapes.ard"):        "struct Box { w: Int }\n",
		filepath.Join(widgets, "shapes", "geo.ard"): "struct Point { x: Int }\n",
		filepath.Join(app, "ard.toml"):              "name = \"app\"\nard = \">= 0.1.0\"\n\n[dependencies]\nui = { path = \"../widgets\" }\n",
		filepath.Join(app, "api.ard"):               "fn ping() Int { 1 }\n",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lock := fmt.Sprintf(`{
  "version": 1,
  "root": "root",
  "packages": [
    {"id": "root", "name": "app", "path": ".", "dependencies": {"ui": "path:%s"}},
    {"id": "path:%s", "name": "widgets", "path": "%s", "dependencies": {"helper": "path:%s"}},
    {"id": "path:%s", "name": "helper", "path": "%s"}
  ]
}
`, widgets, widgets, widgets, helper, helper, helper)
	if err := os.WriteFile(filepath.Join(app, "ard.lock"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver, err := checker.NewModuleResolver(app)
	if err != nil {
		t.Fatalf("new resolver: %v", err)
	}
	return app, resolver
}

// ImportPath inverts import resolution from the root package: the path it
// returns resolves back to the same canonical module path.
func TestProjectInfoImportPathRoundTripsRootImports(t *testing.T) {
	_, resolver := importPathWorkspace(t)
	project := resolver.GetProjectInfo()
	cases := []struct {
		use  string
		want string
	}{
		{use: "app/api", want: "app/api"},
		{use: "ui", want: "ui"},
		{use: "ui/widgets", want: "ui"},
		{use: "ui/shapes", want: "ui/shapes"},
		{use: "ui/shapes/geo", want: "ui/shapes/geo"},
	}
	for _, tc := range cases {
		t.Run(tc.use, func(t *testing.T) {
			resolved, err := resolver.ResolveImport("", tc.use)
			if err != nil {
				t.Fatalf("resolve %q: %v", tc.use, err)
			}
			got, ok := project.ImportPath(resolved.ModulePath)
			if !ok || got != tc.want {
				t.Fatalf("ImportPath(%q) = %q, %v; want %q, true", resolved.ModulePath, got, ok, tc.want)
			}
			again, err := resolver.ResolveImport("", got)
			if err != nil {
				t.Fatalf("resolve returned import path %q: %v", got, err)
			}
			if again.ModulePath != resolved.ModulePath {
				t.Fatalf("%q resolves to %q, want %q", got, again.ModulePath, resolved.ModulePath)
			}
		})
	}
}

func TestProjectInfoImportPathRejectsModulesTheRootCannotImport(t *testing.T) {
	_, resolver := importPathWorkspace(t)
	project := resolver.GetProjectInfo()
	widgets, err := resolver.ResolveImport("", "ui")
	if err != nil {
		t.Fatal(err)
	}
	// widgets imports helper under its own alias; the root declares no alias
	// for helper, so no root `use` line can name it.
	helper, err := resolver.ResolveImport(widgets.ModulePath, "helper")
	if err != nil {
		t.Fatalf("resolve helper from widgets: %v", err)
	}
	if !strings.HasPrefix(helper.ModulePath, "_pkg_") {
		t.Fatalf("helper module path = %q, want a canonical dependency path", helper.ModulePath)
	}
	if got, ok := project.ImportPath(helper.ModulePath); ok {
		t.Fatalf("ImportPath(%q) = %q, true; want false for a transitive dependency", helper.ModulePath, got)
	}
}

func TestProjectInfoImportPathPassesThroughNonDependencyPaths(t *testing.T) {
	var none *checker.ProjectInfo
	for _, path := range []string{"ard/list", "app/api", "main"} {
		if got, ok := none.ImportPath(path); !ok || got != path {
			t.Fatalf("nil ProjectInfo ImportPath(%q) = %q, %v; want unchanged", path, got, ok)
		}
	}
	// A root package may itself be named like the dependency namespace; only
	// prefixes of known dependency packages are canonical dependency paths.
	project := &checker.ProjectInfo{
		ProjectName:   "_pkg_example",
		RootPackageID: "root",
		Packages: map[string]checker.PackageInfo{
			"root":       {ID: "root", Name: "_pkg_example"},
			"path:/deps": {ID: "path:/deps", Name: "deps"},
		},
	}
	for _, path := range []string{"_pkg_example/api", "_pkg_0000000000000000/thing"} {
		if got, ok := project.ImportPath(path); !ok || got != path {
			t.Fatalf("ImportPath(%q) = %q, %v; want unchanged", path, got, ok)
		}
	}
}

func TestProjectInfoImportPathChoosesFirstAliasAndFallsBackToAliasID(t *testing.T) {
	project := &checker.ProjectInfo{
		RootPackageID: "root",
		Dependencies: map[string]checker.DependencyInfo{
			"zui":  {Alias: "zui", Name: "widgets", PackageID: "path:/widgets"},
			"ui":   {Alias: "ui", Name: "widgets", PackageID: "path:/widgets"},
			"util": {Alias: "util"},
		},
	}
	widgets := checker.PackageModulePrefix("path:/widgets")
	if got, ok := project.ImportPath(widgets + "/shapes"); !ok || got != "ui/shapes" {
		t.Fatalf("ImportPath(shapes) = %q, %v; want ui/shapes", got, ok)
	}
	if got, ok := project.ImportPath(widgets + "/widgets"); !ok || got != "ui" {
		t.Fatalf("ImportPath(main module) = %q, %v; want ui", got, ok)
	}
	if got, ok := project.ImportPath(widgets + "/sub/widgets"); !ok || got != "ui/sub/widgets" {
		t.Fatalf("ImportPath(nested module named like the package) = %q, %v; want ui/sub/widgets", got, ok)
	}
	// A dependency without a package ID is identified by its alias, and a
	// bare `use util` imports its module named after the alias.
	util := checker.PackageModulePrefix("util")
	if got, ok := project.ImportPath(util + "/util"); !ok || got != "util" {
		t.Fatalf("ImportPath(alias-identified main module) = %q, %v; want util", got, ok)
	}
	if got, ok := project.ImportPath(util + "/text"); !ok || got != "util/text" {
		t.Fatalf("ImportPath(alias-identified module) = %q, %v; want util/text", got, ok)
	}
}
