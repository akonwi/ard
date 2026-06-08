package zig

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/frontend"
	"github.com/akonwi/ard/parse"
)

func TestGenerateSourcesPrimitiveProgram(t *testing.T) {
	program := lowerSource(t, `
		extern fn print(value: Str) Void = {
			zig = "print"
		}

		fn add(a: Int, b: Int) Int {
			a + b
		}

		fn main() {
			let sum = add(1, 2)
			print("sum: " + sum.to_str())
		}
	`)

	sources, err := GenerateSources(program, Options{})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	main := string(sources["main.zig"])
	runtime := string(sources["ard_runtime.zig"])

	for _, want := range []string{
		`const ard = @import("ard_runtime.zig");`,
		`fn ard_fn_0_add(ctx: *ard.Context, l0_a: i64, l1_b: i64) !i64`,
		`return (l0_a + l1_b);`,
		`const l0_sum: i64 = try ard_fn_0_add(ctx, 1, 2);`,
		`try ard.print(ctx, try ard.concat(ctx, "sum: ", try ard.toStr(ctx, l0_sum)));`,
		`try ard_fn_1_main(&ctx);`,
	} {
		if !strings.Contains(main, want) {
			t.Fatalf("generated main source missing %q:\n%s", want, main)
		}
	}

	for _, want := range []string{
		`pub const Context = struct`,
		`allocator: std.mem.Allocator`,
		`io: std.Io`,
		`pub fn print(ctx: *Context, value: []const u8) !void`,
		`pub fn concat(ctx: *Context, left: []const u8, right: []const u8) ![]const u8`,
	} {
		if !strings.Contains(runtime, want) {
			t.Fatalf("generated runtime source missing %q:\n%s", want, runtime)
		}
	}
}

func TestGenerateSourcesIfExpression(t *testing.T) {
	program := lowerSource(t, `
		fn choose(flag: Bool) Int {
			if flag {
				1
			} else {
				2
			}
		}
	`)

	sources, err := GenerateSources(program, Options{})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	main := string(sources["main.zig"])
	if !strings.Contains(main, `return if (l0_flag) 1 else 2;`) {
		t.Fatalf("generated source missing if expression:\n%s", main)
	}
}

func TestBuildProgramPrimitiveProgramCompilesWithZig(t *testing.T) {
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skipf("zig not installed: %v", err)
	}
	program := lowerSource(t, `
		extern fn print(value: Str) Void = {
			zig = "print"
		}

		fn add(a: Int, b: Int) Int {
			a + b
		}

		fn main() {
			let sum = add(1, 2)
			print("sum: " + sum.to_str())
		}
	`)

	outputPath := filepath.Join(t.TempDir(), "primitive")
	if _, err := BuildProgram(program, outputPath); err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
}

func TestRunVariablesSample(t *testing.T) {
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skipf("zig not installed: %v", err)
	}
	program := lowerFile(t, filepath.Join("..", "samples", "variables.ard"))

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", "samples/variables.ard"})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"Hello, World!",
		"name = Alice",
		"age = 30",
		"is_student = true",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunFizzbuzzSample(t *testing.T) {
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skipf("zig not installed: %v", err)
	}
	program := lowerFile(t, filepath.Join("..", "samples", "fizzbuzz.ard"))

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", "samples/fizzbuzz.ard"})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"1",
		"2",
		"Fizz",
		"4",
		"Buzz",
		"Fizz",
		"7",
		"8",
		"Fizz",
		"Buzz",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunLoopsSample(t *testing.T) {
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skipf("zig not installed: %v", err)
	}
	program := lowerFile(t, filepath.Join("..", "samples", "loops.ard"))

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", "samples/loops.ard"})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"1",
		"2",
		"3",
		"4",
		"5",
		"6",
		"7",
		"8",
		"9",
		"10",
		"counting from 1 to 3",
		"1",
		"2",
		"3",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunTemperaturesSample(t *testing.T) {
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skipf("zig not installed: %v", err)
	}
	program := lowerFile(t, filepath.Join("..", "samples", "temperatures.ard"))

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", "samples/temperatures.ard"})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"0 F = -17.78 C",
		"20 F = -6.67 C",
		"40 F = 4.44 C",
		"60 F = 15.56 C",
		"80 F = 26.67 C",
		"100 F = 37.78 C",
		"120 F = 48.89 C",
		"140 F = 60.00 C",
		"160 F = 71.11 C",
		"180 F = 82.22 C",
		"200 F = 93.33 C",
		"220 F = 104.44 C",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunGradesSample(t *testing.T) {
	if _, err := exec.LookPath("zig"); err != nil {
		t.Skipf("zig not installed: %v", err)
	}
	program := lowerFile(t, filepath.Join("..", "samples", "grades.ard"))

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", "samples/grades.ard"})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"Alice got a 95",
		"Bob got a 82",
		"Charlie got a 88",
		"Class average is 88",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func lowerSource(t *testing.T, input string) *air.Program {
	t.Helper()
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{Target: backend.TargetZig})
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}

func runProgramCaptureStdout(program *air.Program, args []string) (string, error) {
	var stdout bytes.Buffer
	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = oldStdout
	}()

	runErr := RunProgram(program, args)
	if closeErr := writePipe.Close(); closeErr != nil {
		return "", closeErr
	}
	if _, copyErr := stdout.ReadFrom(readPipe); copyErr != nil {
		return "", copyErr
	}
	if closeErr := readPipe.Close(); closeErr != nil {
		return "", closeErr
	}
	return stdout.String(), runErr
}

func lowerFile(t *testing.T, path string) *air.Program {
	t.Helper()
	loaded, err := frontend.LoadModule(path, backend.TargetZig)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}
