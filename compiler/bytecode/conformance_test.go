package bytecode_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/akonwi/ard/bytecode"
	bytecodevm "github.com/akonwi/ard/bytecode/vm"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/vm"
)

func TestBytecodeConformanceSamples(t *testing.T) {
	samples, err := listConformanceSamples()
	if err != nil {
		t.Fatalf("Failed to list samples: %v", err)
	}
	for _, path := range samples {
		t.Run(filepath.Base(path), func(t *testing.T) {
			module, err := loadModuleForConformance(path)
			if err != nil {
				t.Fatalf("Load error: %v", err)
			}
			hasMain := moduleHasMain(module)

			interpOut, err := captureStdout(func() error {
				g := vm.NewRuntime(module)
				if hasMain {
					return g.Run("main")
				}
				_, err := g.Interpret()
				return err
			})
			if err != nil {
				t.Fatalf("Interpreter error: %v", err)
			}

			bytecodeOut, err := captureStdout(func() error {
				program, err := bytecode.NewEmitter().EmitProgram(module)
				if err != nil {
					return err
				}
				if err := bytecode.VerifyProgram(program); err != nil {
					return err
				}
				_, err = bytecodevm.New(program).Run("main")
				return err
			})
			if err != nil {
				t.Fatalf("Bytecode error: %v", err)
			}

			if interpOut != bytecodeOut {
				t.Fatalf("Output mismatch\n--- interp ---\n%s\n--- bytecode ---\n%s", interpOut, bytecodeOut)
			}
		})
	}
}

func moduleHasMain(module checker.Module) bool {
	prog := module.Program()
	if prog == nil {
		return false
	}
	for i := range prog.Statements {
		stmt := prog.Statements[i]
		if def, ok := stmt.Expr.(*checker.FunctionDef); ok {
			if def.Name == "main" {
				return true
			}
		}
	}
	return false
}

func listConformanceSamples() ([]string, error) {
	root, err := findModuleRootFrom("..")
	if err != nil {
		return nil, err
	}
	samplesDir := filepath.Join(root, "samples")
	entries, err := filepath.Glob(filepath.Join(samplesDir, "*.ard"))
	if err != nil {
		return nil, err
	}
	filter := os.Getenv("ARD_CONFORMANCE_SAMPLE")
	filtered := make([]string, 0, len(entries))
	for _, path := range entries {
		base := filepath.Base(path)
		if filter != "" && base != filter {
			continue
		}
		if base == "maps.ard" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		content := string(data)
		if strings.Contains(content, "read_line") {
			continue
		}
		if strings.Contains(content, "ard/http") || strings.Contains(content, "http::") {
			continue
		}
		filtered = append(filtered, path)
	}
	sort.Strings(filtered)
	return filtered, nil
}

func loadModuleForConformance(path string) (checker.Module, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := parse.Parse(source, path)
	if len(result.Errors) > 0 {
		result.PrintErrors()
		return nil, fmt.Errorf("parse errors")
	}
	workingDir := filepath.Dir(path)
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		return nil, err
	}
	relPath, err := filepath.Rel(workingDir, path)
	if err != nil {
		relPath = path
	}
	c := checker.New(relPath, result.Program, moduleResolver)
	c.Check()
	if c.HasErrors() {
		return nil, fmt.Errorf("type errors: %v", c.Diagnostics())
	}
	return c.Module(), nil
}

func captureStdout(fn func() error) (string, error) {
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = writer
	readCh := make(chan string, 1)
	readErrCh := make(chan error, 1)
	go func() {
		data, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			readErrCh <- readErr
			return
		}
		readCh <- string(data)
	}()
	var runErr error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				runErr = fmt.Errorf("panic: %v", rec)
			}
		}()
		runErr = fn()
	}()
	_ = writer.Close()
	os.Stdout = old
	select {
	case readErr := <-readErrCh:
		return "", readErr
	case output := <-readCh:
		return output, runErr
	}
}

func findModuleRootFrom(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found")
}
