package go_backend

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/frontend"
	"github.com/akonwi/ard/go_backend/lowering"
)

func writeGeneratedProject(generatedDir string, project *checker.ProjectInfo, entrypoint checker.Module) error {
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		return err
	}
	moduleRoot, err := compilerModuleRoot()
	if err != nil {
		return err
	}
	goMod := fmt.Sprintf("module %s\n\ngo %s\n\nrequire %s v0.0.0\n\nreplace %s => %s\n", project.ProjectName, generatedGoVersion, ardModulePath, ardModulePath, filepath.Clean(moduleRoot))
	if err := os.WriteFile(filepath.Join(generatedDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}
	if goSum, err := os.ReadFile(filepath.Join(moduleRoot, "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(generatedDir, "go.sum"), goSum, 0o644); err != nil {
			return err
		}
	}

	source, err := compileModuleSource(entrypoint, "main", true, project.ProjectName)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(generatedDir, "main.go"), source, 0o644); err != nil {
		return err
	}

	written := map[string]struct{}{}
	mods, err := requiredImportedModules(entrypoint, project.ProjectName)
	if err != nil {
		return err
	}
	for _, mod := range mods {
		if err := writeImportedModule(generatedDir, project.ProjectName, mod, written); err != nil {
			return err
		}
	}
	return nil
}

func requiredImportedModules(module checker.Module, projectName string) ([]checker.Module, error) {
	if module == nil || module.Program() == nil {
		return nil, nil
	}
	irModule, err := lowering.LowerModuleToBackendIR(module, packageNameForModulePath(module.Path()), false, projectName)
	if err != nil {
		return nil, err
	}
	mods := make([]checker.Module, 0, len(irModule.ImportedModulePaths))
	for _, path := range irModule.ImportedModulePaths {
		mod, ok := module.Program().Imports[path]
		if !ok {
			continue
		}
		mods = append(mods, mod)
	}
	return mods, nil
}

func writeImportedModule(generatedDir, projectName string, module checker.Module, written map[string]struct{}) error {
	if module == nil {
		return nil
	}
	if _, ok := written[module.Path()]; ok {
		return nil
	}
	written[module.Path()] = struct{}{}

	source, err := compilePackageSource(module, projectName)
	if err != nil {
		return err
	}
	outputPath, err := generatedPathForModule(generatedDir, projectName, module.Path())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, source, 0o644); err != nil {
		return err
	}
	mods, err := requiredImportedModules(module, projectName)
	if err != nil {
		return err
	}
	for _, mod := range mods {
		if err := writeImportedModule(generatedDir, projectName, mod, written); err != nil {
			return err
		}
	}
	return nil
}

func loadModule(inputPath string) (checker.Module, *checker.ProjectInfo, error) {
	result, err := frontend.LoadModule(inputPath, backend.TargetGo)
	if err != nil {
		return nil, nil, err
	}
	return result.Module, result.ProjectInfo, nil
}

func compilerModuleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to determine compiler module root")
	}
	return filepath.Dir(filepath.Dir(file)), nil
}
