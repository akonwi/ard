package checker

import (
	"bytes"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestGoPackagesResolverLoadsExportDataWithoutUnusedTypeChecking(t *testing.T) {
	resolver := NewGoPackagesResolver(t.TempDir(), nil)
	mode := resolver.loadConfig().Mode
	if mode&packages.NeedExportFile == 0 {
		t.Fatal("load mode does not request compiler export data")
	}
	if mode&packages.NeedTypes != 0 {
		t.Fatal("load mode asks go/packages to duplicate export-data type loading")
	}
	if mode&packages.NeedTypesInfo != 0 {
		t.Fatal("load mode requests unused expression type information")
	}
}

func TestExternalGoPackagesDriverDetection(t *testing.T) {
	t.Setenv("GOPACKAGESDRIVER", "/process/driver")
	if !externalGoPackagesDriverConfigured(&packages.Config{}) {
		t.Fatal("process GOPACKAGESDRIVER was ignored")
	}
	if externalGoPackagesDriverConfigured(&packages.Config{Env: append(os.Environ(), "GOPACKAGESDRIVER=off")}) {
		t.Fatal("config GOPACKAGESDRIVER=off did not override process driver")
	}

	t.Setenv("GOPACKAGESDRIVER", "off")
	if externalGoPackagesDriverConfigured(&packages.Config{}) {
		t.Fatal("process GOPACKAGESDRIVER=off did not select direct go list")
	}
	configEnv := append(os.Environ(), "GOPACKAGESDRIVER=/config/driver")
	if !externalGoPackagesDriverConfigured(&packages.Config{Env: configEnv}) {
		t.Fatal("config GOPACKAGESDRIVER did not override process off")
	}
}

func TestGoListArgsDoNotInjectBuildParallelism(t *testing.T) {
	args := goListArgs(&packages.Config{}, []string{"fmt"})
	for _, arg := range args {
		name := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
		if name == "p" {
			t.Fatalf("go list args override build parallelism: %v", args)
		}
	}
}

func TestOrderGoListRootsByDependency(t *testing.T) {
	tests := []struct {
		name       string
		loaded     []string
		imports    map[string][]string
		errorPaths map[string]bool
		want       []string
	}{
		{
			name:    "chain",
			loaded:  []string{"leaf", "middle", "root"},
			imports: map[string][]string{"root": {"middle"}, "middle": {"leaf"}},
			want:    []string{"root", "middle", "leaf"},
		},
		{
			name:    "shared dependency",
			loaded:  []string{"shared", "left", "right"},
			imports: map[string][]string{"left": {"shared"}, "right": {"shared"}},
			want:    []string{"left", "right", "shared"},
		},
		{
			name:    "independent roots preserve input order",
			loaded:  []string{"second", "first", "third"},
			imports: map[string][]string{"second": {"outside"}, "first": {"first"}},
			want:    []string{"second", "first", "third"},
		},
		{
			name:       "cycle and errored root remain deterministic",
			loaded:     []string{"cycle-b", "broken", "cycle-a"},
			imports:    map[string][]string{"cycle-a": {"cycle-b"}, "cycle-b": {"cycle-a"}},
			errorPaths: map[string]bool{"broken": true},
			want:       []string{"broken", "cycle-b", "cycle-a"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loaded := make([]*packages.Package, 0, len(test.loaded))
			for _, path := range test.loaded {
				pkg := &packages.Package{PkgPath: path}
				if test.errorPaths[path] {
					pkg.Errors = []packages.Error{{Msg: "load failed", Kind: packages.ListError}}
				}
				loaded = append(loaded, pkg)
			}

			ordered := orderGoListRootsByDependency(loaded, test.imports)
			got := make([]string, 0, len(ordered))
			for _, pkg := range ordered {
				got = append(got, pkg.PkgPath)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("ordered roots = %v, want %v", got, test.want)
			}
		})
	}
}

func TestExportDataRootsReuseDependencyTypeIdentity(t *testing.T) {
	loaded, err := loadGoListPackages(&packages.Config{Dir: t.TempDir()}, []string{"net/http", "context"})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]*packages.Package{}
	for _, pkg := range loaded {
		byPath[pkg.PkgPath] = pkg
	}
	httpPkg := byPath["net/http"]
	contextPkg := byPath["context"]
	if httpPkg == nil || contextPkg == nil {
		t.Fatalf("loaded packages = %v", byPath)
	}
	if errorsByPath := loadPackageExportData([]*packages.Package{httpPkg, contextPkg}); len(errorsByPath) > 0 {
		t.Fatalf("load export data: %v", errorsByPath)
	}
	newRequest, ok := httpPkg.Types.Scope().Lookup("NewRequestWithContext").(*types.Func)
	if !ok {
		t.Fatal("net/http.NewRequestWithContext not found")
	}
	signature := newRequest.Type().(*types.Signature)
	contextType := signature.Params().At(0).Type().(*types.Named)
	if contextType.Obj().Pkg() != contextPkg.Types {
		t.Fatal("explicit context root did not reuse net/http's imported context package")
	}
}

