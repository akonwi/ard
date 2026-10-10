package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// Entry modules (such as `ard test` files) share the resolver's import cache,
// so a project file that is both an entry and another entry's import is
// checked once per resolver.
func TestCheckEntrySharesModuleCacheWithImports(t *testing.T) {
	const warningModule = "let signal: Void? = Maybe::new()\n\nfn answer() Int { 42 }\n"
	const importer = "use app/one\n\nfn value() Int { one::answer() }\n"

	setup := func(t *testing.T, oneSource string) string {
		t.Helper()
		root := t.TempDir()
		files := map[string]string{
			"ard.toml":   "name = \"app\"\nard = \">= 0.1.0\"\n",
			"one.ard":    oneSource,
			"test/a.ard": importer,
		}
		for name, contents := range files {
			path := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}
	newResolver := func(t *testing.T, root string) *checker.ModuleResolver {
		t.Helper()
		resolver, err := checker.NewModuleResolver(root)
		if err != nil {
			t.Fatalf("new resolver: %v", err)
		}
		return resolver
	}
	checkEntry := func(t *testing.T, resolver *checker.ModuleResolver, root, rel, modulePath string) checker.EntryResult {
		t.Helper()
		sourcePath := filepath.Join(root, rel)
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		parsed := parse.Parse(source, sourcePath)
		if len(parsed.Errors) > 0 {
			t.Fatalf("parse errors: %v", parsed.Errors)
		}
		return checker.CheckEntry(sourcePath, rel, parsed.Program, resolver, checker.CheckOptions{ModulePath: modulePath})
	}
	countFrom := func(diagnostics []checker.Diagnostic, base string) int {
		count := 0
		for _, diagnostic := range diagnostics {
			if filepath.Base(diagnostic.Primary.Span.FilePath) == base {
				count++
			}
		}
		return count
	}

	t.Run("entry reuses a module checked by an earlier import", func(t *testing.T) {
		root := setup(t, warningModule)
		resolver := newResolver(t, root)
		a := checkEntry(t, resolver, root, "test/a.ard", "app/test/a")
		if countFrom(a.Diagnostics, "one.ard") != 1 {
			t.Fatalf("importer diagnostics = %#v, want one.ard warning", a.Diagnostics)
		}
		one := checkEntry(t, resolver, root, "one.ard", "app/one")
		if !one.Reused || len(one.Diagnostics) != 0 {
			t.Fatalf("entry reused = %v, diagnostics = %#v; want cached module without diagnostics", one.Reused, one.Diagnostics)
		}
		if one.Module != a.Module.Program().Imports["app/one"] {
			t.Fatal("entry should be the module the importer already checked")
		}
	})

	t.Run("import reuses a module checked as an earlier entry", func(t *testing.T) {
		root := setup(t, warningModule)
		resolver := newResolver(t, root)
		one := checkEntry(t, resolver, root, "one.ard", "app/one")
		if one.Reused || countFrom(one.Diagnostics, "one.ard") != 1 {
			t.Fatalf("entry reused = %v, diagnostics = %#v; want fresh check with one warning", one.Reused, one.Diagnostics)
		}
		a := checkEntry(t, resolver, root, "test/a.ard", "app/test/a")
		if countFrom(a.Diagnostics, "one.ard") != 0 {
			t.Fatalf("importer re-reported entry diagnostics: %#v", a.Diagnostics)
		}
		if a.Module.Program().Imports["app/one"] != one.Module {
			t.Fatal("import should reuse the entry module")
		}
	})

	// A package's root module file (app.ard in package app) is imported as
	// `use app` under the canonical module path "app", while path-derived
	// entry names spell it "app/app". Both name the same file.
	t.Run("root module entry adopts the canonical import identity", func(t *testing.T) {
		root := setup(t, warningModule)
		if err := os.WriteFile(filepath.Join(root, "app.ard"), []byte(warningModule), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "test", "b.ard"), []byte("use app\n\nfn value() Int { app::answer() }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		resolver := newResolver(t, root)
		entry := checkEntry(t, resolver, root, "app.ard", "app/app")
		if entry.Module.Path() != "app" {
			t.Fatalf("entry module path = %q, want canonical %q", entry.Module.Path(), "app")
		}
		b := checkEntry(t, resolver, root, "test/b.ard", "app/test/b")
		if countFrom(b.Diagnostics, "app.ard") != 0 {
			t.Fatalf("importer re-reported root module diagnostics: %#v", b.Diagnostics)
		}
		if b.Module.Program().Imports["app"] != entry.Module {
			t.Fatal("import should reuse the root module entry")
		}
	})

	t.Run("entry with a different module identity is not shared", func(t *testing.T) {
		root := setup(t, warningModule)
		resolver := newResolver(t, root)
		one := checkEntry(t, resolver, root, "one.ard", "elsewhere/one")
		if one.Reused {
			t.Fatal("entry with a non-import module path should be checked fresh")
		}
		a := checkEntry(t, resolver, root, "test/a.ard", "app/test/a")
		if a.Module.Program().Imports["app/one"] == one.Module {
			t.Fatal("import must not reuse an entry checked under a different module path")
		}
	})

	t.Run("entry with errors is not cached", func(t *testing.T) {
		root := setup(t, "fn answer() Int { \"nope\" }\n")
		resolver := newResolver(t, root)
		one := checkEntry(t, resolver, root, "one.ard", "app/one")
		if !one.HasErrors() {
			t.Fatalf("entry diagnostics = %#v, want an error", one.Diagnostics)
		}
		a := checkEntry(t, resolver, root, "test/a.ard", "app/test/a")
		if countFrom(a.Diagnostics, "one.ard") == 0 {
			t.Fatal("import of a failed entry should be checked and report its errors")
		}
	})
}
