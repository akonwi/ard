package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	gotarget "github.com/akonwi/ard/go"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close stdout writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("failed to close stdout reader: %v", err)
	}
	return string(out)
}

func TestParseRunArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		target     string
		expectErr  bool
		errMessage string
	}{
		{
			name:   "input only",
			args:   []string{"samples/main.ard"},
			path:   "samples/main.ard",
			target: "",
		},
		{
			name:   "explicit target",
			args:   []string{"--target", "go", "samples/main.ard"},
			path:   "samples/main.ard",
			target: "go",
		},
		{
			name:   "vm_next target",
			args:   []string{"--target", "vm_next", "samples/main.ard"},
			path:   "samples/main.ard",
			target: "vm_next",
		},
		{
			name:       "missing target value",
			args:       []string{"--target"},
			expectErr:  true,
			errMessage: "--target requires a value",
		},
		{
			name:       "unknown target",
			args:       []string{"--target", "wasm", "samples/main.ard"},
			expectErr:  true,
			errMessage: "unknown target: wasm",
		},
		{
			name:       "unknown flag",
			args:       []string{"--watch", "samples/main.ard"},
			expectErr:  true,
			errMessage: "unknown flag: --watch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, target, err := parseRunArgs(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMessage)
				}
				if err.Error() != tt.errMessage {
					t.Fatalf("expected error %q, got %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, path)
			}
			if target != tt.target {
				t.Fatalf("expected target %q, got %q", tt.target, target)
			}
		})
	}
}

func TestRunVMNextProgram(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "main.ard")
	source := `
		mut count = 40
		count = count + 2
		count
	`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	module, err := loadModule(sourcePath, backend.TargetVMNext)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := runVMNextProgram(program, []string{"ard", "run", "--target", "vm_next", sourcePath}); err != nil {
		t.Fatalf("run vm_next: %v", err)
	}
}

func TestRunGoProgram(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "main.ard")
	source := `
		fn add(a: Int, b: Int) Int {
			a + b
		}

		fn main() Int {
			add(2, 3)
		}
	`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go target: %v", err)
	}
}

func TestRunGoTargetVariablesSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "variables.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go variables sample: %v", err)
	}
}

func TestRunGoTargetNullablesSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "nullables.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go nullables sample: %v", err)
	}
}

func TestRunGoTargetLoopsSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "loops.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go loops sample: %v", err)
	}
}

func TestRunGoTargetCollectionsSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "collections.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go collections sample: %v", err)
	}
}

func TestRunGoTargetMapsSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "maps.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go maps sample: %v", err)
	}
}

func TestRunGoTargetModulesSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "modules.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go modules sample: %v", err)
	}
}

func TestBuildGoBinary(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "main.ard")
	outputPath := filepath.Join(tempDir, "main-bin")
	source := `
		fn main() Void {
			()
		}
	`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	builtPath, err := buildGoBinary(sourcePath, outputPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("build go target: %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("built path = %q, want %q", builtPath, outputPath)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("stat built binary: %v", err)
	}
}

func TestParseTestArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		filter     string
		failFast   bool
		expectErr  bool
		errMessage string
	}{
		{
			name:     "defaults to current directory",
			args:     []string{},
			path:     ".",
			filter:   "",
			failFast: false,
		},
		{
			name:     "path and flags",
			args:     []string{"samples", "--filter", "math", "--fail-fast"},
			path:     "samples",
			filter:   "math",
			failFast: true,
		},
		{
			name:       "missing filter value",
			args:       []string{"--filter"},
			expectErr:  true,
			errMessage: "--filter requires a value",
		},
		{
			name:       "unknown flag",
			args:       []string{"--list"},
			expectErr:  true,
			errMessage: "unknown flag: --list",
		},
		{
			name:       "unexpected extra argument",
			args:       []string{"a.ard", "b.ard"},
			expectErr:  true,
			errMessage: "unexpected argument: b.ard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, filter, failFast, err := parseTestArgs(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMessage)
				}
				if err.Error() != tt.errMessage {
					t.Fatalf("expected error %q, got %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, path)
			}
			if filter != tt.filter {
				t.Fatalf("expected filter %q, got %q", tt.filter, filter)
			}
			if failFast != tt.failFast {
				t.Fatalf("expected failFast %t, got %t", tt.failFast, failFast)
			}
		})
	}
}

func TestParseBuildArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		out        string
		target     string
		expectErr  bool
		errMessage string
	}{
		{
			name:   "input only",
			args:   []string{"demo.ard"},
			path:   "demo.ard",
			out:    "demo",
			target: "",
		},
		{
			name:   "explicit output and target",
			args:   []string{"samples/main.ard", "--out", "demo", "--target", "go"},
			path:   "samples/main.ard",
			out:    "demo",
			target: "go",
		},
		{
			name:   "vm_next target",
			args:   []string{"samples/main.ard", "--out", "demo", "--target", "vm_next"},
			path:   "samples/main.ard",
			out:    "demo",
			target: "vm_next",
		},
		{
			name:       "missing target value",
			args:       []string{"samples/main.ard", "--target"},
			expectErr:  true,
			errMessage: "--target requires a value",
		},
		{
			name:       "unknown target",
			args:       []string{"samples/main.ard", "--target", "wasm"},
			expectErr:  true,
			errMessage: "unknown target: wasm",
		},
		{
			name:       "unknown flag",
			args:       []string{"samples/main.ard", "--wat"},
			expectErr:  true,
			errMessage: "unknown flag: --wat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, out, target, err := parseBuildArgs(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMessage)
				}
				if err.Error() != tt.errMessage {
					t.Fatalf("expected error %q, got %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, path)
			}
			if out != tt.out {
				t.Fatalf("expected output %q, got %q", tt.out, out)
			}
			if target != tt.target {
				t.Fatalf("expected target %q, got %q", tt.target, target)
			}
		})
	}
}

func TestParseFormatArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		checkOnly  bool
		expectErr  bool
		errMessage string
	}{
		{
			name:      "single file path",
			args:      []string{"samples/hello.ard"},
			path:      "samples/hello.ard",
			checkOnly: false,
		},
		{
			name:      "check mode",
			args:      []string{"--check", "samples/hello.ard"},
			path:      "samples/hello.ard",
			checkOnly: true,
		},
		{
			name:       "unknown flag",
			args:       []string{"--watch", "samples/hello.ard"},
			expectErr:  true,
			errMessage: "unknown flag: --watch",
		},
		{
			name:       "missing filepath",
			args:       []string{"--check"},
			expectErr:  true,
			errMessage: "expected filepath argument",
		},
		{
			name:       "unexpected extra argument",
			args:       []string{"a.ard", "b.ard"},
			expectErr:  true,
			errMessage: "unexpected argument: b.ard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, checkOnly, err := parseFormatArgs(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMessage)
				}
				if err.Error() != tt.errMessage {
					t.Fatalf("expected error %q, got %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, path)
			}
			if checkOnly != tt.checkOnly {
				t.Fatalf("expected checkOnly %t, got %t", tt.checkOnly, checkOnly)
			}
		})
	}
}

func TestFormatFile(t *testing.T) {
	t.Run("writes formatted source", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "example.ard")
		if err := os.WriteFile(path, []byte("let x = 1  \n"), 0o644); err != nil {
			t.Fatalf("failed to seed test file: %v", err)
		}

		changed, err := formatFile(path, false)
		if err != nil {
			t.Fatalf("did not expect error: %v", err)
		}
		if !changed {
			t.Fatalf("expected file to change")
		}

		out, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read formatted file: %v", err)
		}
		if string(out) != "let x = 1\n" {
			t.Fatalf("expected formatted content, got %q", string(out))
		}
	})

	t.Run("check mode does not write file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "example.ard")
		original := "let x = 1  \n"
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatalf("failed to seed test file: %v", err)
		}

		changed, err := formatFile(path, true)
		if err != nil {
			t.Fatalf("did not expect error: %v", err)
		}
		if !changed {
			t.Fatalf("expected check mode to report changes")
		}

		out, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read source file: %v", err)
		}
		if string(out) != original {
			t.Fatalf("expected file to stay unchanged, got %q", string(out))
		}
	})
}

func TestFormatPath(t *testing.T) {
	t.Run("formats directories recursively", func(t *testing.T) {
		dir := t.TempDir()
		nestedDir := filepath.Join(dir, "nested")
		if err := os.MkdirAll(nestedDir, 0o755); err != nil {
			t.Fatalf("failed to create nested dir: %v", err)
		}

		first := filepath.Join(dir, "first.ard")
		second := filepath.Join(nestedDir, "second.ard")
		if err := os.WriteFile(first, []byte("let x = 1  \n"), 0o644); err != nil {
			t.Fatalf("failed to seed first file: %v", err)
		}
		if err := os.WriteFile(second, []byte("let y = 2\n"), 0o644); err != nil {
			t.Fatalf("failed to seed second file: %v", err)
		}

		changedPaths, err := formatPath(dir, false)
		if err != nil {
			t.Fatalf("did not expect error: %v", err)
		}
		if len(changedPaths) != 1 {
			t.Fatalf("expected one changed path, got %d", len(changedPaths))
		}
		if changedPaths[0] != first && changedPaths[0] != second {
			t.Fatalf("unexpected changed path %q", changedPaths[0])
		}
	})
}

