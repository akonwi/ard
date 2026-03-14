package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	compilerBin := filepath.Join(dir, "ard-test")
	buildCompiler := exec.Command("go", "build", "-tags=goexperiment.jsonv2", "-o", compilerBin, ".")
	buildCompiler.Dir = "."
	out, err := buildCompiler.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build compiler: %v\n%s", err, out)
	}

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(projectDir, "test"), 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"demo\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainSource := `use ard/testing
use ard/maybe

test fn passes() Void!Str {
  try testing::assert(true, maybe::none())
  try testing::equal(1 + 1, 2)
  try testing::not_equal(1, 2)
  Result::ok(())
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
		cmd := exec.Command(compilerBin, "test", "--filter", "passes", projectDir)
		cmd.Dir = "."
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("test command failed: %v\n%s", err, out)
		}
		output := string(out)
		if !strings.Contains(output, "PASS") || !strings.Contains(output, "1 passed; 0 failed; 0 panicked") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})

	t.Run("fail and panic classification", func(t *testing.T) {
		cmd := exec.Command(compilerBin, "test", "--filter", "failures", projectDir)
		cmd.Dir = "."
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected failing test command to exit non-zero\n%s", out)
		}
		output := string(out)
		if !strings.Contains(output, "FAIL") || !strings.Contains(output, "PANIC") || !strings.Contains(output, "0 passed; 1 failed; 1 panicked") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})

	t.Run("fail fast stops after first failure", func(t *testing.T) {
		cmd := exec.Command(compilerBin, "test", "--fail-fast", "--filter", "failures", projectDir)
		cmd.Dir = "."
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected failing test command to exit non-zero\n%s", out)
		}
		output := string(out)
		if strings.Contains(output, "PANIC") || !strings.Contains(output, "0 passed; 1 failed; 0 panicked") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})
}

func TestTestCommandRespectsPrivateAccessInTestDir(t *testing.T) {
	dir := t.TempDir()
	compilerBin := filepath.Join(dir, "ard-test")
	buildCompiler := exec.Command("go", "build", "-tags=goexperiment.jsonv2", "-o", compilerBin, ".")
	buildCompiler.Dir = "."
	out, err := buildCompiler.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build compiler: %v\n%s", err, out)
	}

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(projectDir, "test"), 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"demo\"\n"), 0o644); err != nil {
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

	cmd := exec.Command(compilerBin, "test", projectDir)
	cmd.Dir = "."
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected private access test command to exit non-zero\n%s", out)
	}
	output := string(out)
	if !strings.Contains(output, "Undefined: utils::private_helper") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestStandaloneBuildPreservesRuntimeArgs(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "argv.ard")
	source := `use ard/argv
use ard/io

fn main() {
  let args = argv::load()
  match args.arguments.size() {
    0 => io::print("missing"),
    _ => io::print(args.arguments.at(0)),
  }
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	compilerBin := filepath.Join(dir, "ard-test")
	buildCompiler := exec.Command("go", "build", "-tags=goexperiment.jsonv2", "-o", compilerBin, ".")
	buildCompiler.Dir = "."
	out, err := buildCompiler.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build compiler: %v\n%s", err, out)
	}

	t.Run("interpreter mode", func(t *testing.T) {
		cmd := exec.Command(compilerBin, "run", sourcePath, "up")
		cmd.Dir = "."
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("interpreter run failed: %v\n%s", err, out)
		}
		if string(out) != "up\n" {
			t.Fatalf("expected interpreter output %q, got %q", "up\\n", string(out))
		}
	})

	t.Run("compiled binary mode", func(t *testing.T) {
		programBin := filepath.Join(dir, "argv-bin")
		buildProgram := exec.Command(compilerBin, "build", sourcePath, "--out", programBin)
		buildProgram.Dir = "."
		out, err := buildProgram.CombinedOutput()
		if err != nil {
			t.Fatalf("failed to build standalone binary: %v\n%s", err, out)
		}

		cmd := exec.Command(programBin, "up")
		cmd.Dir = "."
		out, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("compiled binary failed: %v\n%s", err, out)
		}
		if string(out) != "up\n" {
			t.Fatalf("expected compiled output %q, got %q", "up\\n", string(out))
		}
	})
}
