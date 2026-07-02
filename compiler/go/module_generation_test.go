package gotarget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
)

func TestGeneratedGoModCopiesProjectModuleAndRewritesRelativeReplace(t *testing.T) {
	root := t.TempDir()
	localDep, err := filepath.Abs(filepath.Join(root, "..", "localdep"))
	if err != nil {
		t.Fatal(err)
	}
	goMod := "module example.com/app\n\ngo 1.21\n\nrequire example.com/localdep v0.0.0\n\nreplace example.com/localdep => ../localdep\n"
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
	wantReplace := "replace example.com/localdep => " + localDep
	if !strings.Contains(generated, wantReplace) {
		t.Fatalf("generated go.mod missing rewritten replace %q:\n%s", wantReplace, generated)
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
