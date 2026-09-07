package gotarget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"golang.org/x/mod/modfile"
)

func TestGeneratedGoModCopiesProjectModuleAndRewritesRelativeReplace(t *testing.T) {
	root := t.TempDir()
	localDep, err := filepath.Abs(filepath.Join(root, "..", "localdep"))
	if err != nil {
		t.Fatal(err)
	}
	goMod := "module example.com/app\n\ngo 1.21\n\ntoolchain go1.26.0\n\nrequire example.com/localdep v0.0.0\n\nreplace example.com/localdep => ../localdep\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{RootPath: root, ProjectName: "app"}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if !strings.Contains(generated, "module example.com/app") {
		t.Fatalf("generated go.mod did not preserve module path:\n%s", generated)
	}
	if !strings.Contains(generated, "go 1.27.0") {
		t.Fatalf("generated go.mod did not raise the project to Go 1.27:\n%s", generated)
	}
	if !strings.Contains(generated, "toolchain go1.27.0") {
		t.Fatalf("generated go.mod retained a toolchain below Go 1.27:\n%s", generated)
	}
	wantReplace := "replace example.com/localdep => " + localDep
	if !strings.Contains(generated, wantReplace) {
		t.Fatalf("generated go.mod missing rewritten replace %q:\n%s", wantReplace, generated)
	}
}

func TestGeneratedGoModPreservesNewerGoVersionAndToolchain(t *testing.T) {
	root := t.TempDir()
	goMod := "module example.com/app\n\ngo 1.28.0\n\ntoolchain go1.28.1\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{RootPath: root, ProjectName: "app"}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if !strings.Contains(generated, "go 1.28.0") || !strings.Contains(generated, "toolchain go1.28.1") {
		t.Fatalf("generated go.mod downgraded a newer toolchain:\n%s", generated)
	}
}

func TestGeneratedGoModPreservesDefaultToolchainDirective(t *testing.T) {
	root := t.TempDir()
	goMod := "module example.com/app\n\ngo 1.27.0\n\ntoolchain default\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{RootPath: root, ProjectName: "app"}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if !strings.Contains(generated, "toolchain default") {
		t.Fatalf("generated go.mod replaced the default toolchain directive:\n%s", generated)
	}
}

func TestGeneratedGoModDoesNotDuplicateExistingArdDependency(t *testing.T) {
	root := t.TempDir()
	goMod := "module example.com/app\n\ngo 1.21\n\nrequire github.com/akonwi/ard v0.0.0\n\nreplace github.com/akonwi/ard => /tmp/ard\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{RootPath: root, ProjectName: "app"}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if count := strings.Count(generated, "require github.com/akonwi/ard"); count != 1 {
		t.Fatalf("github.com/akonwi/ard require count = %d in:\n%s", count, generated)
	}
	if count := strings.Count(generated, "replace github.com/akonwi/ard"); count != 1 {
		t.Fatalf("github.com/akonwi/ard replace count = %d in:\n%s", count, generated)
	}
}

func TestGeneratedGoModUsesSyntheticModuleWithoutProjectGoMod(t *testing.T) {
	project := &checker.ProjectInfo{RootPath: t.TempDir(), ProjectName: "demo"}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if !strings.Contains(generated, "module demo") {
		t.Fatalf("generated go.mod did not use project name:\n%s", generated)
	}
	if !strings.Contains(generated, "go 1.27.0") {
		t.Fatalf("generated go.mod did not require Go 1.27:\n%s", generated)
	}
}

