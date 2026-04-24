package frontend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

type LoadResult struct {
	Module      checker.Module
	ProjectInfo *checker.ProjectInfo
}

func ResolveTarget(inputPath, requestedTarget string) (string, error) {
	if requestedTarget != "" {
		return requestedTarget, nil
	}
	inputDir := filepath.Dir(inputPath)
	if inputDir == "" {
		inputDir = "."
	}
	project, err := checker.FindProjectRoot(inputDir)
	if err != nil {
		return "", err
	}
	return backend.ParseTarget(project.Target)
}

func LoadModule(inputPath string, target string) (*LoadResult, error) {
	sourceCode, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s - %v", inputPath, err)
	}

	result := parse.Parse(sourceCode, inputPath)
	if len(result.Errors) > 0 {
		result.PrintErrors()
		return nil, fmt.Errorf("parse errors")
	}
	program := result.Program

	workingDir := filepath.Dir(inputPath)
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		return nil, fmt.Errorf("error initializing module resolver: %w", err)
	}

	relPath, err := filepath.Rel(workingDir, inputPath)
	if err != nil {
		relPath = inputPath
	}

	c := checker.New(relPath, program, moduleResolver, checker.CheckOptions{Target: target})
	c.Check()
	if c.HasErrors() {
		for _, diagnostic := range c.Diagnostics() {
			fmt.Println(diagnostic)
		}
		return nil, fmt.Errorf("type errors")
	}

	return &LoadResult{
		Module:      c.Module(),
		ProjectInfo: moduleResolver.GetProjectInfo(),
	}, nil
}
