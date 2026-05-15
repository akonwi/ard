package gotarget

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
)

type Options struct {
	PackageName string
	ProjectInfo *checker.ProjectInfo
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

func RunProgram(program *air.Program, args []string, projectInfo ...*checker.ProjectInfo) error {
	workspaceDir, err := artifactWorkspace(inputPathFromCLIArgs(args), "run")
	if err != nil {
		return err
	}
	if err := writeProgram(workspaceDir, program, Options{PackageName: "main", ProjectInfo: optionalProjectInfo(projectInfo)}); err != nil {
		return err
	}
	binaryPath := filepath.Join(workspaceDir, "ard-program")
	if err := buildGeneratedProgram(workspaceDir, binaryPath); err != nil {
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

func BuildProgram(program *air.Program, outputPath string, projectInfo ...*checker.ProjectInfo) (string, error) {
	workspaceDir, err := artifactWorkspace(outputPath, "build")
	if err != nil {
		return "", err
	}
	if err := writeProgram(workspaceDir, program, Options{PackageName: "main", ProjectInfo: optionalProjectInfo(projectInfo)}); err != nil {
		return "", err
	}
	if outputPath == "" {
		outputPath = "main"
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	if err := buildGeneratedProgram(workspaceDir, absOutput); err != nil {
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
	if err := writeProjectFFICompanions(dir, program, options.ProjectInfo); err != nil {
		return err
	}
	goMod := "module generated\n\ngo 1.26.0\n"
	if moduleRoot, ok := compilerModuleRoot(); ok {
		goMod += "\nrequire github.com/akonwi/ard v0.0.0\n"
		goMod += fmt.Sprintf("replace github.com/akonwi/ard => %s\n", moduleRoot)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}
	return nil
}

func optionalProjectInfo(projectInfo []*checker.ProjectInfo) *checker.ProjectInfo {
	if len(projectInfo) == 0 {
		return nil
	}
	return projectInfo[0]
}

func inputPathFromCLIArgs(args []string) string {
	if len(args) >= 3 && strings.TrimSpace(args[2]) != "" {
		return args[2]
	}
	return "."
}

func artifactWorkspace(pathHint string, purpose string) (string, error) {
	rootDir, err := artifactRootDir(pathHint)
	if err != nil {
		return "", err
	}
	base := filepath.Join(rootDir, "ard-out", "go", purpose)
	if err := os.RemoveAll(base); err != nil {
		return "", err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	return base, nil
}

func artifactRootDir(pathHint string) (string, error) {
	if strings.TrimSpace(pathHint) == "" {
		return os.Getwd()
	}
	pathHint = filepath.Clean(pathHint)
	absPath, err := filepath.Abs(pathHint)
	if err != nil {
		return "", err
	}
	candidate := absPath
	if info, statErr := os.Stat(absPath); statErr == nil && !info.IsDir() {
		candidate = filepath.Dir(absPath)
	} else if statErr != nil {
		candidate = filepath.Dir(absPath)
	}
	if project, err := checker.FindProjectRoot(candidate); err == nil && strings.TrimSpace(project.RootPath) != "" {
		return project.RootPath, nil
	}
	return candidate, nil
}

func writeProjectFFICompanions(dir string, program *air.Program, projectInfo *checker.ProjectInfo) error {
	if !programUsesProjectFFI(program) {
		return nil
	}
	if projectInfo == nil || strings.TrimSpace(projectInfo.RootPath) == "" {
		return fmt.Errorf("go target uses project externs but project information is unavailable")
	}
	copied := false
	if err := copyProjectFFIFile(filepath.Join(projectInfo.RootPath, "ffi.go"), filepath.Join(dir, "ffi.go")); err != nil {
		return err
	} else if fileExists(filepath.Join(projectInfo.RootPath, "ffi.go")) {
		copied = true
	}
	matches, err := filepath.Glob(filepath.Join(projectInfo.RootPath, "ffi", "*.go"))
	if err != nil {
		return err
	}
	for _, sourcePath := range matches {
		name := "ffi_" + filepath.Base(sourcePath)
		if err := copyProjectFFIFile(sourcePath, filepath.Join(dir, name)); err != nil {
			return err
		}
		copied = true
	}
	if !copied {
		return fmt.Errorf("go target uses project externs but no project Go FFI companion was found at %s or %s", filepath.Join(projectInfo.RootPath, "ffi.go"), filepath.Join(projectInfo.RootPath, "ffi", "*.go"))
	}
	return nil
}

func copyProjectFFIFile(sourcePath, destPath string) error {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read project Go FFI companion %s: %w", sourcePath, err)
	}
	if err := os.WriteFile(destPath, content, 0o644); err != nil {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func programUsesProjectFFI(program *air.Program) bool {
	if program == nil {
		return false
	}
	for _, ext := range program.Externs {
		if !externModuleIsStdlib(program, ext) {
			return true
		}
	}
	return false
}

func externModuleIsStdlib(program *air.Program, ext air.Extern) bool {
	if program == nil || int(ext.Module) < 0 || int(ext.Module) >= len(program.Modules) {
		return false
	}
	return strings.HasPrefix(program.Modules[ext.Module].Path, "ard/")
}

func compilerModuleRoot() (string, bool) {
	_, file, _, ok := goruntime.Caller(0)
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
	cmd := exec.Command("go", "build", "-tags=goexperiment.jsonv2", "-mod=mod", "-o", outputPath, ".")
	cmd.Dir = dir
	cmd.Env = appendGoExperimentJSONv2(os.Environ())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func appendGoExperimentJSONv2(env []string) []string {
	for i, entry := range env {
		if !strings.HasPrefix(entry, "GOEXPERIMENT=") {
			continue
		}
		current := strings.TrimPrefix(entry, "GOEXPERIMENT=")
		if current == "" {
			env[i] = "GOEXPERIMENT=jsonv2"
			return env
		}
		for _, experiment := range strings.Split(current, ",") {
			if experiment == "jsonv2" {
				return env
			}
		}
		env[i] = entry + ",jsonv2"
		return env
	}
	return append(env, "GOEXPERIMENT=jsonv2")
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
	if rootID, ok := findRootFunction(program); ok {
		return rootID, nil
	}
	return air.NoFunction, fmt.Errorf("AIR program has no entry or script function")
}

func findRootFunction(program *air.Program) (air.FunctionID, bool) {
	if program == nil {
		return air.NoFunction, false
	}
	if program.Entry != air.NoFunction {
		return program.Entry, true
	}
	if program.Script != air.NoFunction {
		return program.Script, true
	}
	return air.NoFunction, false
}