func TestGeneratedGoModAvoidsTransitiveModulePathOverlap(t *testing.T) {
	root := t.TempDir()
	dependency := filepath.Join(root, "dependency")
	transitive := filepath.Join(root, "transitive")
	for _, dir := range []string{dependency, transitive} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dependencyMod := "module example.com/dep\n\ngo 1.27.0\n\nrequire demo v0.0.0\n\nreplace demo => ../transitive\n"
	if err := os.WriteFile(filepath.Join(dependency, "go.mod"), []byte(dependencyMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(transitive, "go.mod"), []byte("module demo\n\ngo 1.27.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{
		RootPath:    filepath.Join(root, "consumer"),
		ProjectName: "demo",
		Dependencies: map[string]checker.DependencyInfo{
			"dep": {Alias: "dep", SourcePath: dependency, RootPath: dependency},
		},
	}
	if err := os.MkdirAll(project.RootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if strings.HasPrefix(generated, "module demo\n") || !strings.Contains(generated, "module ard-generated") {
		t.Fatalf("generated module overlaps transitive module demo:\n%s", generated)
	}

	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(output, "go.mod"), []byte(generated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "main.go"), []byte("package main\n\nimport _ \"demo\"\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(transitive, "demo.go"), []byte("package demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := buildGeneratedProgram(output, filepath.Join(output, "demo-bin")); err != nil {
		t.Fatalf("build generated module with transitive name collision: %v", err)
	}
}

func TestGeneratedGoModWiresPathDependencyModule(t *testing.T) {
	dependency := t.TempDir()
	if err := os.WriteFile(filepath.Join(dependency, "go.mod"), []byte("module example.com/dep\n\ngo 1.27.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ffiDir := filepath.Join(dependency, "ffi")
	if err := os.MkdirAll(ffiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "ffi.go"), []byte("package ffi\n\nfunc Value() string { return \"ok\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	project := &checker.ProjectInfo{
		RootPath:    t.TempDir(),
		ProjectName: "consumer",
		Dependencies: map[string]checker.DependencyInfo{
			"dep": {
				Alias:      "dep",
				SourcePath: dependency,
				RootPath:   dependency,
			},
		},
	}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if !strings.Contains(generated, "example.com/dep v0.0.0") {
		t.Fatalf("generated go.mod missing path dependency requirement:\n%s", generated)
	}
	wantReplace := "example.com/dep => " + dependency
	if !strings.Contains(generated, wantReplace) {
		t.Fatalf("generated go.mod missing path dependency replacement %q:\n%s", wantReplace, generated)
	}

	output := t.TempDir()
	if err := os.WriteFile(filepath.Join(output, "go.mod"), []byte(generated), 0o644); err != nil {
		t.Fatal(err)
	}
	mainSource := "package main\n\nimport \"example.com/dep/ffi\"\n\nfunc main() { _ = ffi.Value() }\n"
	if err := os.WriteFile(filepath.Join(output, "main.go"), []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := buildGeneratedProgram(output, filepath.Join(output, "consumer-bin")); err != nil {
		t.Fatalf("build generated program with path dependency FFI: %v", err)
	}
}

func TestGeneratedGoModUsesMajorDependencyVersionAndPromotesRelativeReplace(t *testing.T) {
	root := t.TempDir()
	dependency := filepath.Join(root, "dependency with space")
	helper := filepath.Join(root, "helper with space")
	for _, dir := range []string{dependency, helper} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dependencyMod := "module example.com/dep/v2\n\ngo 1.27.0\n\nrequire example.com/helper v0.0.0\n\nreplace example.com/helper => \"../helper with space\"\n"
	if err := os.WriteFile(filepath.Join(dependency, "go.mod"), []byte(dependencyMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helper, "go.mod"), []byte("module example.com/helper\n\ngo 1.27.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{
		RootPath:    filepath.Join(root, "consumer"),
		ProjectName: "consumer",
		Dependencies: map[string]checker.DependencyInfo{
			"dep": {Alias: "dep", SourcePath: dependency, RootPath: dependency},
		},
	}
	if err := os.MkdirAll(project.RootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if !strings.Contains(generated, "example.com/dep/v2 v2.0.0") {
		t.Fatalf("generated go.mod has invalid major dependency version:\n%s", generated)
	}
	wantRootReplace := "example.com/dep/v2 => " + modfile.AutoQuote(dependency)
	if !strings.Contains(generated, wantRootReplace) {
		t.Fatalf("generated go.mod missing quoted dependency root %q:\n%s", wantRootReplace, generated)
	}
	wantReplace := "example.com/helper => " + modfile.AutoQuote(helper)
	if !strings.Contains(generated, wantReplace) {
		t.Fatalf("generated go.mod missing promoted dependency replacement %q:\n%s", wantReplace, generated)
	}
}

func TestGeneratedGoModLockedDependencyRootOverridesProjectReplace(t *testing.T) {
	root := t.TempDir()
	dependency := filepath.Join(root, "dependency")
	stale := filepath.Join(root, "stale")
	for _, dir := range []string{dependency, stale} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/dep\n\ngo 1.27.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	consumer := filepath.Join(root, "consumer")
	if err := os.MkdirAll(consumer, 0o755); err != nil {
		t.Fatal(err)
	}
	consumerMod := "module example.com/app\n\ngo 1.27.0\n\nrequire example.com/dep v0.0.0\n\nreplace example.com/dep => " + stale + "\n"
	if err := os.WriteFile(filepath.Join(consumer, "go.mod"), []byte(consumerMod), 0o644); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{
		RootPath: consumer,
		Dependencies: map[string]checker.DependencyInfo{
			"dep": {Alias: "dep", SourcePath: dependency, RootPath: dependency},
		},
	}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	want := "example.com/dep => " + dependency
	if !strings.Contains(generated, want) || strings.Contains(generated, "example.com/dep => "+stale) {
		t.Fatalf("generated go.mod does not pin the selected dependency root %q:\n%s", want, generated)
	}
}

func TestGeneratedGoModPreservesConsumerWildcardReplace(t *testing.T) {
	root := t.TempDir()
	consumer := filepath.Join(root, "consumer")
	dependency := filepath.Join(root, "dependency")
	trusted := filepath.Join(root, "trusted")
	stale := filepath.Join(root, "stale")
	for _, dir := range []string{consumer, dependency, trusted, stale} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	consumerMod := "module example.com/app\n\ngo 1.27.0\n\nrequire example.com/transitive v1.2.3\n\nreplace example.com/transitive => " + trusted + "\n"
	dependencyMod := "module example.com/dep\n\ngo 1.27.0\n\nrequire example.com/transitive v1.2.3\n\nreplace example.com/transitive v1.2.3 => " + stale + "\n"
	if err := os.WriteFile(filepath.Join(consumer, "go.mod"), []byte(consumerMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependency, "go.mod"), []byte(dependencyMod), 0o644); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{
		RootPath: consumer,
		Dependencies: map[string]checker.DependencyInfo{
			"dep": {Alias: "dep", SourcePath: dependency, RootPath: dependency},
		},
	}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if !strings.Contains(generated, "example.com/transitive => "+trusted) || strings.Contains(generated, stale) {
		t.Fatalf("dependency module overrode consumer wildcard replacement:\n%s", generated)
	}
}

func TestGeneratedGoModUsesFirstDependencyWildcardReplace(t *testing.T) {
	root := t.TempDir()
	dependencyA := filepath.Join(root, "a")
	dependencyB := filepath.Join(root, "b")
	trusted := filepath.Join(root, "trusted")
	specific := filepath.Join(root, "specific")
	stale := filepath.Join(root, "stale")
	for _, dir := range []string{dependencyA, dependencyB, trusted, specific, stale} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	aMod := "module example.com/a\n\ngo 1.27.0\n\nrequire example.com/transitive v1.2.3\n\nreplace (\n\texample.com/transitive => " + trusted + "\n\texample.com/transitive v1.2.3 => " + specific + "\n)\n"
	bMod := "module example.com/b\n\ngo 1.27.0\n\nrequire example.com/transitive v1.2.4\n\nreplace example.com/transitive v1.2.4 => " + stale + "\n"
	if err := os.WriteFile(filepath.Join(dependencyA, "go.mod"), []byte(aMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dependencyB, "go.mod"), []byte(bMod), 0o644); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{
		RootPath: filepath.Join(root, "consumer"),
		Dependencies: map[string]checker.DependencyInfo{
			"a": {Alias: "a", SourcePath: dependencyA, RootPath: dependencyA},
			"b": {Alias: "b", SourcePath: dependencyB, RootPath: dependencyB},
		},
	}
	if err := os.MkdirAll(project.RootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	if !strings.Contains(generated, "example.com/transitive => "+trusted) || !strings.Contains(generated, "example.com/transitive v1.2.3 => "+specific) || strings.Contains(generated, stale) {
		t.Fatalf("dependency replacement precedence is inconsistent:\n%s", generated)
	}
}

func TestGeneratedGoModSelectedRootOverridesDependencyVersionReplace(t *testing.T) {
	root := t.TempDir()
	dependencyA := filepath.Join(root, "a")
	dependencyB := filepath.Join(root, "b")
	staleB := filepath.Join(root, "stale-b")
	for modulePath, dir := range map[string]string{
		"example.com/a": dependencyA,
		"example.com/b": dependencyB,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		goMod := "module " + modulePath + "\n\ngo 1.27.0\n"
		if modulePath == "example.com/a" {
			goMod += "\nrequire example.com/b v1.2.3\n\nreplace example.com/b v1.2.3 => ../stale-b\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(staleB, 0o755); err != nil {
		t.Fatal(err)
	}
	consumer := filepath.Join(root, "consumer")
	if err := os.MkdirAll(consumer, 0o755); err != nil {
		t.Fatal(err)
	}
	project := &checker.ProjectInfo{
		RootPath: consumer,
		Dependencies: map[string]checker.DependencyInfo{
			"a": {Alias: "a", SourcePath: dependencyA, RootPath: dependencyA},
			"b": {Alias: "b", SourcePath: dependencyB, RootPath: dependencyB},
		},
	}
	generated, err := generatedGoMod(t.TempDir(), &air.Program{}, project)
	if err != nil {
		t.Fatalf("generatedGoMod: %v", err)
	}
	want := "example.com/b => " + dependencyB
	if !strings.Contains(generated, want) || strings.Contains(generated, "example.com/b v1.2.3 =>") || strings.Contains(generated, staleB) {
		t.Fatalf("generated go.mod does not preserve selected dependency root %q:\n%s", want, generated)
	}
}

func TestMergeGoSumTrustsProjectAndGeneratedChecksumsOnly(t *testing.T) {
	output := t.TempDir()
	projectRoot := t.TempDir()
	dependency := t.TempDir()
	if err := os.WriteFile(filepath.Join(dependency, "go.mod"), []byte("module example.com/dep\n\ngo 1.27.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	generatedSum := "example.com/generated v1.0.0 h1:generated\n"
	projectSum := "example.com/project v1.0.0 h1:project\n"
	dependencySum := "example.com/untrusted v1.0.0 h1:untrusted\n"
	for path, content := range map[string]string{
		filepath.Join(output, "go.sum"):      generatedSum,
		filepath.Join(projectRoot, "go.sum"): projectSum,
		filepath.Join(dependency, "go.sum"):  dependencySum,
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	project := &checker.ProjectInfo{
		RootPath: projectRoot,
		Dependencies: map[string]checker.DependencyInfo{
			"dep": {Alias: "dep", SourcePath: dependency, RootPath: dependency},
		},
	}
	if err := mergeGoSum(output, &air.Program{}, project); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(output, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), strings.TrimSpace(generatedSum)) || !strings.Contains(string(got), strings.TrimSpace(projectSum)) {
		t.Fatalf("merged go.sum lost trusted checksums:\n%s", got)
	}
	if strings.Contains(string(got), "example.com/untrusted") {
		t.Fatalf("merged go.sum trusts dependency checksums:\n%s", got)
	}
}

func TestBuildGeneratedProgramUsesConfiguredBuildTags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tagged\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tagged.go"), []byte("//go:build special\n\npackage main\n\nfunc init() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "needs_tag.go"), []byte("//go:build !special\n\npackage main\n\nfunc init() { missingSymbol() }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := buildGeneratedProgram(dir, filepath.Join(dir, "tagged-bin"), "special"); err != nil {
		t.Fatalf("buildGeneratedProgram with tag: %v", err)
	}
}

func TestWriteProgramCopiesProjectFFIDirectory(t *testing.T) {
	root := t.TempDir()
	ffiDir := filepath.Join(root, "ffi", "sub")
	if err := os.MkdirAll(ffiDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "shim.go"), []byte("package sub\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ffiDir, "shim_test.go"), []byte("package sub\n"), 0644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ffiDir, "outside_link.txt")); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	project := &checker.ProjectInfo{RootPath: root, ProjectName: "demo"}
	if err := copyProjectFFIDir(out, project); err != nil {
		t.Fatalf("copyProjectFFIDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "ffi", "sub", "shim.go")); err != nil {
		t.Fatalf("copied shim.go missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "ffi", "sub", "shim_test.go")); !os.IsNotExist(err) {
		t.Fatalf("shim_test.go should not be copied, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "ffi", "sub", "outside_link.txt")); !os.IsNotExist(err) {
		t.Fatalf("symlink should not be copied, stat err=%v", err)
	}
}