func TestExportDataFallbackTriggersForMissingAndInvalidArchives(t *testing.T) {
	missing := &packages.Package{PkgPath: "example.com/missing"}
	errorsByPath := loadPackageExportData([]*packages.Package{missing})
	if errorsByPath[missing.PkgPath] == nil {
		t.Fatal("missing export data did not request source fallback")
	}

	invalidPath := filepath.Join(t.TempDir(), "invalid.a")
	if err := os.WriteFile(invalidPath, []byte("not Go export data"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalid := &packages.Package{PkgPath: "example.com/invalid", ExportFile: invalidPath}
	errorsByPath = loadPackageExportData([]*packages.Package{invalid})
	if errorsByPath[invalid.PkgPath] == nil {
		t.Fatal("invalid export data did not request source fallback")
	}
}

func TestGoListCgoFilesPreserveLocalFFIBoundaryValidation(t *testing.T) {
	root := t.TempDir()
	outsideDir := filepath.Join(root, "internal")
	resolver := &GoPackagesResolver{ProjectRoot: root, modulePath: "example.com/app"}
	listed := goListPackage{Dir: outsideDir, CgoFiles: []string{"cgo.go"}}
	pkg := &packages.Package{GoFiles: goListSourceFiles(listed)}

	if err := resolver.validateLocalFFIBoundary("example.com/app/internal", pkg); err == nil {
		t.Fatal("cgo-only project package outside ffi was accepted")
	}
}

func TestReadonlyModuleCompletionErrorClassification(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "missing sum", message: "missing go.sum entry for module providing package example.com/dep", want: true},
		{name: "mod update", message: "go: updates to go.mod needed; to update it: go mod tidy", want: true},
		{name: "sum update", message: "updates to go.sum needed, disabled by -mod=readonly", want: true},
		{name: "readonly lookup", message: "cannot find module providing package example.com/dep: import lookup disabled by -mod=readonly", want: true},
		{name: "checksum mismatch", message: "SECURITY ERROR: checksum mismatch", want: false},
		{name: "source error", message: "broken.go: expected declaration", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isReadonlyModuleCompletionError(test.message); got != test.want {
				t.Fatalf("isReadonlyModuleCompletionError(%q) = %v, want %v", test.message, got, test.want)
			}
		})
	}
}

func TestDependencyReplaceOverlayUsesCanonicalModuleOrder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver := NewGoPackagesResolver(root, nil)
	resolver.DependencyModuleRoots = map[string]string{
		"example.com/z_last":  t.TempDir(),
		"example.com/a_first": t.TempDir(),
	}

	overlay := resolver.dependencyReplaceOverlay()
	got := string(overlay[filepath.Join(root, "go.mod")])
	firstRequire := strings.Index(got, "example.com/a_first v0.0.0")
	lastRequire := strings.Index(got, "example.com/z_last v0.0.0")
	firstReplace := strings.Index(got, "replace example.com/a_first")
	lastReplace := strings.Index(got, "replace example.com/z_last")
	if firstRequire < 0 || lastRequire < 0 || firstRequire > lastRequire {
		t.Fatalf("dependency requirements are not canonical:\n%s", got)
	}
	if firstReplace < 0 || lastReplace < 0 || firstReplace > lastReplace {
		t.Fatalf("dependency replacements are not canonical:\n%s", got)
	}
}