func TestTestCommand(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(projectDir, "test"), 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainSource := `use ard/testing

test fn passes() Void!Str {
  try testing::assert(true, "true should pass")
  try testing::assert(1 + 1 == 2, "math should hold")
  testing::pass()
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "main.ard"), []byte(mainSource), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}
	failureSource := `use ard/testing

test fn fails() Void!Str {
  testing::fail("nope")
}

test fn panics() Void!Str {
  panic("boom")
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "test", "failures.ard"), []byte(failureSource), 0o644); err != nil {
		t.Fatalf("failed to write test source: %v", err)
	}

	t.Run("passing filter", func(t *testing.T) {
		var ok bool
		output := captureStdout(t, func() {
			ok = runTests(projectDir, "passes", false)
		})
		if !ok {
			t.Fatalf("expected tests to pass\n%s", output)
		}
		if !strings.Contains(output, "✓") || !strings.Contains(output, "1 passed; 0 failed; 0 panicked") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})

	t.Run("fail and panic classification", func(t *testing.T) {
		var ok bool
		output := captureStdout(t, func() {
			ok = runTests(projectDir, "failures", false)
		})
		if ok {
			t.Fatalf("expected failing test command behavior\n%s", output)
		}
		if !strings.Contains(output, "✗") || !strings.Contains(output, "💥") || !strings.Contains(output, "0 passed; 1 failed; 1 panicked") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})

	t.Run("fail fast stops after first failure", func(t *testing.T) {
		var ok bool
		output := captureStdout(t, func() {
			ok = runTests(projectDir, "failures", true)
		})
		if ok {
			t.Fatalf("expected failing test command behavior\n%s", output)
		}
		if strings.Contains(output, "💥") || !strings.Contains(output, "0 passed; 1 failed; 0 panicked") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})
}

func TestTestCommandRespectsPrivateAccessInTestDir(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(projectDir, "test"), 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	utilsSource := `private fn private_helper() Int {
  42
}

fn public_helper() Int {
  7
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "utils.ard"), []byte(utilsSource), 0o644); err != nil {
		t.Fatalf("failed to write utils source: %v", err)
	}
	privateAccessSource := `use demo/utils

test fn private_access() Void!Str {
  utils::private_helper()
  Result::ok(())
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "test", "private_access.ard"), []byte(privateAccessSource), 0o644); err != nil {
		t.Fatalf("failed to write private access test: %v", err)
	}

	var ok bool
	output := captureStdout(t, func() {
		ok = runTests(projectDir, "", false)
	})
	if ok {
		t.Fatalf("expected private access test behavior to fail\n%s", output)
	}
	if !strings.Contains(output, "Undefined: utils::private_helper") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestArgsForEmbeddedProgram(t *testing.T) {
	t.Run("strips run-embedded sentinel", func(t *testing.T) {
		got := argsForEmbeddedProgram([]string{"ard", "run-embedded", "one", "two"})
		want := []string{"ard", "one", "two"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("expected %v, got %v", want, got)
		}
	})

	t.Run("returns copy for normal args", func(t *testing.T) {
		input := []string{"ard", "run", "sample.ard"}
		got := argsForEmbeddedProgram(input)
		if strings.Join(got, ",") != strings.Join(input, ",") {
			t.Fatalf("expected %v, got %v", input, got)
		}
		got[0] = "changed"
		if input[0] != "ard" {
			t.Fatalf("expected returned args to be copied")
		}
	})
}

func TestReadEmbeddedPayloadFromPath(t *testing.T) {
	if len(vmNextFooterMarker) != len(bytecodeFooterMarker) {
		t.Fatalf("vm_next footer marker length = %d, want %d", len(vmNextFooterMarker), len(bytecodeFooterMarker))
	}

	path := filepath.Join(t.TempDir(), "embedded")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create embedded fixture: %v", err)
	}
	if _, err := file.WriteString("binary-prefix"); err != nil {
		t.Fatalf("write prefix: %v", err)
	}
	payload := []byte("serialized-air")
	if _, err := file.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := writeFooter(file, vmNextFooterMarker, uint64(len(payload))); err != nil {
		t.Fatalf("write footer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}

	marker, got, err := readEmbeddedPayloadFromPath(path)
	if err != nil {
		t.Fatalf("read embedded payload: %v", err)
	}
	if marker != vmNextFooterMarker {
		t.Fatalf("marker = %q, want %q", marker, vmNextFooterMarker)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestBuildVMNextBinaryRequiresMain(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "script.ard")
	if err := os.WriteFile(sourcePath, []byte("1 + 1"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	_, err := buildVMNextBinary(sourcePath, filepath.Join(t.TempDir(), "script"))
	if err == nil {
		t.Fatal("buildVMNextBinary succeeded, want missing main error")
	}
	if !strings.Contains(err.Error(), "vm_next builds require fn main()") {
		t.Fatalf("error = %v", err)
	}
}
