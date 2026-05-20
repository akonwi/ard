package ffi

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateGoStdlibLoweringSkipsUnimplementedBindings(t *testing.T) {
	tmp := t.TempDir()
	externDir := filepath.Join(tmp, "std_lib")
	if err := os.Mkdir(externDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externDir, "demo.ard"), []byte(strings.Join([]string{
		`extern fn present(value: Str) Str = "Present"`,
		`extern fn missing(value: Str) Str = "Missing"`,
		`extern fn json_encode(value: $T) Str!Str = "JsonEncode"`,
		`extern fn parse(input: Str) $T!Str = "JsonParse"`,
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	implDir := filepath.Join(tmp, "ffi")
	if err := os.Mkdir(implDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(implDir, "host.go"), []byte(`package ffi

func Present(value string) string { return value }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	goOut := filepath.Join(tmp, "stdlib_ffi.gen.go")
	cmd := exec.Command("go", "run", "generate.go",
		"-extern-dir", externDir,
		"-host-out", filepath.Join(tmp, "ard.gen.go"),
		"-vm-out", filepath.Join(tmp, "vm_adapters.gen.go"),
		"-go-out", goOut,
		"-go-impl-dir", implDir,
	)
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run generate.go failed: %v\n%s", err, output)
	}
	generated, err := os.ReadFile(goOut)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(generated), `switch binding`) {
		t.Fatalf("generated lowering should use contract metadata instead of a binding switch:\n%s", generated)
	}
	if !strings.Contains(string(generated), `"Present"`) || !strings.Contains(string(generated), `function: "Present"`) {
		t.Fatalf("generated lowering missing Present metadata:\n%s", generated)
	}
	if strings.Contains(string(generated), `"Missing":`) {
		t.Fatalf("generated lowering includes unimplemented Missing case:\n%s", generated)
	}
	if !strings.Contains(string(generated), `"JsonEncode"`) || !strings.Contains(string(generated), `generatedStdlibExternJSONEncode`) {
		t.Fatalf("generated lowering missing special JsonEncode kind:\n%s", generated)
	}
	if !strings.Contains(string(generated), `"JsonParse"`) || !strings.Contains(string(generated), `generatedStdlibExternJSONParse`) {
		t.Fatalf("generated lowering missing special JsonParse kind:\n%s", generated)
	}
}

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