func TestDependencyModfilesTrustOnlyProjectChecksums(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("HOME", cacheRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("LOCALAPPDATA", cacheRoot)
	root := t.TempDir()
	dependency := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rootSum := "example.com/root v1.0.0 h1:root\n"
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte(rootSum), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependency, "go.mod"), []byte("module example.com/dep\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependency, "go.sum"), []byte("example.com/untrusted v1.0.0 h1:untrusted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver := NewGoPackagesResolver(root, nil)
	resolver.DependencyModuleRoots = map[string]string{"example.com/dep": dependency}
	readonlyConfig, readonlyCleanup, err := resolver.loadConfigWithDependencies()
	if err != nil {
		t.Fatal(err)
	}
	defer readonlyCleanup()
	writableConfig, writableCleanup, available, err := resolver.loadWritableConfigWithDependencies()
	if err != nil {
		t.Fatal(err)
	}
	defer writableCleanup()
	if !available {
		t.Fatal("writable dependency module retry is unavailable")
	}

	for name, cfg := range map[string]*packages.Config{"readonly": readonlyConfig, "writable": writableConfig} {
		var modPath string
		for _, flag := range cfg.BuildFlags {
			if strings.HasPrefix(flag, "-modfile=") {
				modPath = strings.TrimPrefix(flag, "-modfile=")
			}
		}
		if modPath == "" {
			t.Fatalf("%s dependency config has no -modfile", name)
		}
		got, err := os.ReadFile(strings.TrimSuffix(modPath, ".mod") + ".sum")
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != rootSum {
			t.Fatalf("%s go.sum = %q, want only project checksums %q", name, got, rootSum)
		}
	}
}

func TestWriteCachedGoModuleFileIsAtomicForConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ard.mod")
	content := []byte("module example.com/app\n\ngo 1.27\n")

	const writers = 16
	var wg sync.WaitGroup
	errors := make(chan error, writers)
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- writeCachedGoModuleFile(path, content)
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent cache write: %v", err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("cached content = %q, want %q", got, content)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".ard-go-mod-") {
			t.Fatalf("temporary cache file was not cleaned up: %s", entry.Name())
		}
	}

	replacement := []byte("module example.com/replaced\n\ngo 1.27\n")
	if err := writeCachedGoModuleFile(path, replacement); err != nil {
		t.Fatalf("replace cached content: %v", err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, replacement) {
		t.Fatalf("replaced content = %q, want %q", got, replacement)
	}
}

func TestConstTypeFromGoRejectsUnsupportedConstantType(t *testing.T) {
	if _, reason := constTypeFromGo(types.Typ[types.UntypedComplex]); reason == "" {
		t.Fatal("expected unsupported untyped complex constant reason")
	}
}

// A Go alias of a named type is the same type as its target, so it must
// resolve through the aliased type to keep type identity across packages
// (for example `ui.Style = vaxis.Style`).
func TestTypeFromGoResolvesNamedTypeAliases(t *testing.T) {
	source := types.NewPackage("example.com/vaxis", "vaxis")
	structType := types.NewStruct(nil, nil)
	namedObj := types.NewTypeName(token.NoPos, source, "Style", nil)
	named := types.NewNamed(namedObj, structType, nil)

	aliasing := types.NewPackage("example.com/vaxis/ui", "ui")
	aliasObj := types.NewTypeName(token.NoPos, aliasing, "Style", nil)
	alias := types.NewAlias(aliasObj, named)

	resolved, reason := typeFromGo(alias)
	if reason != "" {
		t.Fatalf("typeFromGo(alias) reason = %q", reason)
	}
	foreign, ok := resolved.(*ForeignType)
	if !ok {
		t.Fatalf("typeFromGo(alias) = %T, want *ForeignType", resolved)
	}
	if foreign.Namespace != "example.com/vaxis" || foreign.Qualifier != "vaxis" || foreign.Name != "Style" {
		t.Fatalf("alias resolved to %s::%s (namespace %s), want vaxis::Style", foreign.Qualifier, foreign.Name, foreign.Namespace)
	}

	direct, reason := typeFromGo(named)
	if reason != "" {
		t.Fatalf("typeFromGo(named) reason = %q", reason)
	}
	if !resolved.equal(direct) {
		t.Fatal("alias and aliased type should be the same Ard type")
	}
}

