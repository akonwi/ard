package zig

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
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
