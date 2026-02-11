package vm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func runBytecodeInDir(t *testing.T, dir, filename, input string) any {
	t.Helper()

	result := parse.Parse([]byte(input), filename)
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	resolver, err := checker.NewModuleResolver(dir)
	if err != nil {
		t.Fatalf("Failed to init module resolver: %v", err)
	}

	c := checker.New(filename, result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Diagnostics found: %v", c.Diagnostics())
	}

	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		t.Fatalf("Verify error: %v", err)
	}

	vm := New(program)
	res, err := vm.Run("main")
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	if res == nil {
		return nil
	}
	return res.GoValue()
}

func TestBytecodeVMParityCoreExpressions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "reassigning variables",
			input: `
				mut val = 1
				val = 2
				val = 3
				val
			`,
			want: 3,
		},
		{
			name:  "unary not",
			input: `not true`,
			want:  false,
		},
		{
			name:  "unary negative float",
			input: `-20.1`,
			want:  -20.1,
		},
		{
			name:  "arithmetic precedence",
			input: `30 + (20 * 4)`,
			want:  110,
		},
		{
			name:  "chained comparisons",
			input: `200 <= 250 <= 300`,
			want:  true,
		},
		{
			name: "if/else-if/else",
			input: `
				let is_on = false
				mut result = ""
				if is_on { result = "then" }
				else if result.size() == 0 { result = "else if" }
				else { result = "else" }
				result
			`,
			want: "else if",
		},
		{
			name: "inline block expression",
			input: `
				let value = {
					let x = 10
					let y = 32
					x + y
				}
				value
			`,
			want: 42,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeVMParityTypeAPIs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{name: "Int.to_str", input: `100.to_str()`, want: "100"},
		{name: "Int::from_str", input: `Int::from_str("100")`, want: 100},
		{name: "Float::from_int", input: `Float::from_int(100)`, want: 100.0},
		{name: "Float.to_int", input: `5.9.to_int()`, want: 5},
		{name: "Bool.to_str", input: `true.to_str()`, want: "true"},
		{name: "Str.replace_all", input: `"hello world hello world".replace_all("world", "universe")`, want: "hello universe hello universe"},
		{name: "Map::new", input: `
			mut ages = Map::new<Int>()
			ages.set("Alice", 25)
			ages.get("Alice").or(0)
		`, want: 25},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeVMParityEnumsUnions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{
			name: "enum to int comparison",
			input: `
				enum Status { active, inactive, pending }
				let status = Status::active
				status == 0
			`,
			want: true,
		},
		{
			name: "enum explicit value",
			input: `
				enum HttpStatus {
					Ok = 200,
					Created = 201,
					Not_Found = 404
				}
				HttpStatus::Ok
			`,
			want: 200,
		},
		{
			name: "enum equality",
			input: `
				enum Direction { Up, Down, Left, Right }
				let dir1 = Direction::Up
				let dir2 = Direction::Down
				dir1 == dir2
			`,
			want: false,
		},
		{
			name: "union matching",
			input: `
				type Printable = Str | Int | Bool
				fn print(p: Printable) Str {
				  match p {
					  Str(str) => str,
						Int(int) => int.to_str(),
						_ => "boolean value"
					}
				}
				print(20)
			`,
			want: "20",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runBytecode(t, test.input); got != test.want {
				t.Fatalf("Expected %v, got %v", test.want, got)
			}
		})
	}
}

func TestBytecodeVMParityPanicVoidAndFunctionVariables(t *testing.T) {
	expectBytecodeRuntimeError(t, "This is an error", `
		fn speak() {
		  panic("This is an error")
		}
		speak()
		1 + 1
	`)

	runBytecodeRaw(t, `
		let unit = ()
		unit

		fn void() Void!Str {
			if not 42 == 42 {
				Result::err("42 should equal 42")
			}
			Result::ok(())
		}
		void()
	`)

	if got := runBytecode(t, `
		let multiply = fn(a: Int, b: Int) Int {
			a * b
		}
		multiply(3, 4)
	`); got != 12 {
		t.Fatalf("Expected 12, got %v", got)
	}
}

func TestBytecodeVMParityModuleIntegration(t *testing.T) {
	t.Run("user module function call", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "ard_bytecode_module_test_")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\""), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tempDir, "math.ard"), []byte("fn add(a: Int, b: Int) Int { a + b }"), 0644); err != nil {
			t.Fatal(err)
		}

		out := runBytecodeInDir(t, tempDir, "main.ard", `use test_project/math
math::add(10, 20)`)
		if out != 30 {
			t.Fatalf("Expected 30, got %v", out)
		}
	})

	t.Run("function variable from module", func(t *testing.T) {
		t.Skip("TODO(bytecode): module-level function variables via module namespace parity")

		tempDir, err := os.MkdirTemp("", "ard_bytecode_func_var_test_")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\""), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tempDir, "utils.ard"), []byte(`let add_one = fn(x: Int) Int { x + 1 }`), 0644); err != nil {
			t.Fatal(err)
		}

		out := runBytecodeInDir(t, tempDir, "main.ard", `use test_project/utils
utils::add_one(5)`)
		if out != 6 {
			t.Fatalf("Expected 6, got %v", out)
		}
	})

	t.Run("function variable call directly", func(t *testing.T) {
		t.Skip("TODO(bytecode): module symbol emission for function-valued variables")

		tempDir, err := os.MkdirTemp("", "ard_bytecode_func_call_test_")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"test_project\""), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tempDir, "utils.ard"), []byte(`let double = fn(x: Int) Int { x * 2 }`), 0644); err != nil {
			t.Fatal(err)
		}

		out := runBytecodeInDir(t, tempDir, "main.ard", `use test_project/utils
let f = utils::double
f(10)`)
		if out != 20 {
			t.Fatalf("Expected 20, got %v", out)
		}
	})
}