// An alias of a bare basic type keeps its own named identity, matching the
// pre-existing scalar alias behavior.
func TestTypeFromGoKeepsBasicAliasIdentity(t *testing.T) {
	source := types.NewPackage("example.com/pkg", "pkg")
	aliasObj := types.NewTypeName(token.NoPos, source, "Name", nil)
	alias := types.NewAlias(aliasObj, types.Typ[types.String])

	resolved, reason := typeFromGo(alias)
	if reason != "" {
		t.Fatalf("typeFromGo(alias) reason = %q", reason)
	}
	foreign, ok := resolved.(*ForeignType)
	if !ok {
		t.Fatalf("typeFromGo(alias) = %T, want *ForeignType", resolved)
	}
	if foreign.Qualifier != "pkg" || foreign.Name != "Name" {
		t.Fatalf("basic alias resolved to %s::%s, want pkg::Name", foreign.Qualifier, foreign.Name)
	}
	if !foreign.Underlying.equal(Str) {
		t.Fatalf("basic alias underlying = %s, want Str", foreign.Underlying)
	}
}

// A named empty Go interface (`type Event interface{}`) keeps its type
// identity instead of collapsing to Any, so signatures naming it lower to the
// exact Go type. Only the unnamed empty interface maps to Any.
func TestTypeFromGoKeepsNamedEmptyInterfaceIdentity(t *testing.T) {
	source := types.NewPackage("example.com/vaxis", "vaxis")
	ifaceObj := types.NewTypeName(token.NoPos, source, "Event", nil)
	named := types.NewNamed(ifaceObj, types.NewInterfaceType(nil, nil), nil)

	resolved, reason := typeFromGo(named)
	if reason != "" {
		t.Fatalf("typeFromGo(named empty interface) reason = %q", reason)
	}
	foreign, ok := resolved.(*ForeignType)
	if !ok {
		t.Fatalf("typeFromGo = %T, want *ForeignType", resolved)
	}
	if !foreign.Interface || foreign.Name != "Event" || foreign.Qualifier != "vaxis" {
		t.Fatalf("named empty interface resolved to %#v, want vaxis::Event interface", foreign)
	}
	if !foreign.EmptyInterface() {
		t.Fatal("EmptyInterface() = false, want true")
	}

	if unnamed, reason := typeFromGo(types.NewInterfaceType(nil, nil)); reason != "" || unnamed != Any {
		t.Fatalf("unnamed empty interface = %v (%q), want Any", unnamed, reason)
	}
}

// A named Go func type (`type VoidCallback func(...)`) keeps its identity so
// generated Go names the exact type; its Underlying carries the signature.
func TestTypeFromGoKeepsNamedFuncIdentity(t *testing.T) {
	source := types.NewPackage("example.com/ui", "ui")
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewParam(token.NoPos, source, "v", types.Typ[types.String])),
		nil, false)
	fnObj := types.NewTypeName(token.NoPos, source, "Callback", nil)
	named := types.NewNamed(fnObj, sig, nil)

	resolved, reason := typeFromGo(named)
	if reason != "" {
		t.Fatalf("typeFromGo(named func) reason = %q", reason)
	}
	foreign, ok := resolved.(*ForeignType)
	if !ok {
		t.Fatalf("typeFromGo = %T, want *ForeignType", resolved)
	}
	if foreign.Name != "Callback" || foreign.Qualifier != "ui" {
		t.Fatalf("named func resolved to %s::%s, want ui::Callback", foreign.Qualifier, foreign.Name)
	}
	fn, ok := foreign.Underlying.(*FunctionDef)
	if !ok {
		t.Fatalf("Underlying = %T, want *FunctionDef", foreign.Underlying)
	}
	if len(fn.Parameters) != 1 || !fn.Parameters[0].Type.equal(Str) {
		t.Fatalf("signature parameters = %#v, want (Str)", fn.Parameters)
	}
}

func TestGoFieldsIncludeEmbeddedFieldWithoutPromotingChildren(t *testing.T) {
	baseFields := []*types.Var{
		types.NewField(token.NoPos, nil, "Name", types.Typ[types.String], false),
	}
	base := types.NewNamed(
		types.NewTypeName(token.NoPos, nil, "Base", nil),
		types.NewStruct(baseFields, nil),
		nil,
	)
	outer := types.NewStruct(
		[]*types.Var{types.NewField(token.NoPos, nil, "Base", base, true)},
		nil,
	)

	fields, unsupported := goFieldsForStruct(outer)
	if len(unsupported) != 0 {
		t.Fatalf("unsupported fields = %v, want none", unsupported)
	}
	if fields["Base"] == nil {
		t.Fatal("embedded field Base was not exposed")
	}
	if fields["Name"] != nil {
		t.Fatal("embedded child field Name was promoted")
	}
}
