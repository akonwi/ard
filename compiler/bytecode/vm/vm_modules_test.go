package vm

import (
	"os"
	"path/filepath"
	"testing"
)

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
