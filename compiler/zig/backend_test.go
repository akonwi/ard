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

var zigPathErr error

func init() {
	_, zigPathErr = exec.LookPath("zig")
}

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
	if !strings.Contains(main, "if (l0_flag) {\n        return 1;\n    } else {\n        return 2;\n    }") {
		t.Fatalf("generated source missing if expression:\n%s", main)
	}
}

func TestGenerateSourcesFunctionValueArityLimitError(t *testing.T) {
	program := lowerSource(t, `
		fn apply9(op: fn(Int, Int, Int, Int, Int, Int, Int, Int, Int) Int) Int {
			op(1, 2, 3, 4, 5, 6, 7, 8, 9)
		}
	`)

	_, err := GenerateSources(program, Options{})
	if err == nil {
		t.Fatal("GenerateSources error = nil, want arity error")
	}
	want := "unsupported Zig function value arity 9; supported arity is 0..8"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("GenerateSources error = %q, want to contain %q", err.Error(), want)
	}
}

func TestBuildProgramPrimitiveProgramCompilesWithZig(t *testing.T) {
	requireZig(t)
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
	requireZig(t)
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
	requireZig(t)
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
	requireZig(t)
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
	requireZig(t)
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
	requireZig(t)
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

func TestRunCollectionsSample(t *testing.T) {
	requireZig(t)
	program := lowerFile(t, filepath.Join("..", "samples", "collections.ard"))

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", "samples/collections.ard"})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"numbers.size = 0",
		"adding numbers from 0 to 10",
		"numbers.size = 11",
		"0",
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
		"7th element = 6",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunMapsSample(t *testing.T) {
	requireZig(t)
	program := lowerFile(t, filepath.Join("..", "samples", "maps.ard"))

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", "samples/maps.ard"})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"size is 1",
		"entries:",
		"1 = one",
		"2 = 2",
		"3 = 3",
		"4 = 4",
		"5 = 5",
		"there is an entry for 2",
		"2 is not found",
		"entries:",
		"1 = one",
		"3 = 3",
		"4 = 4",
		"5 = 5",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunLightsSample(t *testing.T) {
	requireZig(t)
	program := lowerFile(t, filepath.Join("..", "samples", "lights.ard"))

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", "samples/lights.ard"})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"Yellow means Yield",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunIntStdlibFunctions(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/int
		use ard/io

		fn main() {
			io::print(42.to_str())
			match int::from_str("123") {
				value => io::print(value),
				_ => io::print("none")
			}
			match int::from_str("not an int") {
				value => io::print(value),
				_ => io::print("none")
			}
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"42",
		"123",
		"none",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunStrMethods(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		fn main() {
			let text = " hello hello "
			io::print(text.size())
			io::print(text.is_empty())
			io::print(text.contains("ell"))
			io::print(text.starts_with(" he"))
			io::print(text.ends_with("lo "))
			io::print(text.replace("hello", "hi"))
			io::print(text.replace_all("hello", "hi"))
			io::print(text.trim())

			let parts = "a,b,c".split(",")
			io::print(parts.size())
			io::print(parts.at(1))

			match "abc".at(1) {
				value => io::print(value),
				_ => io::print("none")
			}
			match "abc".at(3) {
				value => io::print(value),
				_ => io::print("none")
			}

			for char in "ab" {
				io::print(char)
			}
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"13",
		"false",
		"true",
		"true",
		"true",
		" hi hello ",
		" hi hi ",
		"hello hello",
		"3",
		"b",
		"b",
		"none",
		"a",
		"b",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunFloatMethodsAndStdlibFunctions(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/float
		use ard/io

		fn main() {
			io::print(3.75.to_str())
			io::print(3.75.to_int())
			io::print(float::from_int(7))
			io::print(float::floor(3.75))

			match float::from_str("12.5") {
				value => io::print(value),
				_ => io::print("none")
			}
			match float::from_str("not a float") {
				value => io::print(value),
				_ => io::print("none")
			}
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"3.75",
		"3",
		"7.00",
		"3.00",
		"12.50",
		"none",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunBoolMethodsAndOperators(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		fn main() {
			io::print(true.to_str())
			io::print(false.to_str())
			io::print(not false)
			io::print(true and false)
			io::print(true or false)
			io::print(match true {
				true => "yes",
				false => "no"
			})
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"true",
		"false",
		"true",
		"false",
		"true",
		"yes",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunEnumSemantics(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		enum Status {
			Pending,
			Active = 10,
			Archived
		}

		impl Status {
			fn label() Str {
				match self {
					Status::Pending => "pending",
					Status::Active => "active",
					Status::Archived => "archived"
				}
			}
		}

		fn describe(status: Status) Str {
			match status {
				Status::Pending => "wait",
				Status::Active => "go",
				_ => "done"
			}
		}

		fn code_label(code: Int) Str {
			match code {
				Status::Pending => "pending code",
				Status::Active => "active code",
				0..9 => "low code",
				_ => "other code"
			}
		}

		fn main() {
			let status = Status::Active
			io::print(status.label())
			io::print(describe(Status::Pending))
			io::print(describe(Status::Archived))
			io::print(status == 10)
			io::print(Status::Archived > Status::Active)
			io::print(code_label(10))
			io::print(code_label(3))
			let statuses = [Status::Pending, Status::Archived]
			io::print(statuses.at(1) == Status::Archived)
			io::print(match Status::Pending {
				Status::Pending => "direct",
				_ => "other"
			})
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"active",
		"wait",
		"done",
		"true",
		"true",
		"active code",
		"low code",
		"true",
		"direct",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunMatchExpressionSemantics(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io
		use ard/maybe

		fn describe_bool(flag: Bool) Str {
			match flag {
				true => "yes",
				false => "no"
			}
		}

		fn describe_int(value: Int) Str {
			match value {
				0 => "zero",
				1..3 => "small",
				_ => "large"
			}
		}

		fn describe_str(value: Str) Str {
			match value {
				"ard" => "language",
				"zig" => "target",
				_ => "other"
			}
		}

		fn describe_open(value: Int) Str {
			match {
				value < 0 => "negative",
				value == 0 => "zero",
				value < 10 => "small",
				_ => "large"
			}
		}

		fn describe_maybe(value: Int?) Str {
			match value {
				inner => "some {inner}",
				_ => "none"
			}
		}

		fn main() {
			io::print(describe_bool(true))
			io::print(describe_bool(false))
			io::print(describe_int(0))
			io::print(describe_int(2))
			io::print(describe_int(9))
			io::print(describe_str("ard"))
			io::print(describe_str("zig"))
			io::print(describe_str("go"))
			io::print(describe_open(-1))
			io::print(describe_open(0))
			io::print(describe_open(4))
			io::print(describe_open(12))
			io::print(describe_maybe(maybe::some(5)))
			io::print(describe_maybe(maybe::none()))
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"yes",
		"no",
		"zero",
		"small",
		"large",
		"language",
		"target",
		"other",
		"negative",
		"zero",
		"small",
		"large",
		"some 5",
		"none",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunUnionMatchSemantics(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		type Printable = Str | Int | Bool

		fn describe(value: Printable) Str {
			match value {
				Str(text) => "str {text}",
				Int(number) => "int {number}",
				_ => "bool"
			}
		}

		fn main() {
			io::print(describe("ard"))
			io::print(describe(42))
			io::print(describe(true))
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"str ard",
		"int 42",
		"bool",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunResultSemantics(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		fn divide(a: Int, b: Int) Int!Str {
			match b == 0 {
				true => Result::err("divide by zero"),
				false => Result::ok(a / b)
			}
		}

		fn add_one(value: Int!Str) Int!Str {
			let inner = try value
			Result::ok(inner + 1)
		}

		fn catch_value(value: Int!Str) Int {
			let inner = try value -> err {
				err.size()
			}
			inner
		}

		fn pass() Void!Str {
			Result::ok(())
		}

		fn require_pass() Void!Str {
			try pass()
			Result::ok(())
		}

		fn stringify(value: Int) Str {
			"value={value}"
		}

		fn make_multiplier(multiplier: Int) fn(Int) Int {
			fn(value: Int) Int {
				value * multiplier
			}
		}

		fn double_if_even(value: Int) Int!Str {
			match value % 2 == 0 {
				true => Result::ok(value * 2),
				false => Result::err("odd")
			}
		}

		fn main() {
			let ok = divide(8, 2)
			let err = divide(8, 0)
			io::print(ok.is_ok())
			io::print(ok.is_err())
			io::print(err.is_ok())
			io::print(err.is_err())
			io::print(ok.or(99))
			io::print(err.or(99))
			io::print(ok.expect("expected ok"))

			match ok {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}
			match err {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}

			match add_one(ok) {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}
			match add_one(err) {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}
			io::print(catch_value(Result::err("bad")))
			match require_pass() {
				ok(_) => io::print("pass"),
				err(message) => io::print(message)
			}

			let multiplier = make_multiplier(3)
			match ok.map(multiplier) {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}
			match err.map(multiplier) {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}
			match ok.map(stringify) {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}
			match err.map_err(fn(message) { message.size() }) {
				ok(value) => io::print(value),
				err(size) => io::print(size)
			}
			match ok.and_then(double_if_even) {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}
			match Result::ok(3).and_then(double_if_even) {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}
			match err.and_then(double_if_even) {
				ok(value) => io::print(value),
				err(message) => io::print(message)
			}
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"true",
		"false",
		"false",
		"true",
		"4",
		"99",
		"4",
		"4",
		"divide by zero",
		"5",
		"divide by zero",
		"3",
		"pass",
		"12",
		"divide by zero",
		"value=4",
		"14",
		"8",
		"odd",
		"divide by zero",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunMaybeCallbackMethods(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io
		use ard/maybe

		fn make_multiplier(multiplier: Int) fn(Int) Int {
			fn(value: Int) Int {
				value * multiplier
			}
		}

		fn keep_even(value: Int) Int? {
			match value % 2 == 0 {
				true => maybe::some(value),
				false => maybe::none()
			}
		}

		fn main() {
			let value = maybe::some(7)
			let empty: Int? = maybe::none()
			let multiplier = make_multiplier(3)

			match value.map(multiplier) {
				mapped => io::print(mapped),
				_ => io::print("none")
			}
			match empty.map(multiplier) {
				mapped => io::print(mapped),
				_ => io::print("none")
			}
			match maybe::some(8).and_then(keep_even) {
				mapped => io::print(mapped),
				_ => io::print("none")
			}
			match value.and_then(keep_even) {
				mapped => io::print(mapped),
				_ => io::print("none")
			}
			match empty.and_then(keep_even) {
				mapped => io::print(mapped),
				_ => io::print("none")
			}
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"21",
		"none",
		"8",
		"none",
		"none",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunListStdlibCallbacks(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io
		use ard/list

		fn main() {
			let values = [1, 2, 3, 4]
			let doubled = list::map(values, fn(value) { value * 2 })
			for value in doubled {
				io::print(value)
			}

			let kept = list::keep(values, fn(value) { value % 2 == 0 })
			for value in kept {
				io::print(value)
			}

			match list::find(values, fn(value) { value > 2 }) {
				found => io::print(found),
				_ => io::print("none")
			}

			let partition = list::partition_int(values, fn(value) { value <= 2 })
			io::print(partition.selected.size())
			io::print(partition.others.size())
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"2",
		"4",
		"6",
		"8",
		"2",
		"4",
		"3",
		"2",
		"2",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunFunctionValuesAndClosures(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		fn subtract(a: Int, b: Int) Int {
			a - b
		}

		fn apply(op: fn(Int, Int) Int, a: Int, b: Int) Int {
			op(a, b)
		}

		fn apply8(op: fn(Int, Int, Int, Int, Int, Int, Int, Int) Int) Int {
			op(1, 2, 3, 4, 5, 6, 7, 8)
		}

		fn make_adder(base: Int) fn(Int) Int {
			fn(value: Int) Int {
				base + value
			}
		}

		fn make_label(prefix: Str) fn() Str {
			fn() Str {
				prefix + "!"
			}
		}

		fn main() {
			io::print(apply(subtract, 30, 8))
			let add = fn(a: Int, b: Int) Int {
				a + b
			}
			io::print(apply(add, 20, 22))
			let add_five = make_adder(5)
			io::print(add_five(10))
			let label = make_label("go")
			io::print(label())
			let sum8 = fn(a: Int, b: Int, c: Int, d: Int, e: Int, f: Int, g: Int, h: Int) Int {
				a + b + c + d + e + f + g + h
			}
			io::print(apply8(sum8))
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"22",
		"42",
		"15",
		"go!",
		"36",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunStructMethodsAndMutation(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/maybe
		use ard/io

		struct Counter {
			value: Int
		}

		struct Node {
			value: Int,
			parent: Node?
		}

		fn Counter::new(value: Int) Counter {
			Counter{value: value}
		}

		impl Counter {
			fn describe() Str {
				"value={self.value}"
			}

			fn mut bump(n: Int) {
				self.value = self.value + n
			}
		}

		fn read(counter: Counter) Int {
			counter.value
		}

		fn main() {
			mut counter = Counter::new(1)
			io::print(counter.describe())
			counter.value = counter.value + 1
			counter.bump(5)
			io::print(counter.value)

			let copied = counter
			counter.bump(1)
			io::print(copied.value)
			io::print(read(counter))

			let root = Node{value: 10, parent: maybe::none()}
			let child = Node{value: 11, parent: maybe::some(root)}
			match child.parent {
				parent => io::print(parent.value),
				_ => io::print("none")
			}
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"value=1",
		"7",
		"7",
		"8",
		"10",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunTraitObjectDispatch(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		trait Speaks {
			fn speak() Str
			fn label(prefix: Str, count: Int) Str
		}

		struct Dog {
			name: Str
		}

		struct Robot {
			id: Int
		}

		struct Kennel {
			resident: Speaks
		}

		impl Speaks for Dog {
			fn speak() Str {
				"{self.name} says hi"
			}

			fn label(prefix: Str, count: Int) Str {
				"{prefix} {self.name} {count}"
			}
		}

		impl Speaks for Robot {
			fn speak() Str {
				"unit {self.id}"
			}

			fn label(prefix: Str, count: Int) Str {
				"{prefix} unit {self.id} {count}"
			}
		}

		fn describe(speaker: Speaks) Str {
			speaker.speak()
		}

		fn tagged(speaker: Speaks) Str {
			speaker.label("tag", 2)
		}

		fn main() {
			io::print(describe(Dog{name: "Ada"}))
			io::print(describe(Robot{id: 7}))
			io::print(tagged(Dog{name: "Ada"}))

			let kennel = Kennel{resident: Dog{name: "Grace"}}
			io::print(kennel.resident.speak())

			let speakers: [Speaks] = [Dog{name: "Lin"}, Robot{id: 9}]
			io::print(speakers.at(0).speak())
			io::print(speakers.at(1).speak())
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"Ada says hi",
		"unit 7",
		"tag Ada 2",
		"Grace says hi",
		"Lin says hi",
		"unit 9",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunGenericFunctions(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io
		use ard/maybe

		fn identity(value: $T) $T {
			value
		}

		fn singleton(value: $T) [$T] {
			[value]
		}

		fn maybe_value(value: $T) $T? {
			maybe::some(value)
		}

		fn ok_value(value: $T) $T!Str {
			Result::ok(value)
		}

		fn main() {
			io::print(identity<Int>(41) + 1)
			io::print(identity("ard"))
			io::print(singleton<Int>(7).at(0))
			io::print(singleton("zig").at(0))

			match maybe_value<Int>(5) {
				value => io::print(value)
				_ => io::print("none")
			}

			match ok_value<Str>("ok") {
				ok(value) => io::print(value)
				err(message) => io::print(message)
			}
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"42",
		"ard",
		"7",
		"zig",
		"5",
		"ok",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunGenericStructsAndMethods(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		struct Box {
			item: $T
		}

		struct Pair {
			first: $T,
			second: $U
		}

		impl Box {
			fn get() $T {
				self.item
			}

			fn replace(item: $T) Box<$T> {
				Box{item: item}
			}
		}

		fn unwrap(box: Box<$T>) $T {
			box.item
		}

		fn main() {
			let int_box = Box{item: 41}
			io::print(int_box.get() + 1)
			io::print(unwrap<Int>(int_box))

			let str_box = Box{item: "ard"}
			io::print(str_box.get())
			io::print(str_box.replace("zig").item)

			let pair = Pair{first: 7, second: "seven"}
			io::print(pair.first)
			io::print(pair.second)
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"42",
		"41",
		"ard",
		"zig",
		"7",
		"seven",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunLoopSemantics(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		fn main() {
			mut sum = 0
			for value, index in [10, 20, 30] {
				sum = sum + value + index
			}
			io::print(sum)

			for char, index in "ab" {
				io::print("{index}:{char}")
			}

			let map: [Str: Int] = ["a": 1, "b": 2]
			for key, value in map {
				io::print("{key}={value}")
			}

			mut range_sum = 0
			for value, index in 2..4 {
				range_sum = range_sum + value + index
			}
			io::print(range_sum)

			mut c_for_sum = 0
			for mut i = 0; i < 5; i = i + 1 {
				if i == 3 {
					break
				}
				c_for_sum = c_for_sum + i
			}
			io::print(c_for_sum)

			mut count = 0
			while true {
				count = count + 1
				if count == 2 {
					break
				}
			}
			io::print(count)

			mut nested = 0
			for outer in 0..2 {
				for inner in 0..2 {
					if inner == 1 {
						break
					}
					nested = nested + outer + inner
				}
			}
			io::print(nested)
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"63",
		"0:a",
		"1:b",
		"a=1",
		"b=2",
		"12",
		"3",
		"2",
		"3",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunSampleGapRegressions(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io
		use ard/maybe

		struct Book {
			title: Str,
		}

		impl Str::ToString for Book {
			fn to_str() Str {
				"Book: {self.title}"
			}
		}

		struct Left {
			value: Int,
		}

		struct Right {
			value: Int,
		}

		type Side = Left | Right

		fn side_name(side: Side) Str {
			match side {
				Left(_) => "left",
				Right(_) => "right",
			}
		}

		fn main() {
			io::print("Bell\b")
			io::print(Book{title: "Ard"})
			io::print(side_name(Left{value: 1}))
			io::print(maybe::some(3).or(9))

			mut values = [3, 1, 2]
			values.set(0, 4)
			values.sort(fn(a: Int, b: Int) Bool { a < b })
			for value in values {
				io::print(value)
			}
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdout(program, []string{"ard", "run", "--target", "zig", path})
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{
		"Bell\b",
		"Book: Ard",
		"left",
		"3",
		"1",
		"2",
		"4",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunReadLineStdlibFunction(t *testing.T) {
	requireZig(t)
	path := writeTempSource(t, `
		use ard/io

		fn main() {
			io::print(io::read_line().expect("first"))
			io::print(io::read_line().expect("second"))
		}
	`)
	program := lowerFile(t, path)

	stdout, err := runProgramCaptureStdoutWithStdin(program, []string{"ard", "run", "--target", "zig", path}, "one\ntwo\n")
	if err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	want := strings.Join([]string{"one", "two", ""}, "\n")
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
	return runProgramCaptureStdoutWithStdin(program, args, "")
}

func runProgramCaptureStdoutWithStdin(program *air.Program, args []string, stdin string) (string, error) {
	var stdout bytes.Buffer
	oldStdout := os.Stdout
	oldStdin := os.Stdin
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = writePipe
	defer func() {
		os.Stdout = oldStdout
		os.Stdin = oldStdin
	}()

	var stdinRead *os.File
	var stdinWrite *os.File
	if stdin != "" {
		stdinRead, stdinWrite, err = os.Pipe()
		if err != nil {
			return "", err
		}
		if _, err := stdinWrite.WriteString(stdin); err != nil {
			return "", err
		}
		if err := stdinWrite.Close(); err != nil {
			return "", err
		}
		os.Stdin = stdinRead
	}

	runErr := RunProgram(program, args)
	if stdinRead != nil {
		if closeErr := stdinRead.Close(); closeErr != nil {
			return "", closeErr
		}
	}
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

func writeTempSource(t *testing.T, input string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "main.ard")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatalf("write temp source: %v", err)
	}
	return path
}

func requireZig(t *testing.T) {
	t.Helper()
	if zigPathErr != nil {
		t.Skipf("zig not installed: %v", zigPathErr)
	}
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
