package javascript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/backend"
)

func TestBuildWritesSimpleJavaScriptModule(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn choose(num: Int) Int {
  if num > 1 {
    10
  } else {
    20
  }
}

let result = choose(2)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	builtPath, err := Build(mainPath, outputPath, backend.TargetJSBrowser)
	if err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("expected built path %q, got %q", outputPath, builtPath)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "function choose(num) {") {
		t.Fatalf("expected function definition in output, got:\n%s", source)
	}
	if !strings.Contains(source, "const result = choose(2);") {
		t.Fatalf("expected top-level let emission in output, got:\n%s", source)
	}
	if !strings.Contains(source, "export { choose, result };") {
		t.Fatalf("expected exports in output, got:\n%s", source)
	}
}

func TestBuildWritesImportedUserModules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utils.ard"), []byte(`
fn add(a: Int, b: Int) Int {
  a + b
}
`), 0o644); err != nil {
		t.Fatalf("failed to write utils module: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use demo/utils

let result = utils::add(1, 2)
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	outputPath := filepath.Join(dir, "main.mjs")
	if _, err := Build(mainPath, outputPath, backend.TargetJSServer); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}

	out, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated module: %v", err)
	}
	source := string(out)
	if !strings.Contains(source, "const __module_demo_utils = (() => {") {
		t.Fatalf("expected imported module wrapper, got:\n%s", source)
	}
	if !strings.Contains(source, "const result = __module_demo_utils.add(1, 2);") {
		t.Fatalf("expected imported module call, got:\n%s", source)
	}
}

func TestRunRejectsBrowserTarget(t *testing.T) {
	err := Run("main.ard", backend.TargetJSBrowser, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "js-browser cannot be run directly") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunExecutesSimpleServerProgram(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainPath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
fn main() Int {
  1
}
`), 0o644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	if err := Run(mainPath, backend.TargetJSServer, []string{"ard", "run", mainPath, "--target", "js-server"}); err != nil {
		t.Fatalf("did not expect error: %v", err)
	}
}
