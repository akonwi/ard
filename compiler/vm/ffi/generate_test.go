package ffi

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRejectsDuplicateExternBinding(t *testing.T) {
	tmp := t.TempDir()
	externDir := filepath.Join(tmp, "std_lib")
	if err := os.Mkdir(externDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externDir, "dup.ard"), []byte(strings.Join([]string{
		`extern fn first(value: Str) Void = "DuplicateBinding"`,
		`extern fn second(value: Str) Void = "DuplicateBinding"`,
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "generate.go",
		"-extern-dir", externDir,
		"-host-out", filepath.Join(tmp, "ard.gen.go"),
		"-vm-out", filepath.Join(tmp, "vm_adapters.gen.go"),
	)
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("go run generate.go succeeded, want duplicate binding error\n%s", output)
	}
	if !strings.Contains(string(output), "duplicate extern binding DuplicateBinding") {
		t.Fatalf("error output = %s, want duplicate binding message", output)
	}
}
