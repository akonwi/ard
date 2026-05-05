package gotarget

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/akonwi/ard/air"
)

type Options struct {
	PackageName string
}

func GenerateSources(program *air.Program, options Options) (map[string][]byte, error) {
	generated, err := lowerProgram(program, options)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(generated))
	for name, file := range generated {
		source, err := renderFile(file)
		if err != nil {
			return nil, err
		}
		out[name] = source
	}
	return out, nil
}

func RunProgram(program *air.Program, args []string) error {
	tempDir, err := os.MkdirTemp("", "ard-go-target-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	if err := writeProgram(tempDir, program, Options{PackageName: "main"}); err != nil {
		return err
	}
	binaryPath := filepath.Join(tempDir, "ard-program")
	if err := buildGeneratedProgram(tempDir, binaryPath); err != nil {
		return err
	}
	cmd := exec.Command(binaryPath, programArgs(args)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func BuildProgram(program *air.Program, outputPath string) (string, error) {
	tempDir, err := os.MkdirTemp("", "ard-go-target-build-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)
	if err := writeProgram(tempDir, program, Options{PackageName: "main"}); err != nil {
		return "", err
	}
	if outputPath == "" {
		outputPath = "main"
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	if err := buildGeneratedProgram(tempDir, absOutput); err != nil {
		return "", err
	}
	return absOutput, nil
}

func writeProgram(dir string, program *air.Program, options Options) error {
	sources, err := GenerateSources(program, options)
	if err != nil {
		return err
	}
	for name, source := range sources {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, source, 0o644); err != nil {
			return err
		}
	}
	goMod := "module generated\n\ngo 1.24\n"
	if moduleRoot, ok := compilerModuleRoot(); ok {
		goMod += "\nrequire github.com/akonwi/ard v0.0.0\n"
		goMod += fmt.Sprintf("replace github.com/akonwi/ard => %s\n", moduleRoot)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}
	return nil
}

func compilerModuleRoot() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", false
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	if strings.TrimSpace(root) == "" {
		return "", false
	}
	return root, true
}

func buildGeneratedProgram(dir string, outputPath string) error {
	cmd := exec.Command("go", "build", "-mod=mod", "-o", outputPath, ".")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func programArgs(args []string) []string {
	if len(args) <= 3 {
		return nil
	}
	return append([]string(nil), args[3:]...)
}

func defaultPackageName(name string) string {
	if name == "" {
		return "main"
	}
	return name
}

func rootFunction(program *air.Program) (air.FunctionID, error) {
	if program.Entry != air.NoFunction {
		return program.Entry, nil
	}
	if program.Script != air.NoFunction {
		return program.Script, nil
	}
	return air.NoFunction, fmt.Errorf("AIR program has no entry or script function")
}
