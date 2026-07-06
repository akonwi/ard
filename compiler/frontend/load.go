package frontend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

type LoadResult struct {
	Module      checker.Module
	ProjectInfo *checker.ProjectInfo
}

func LoadModule(inputPath string) (*LoadResult, error) {
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
	if err := checker.VerifyDependencies(workingDir); err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(workingDir, inputPath)
	if err != nil {
		relPath = inputPath
	}

	projectInfo := moduleResolver.GetProjectInfo()
	goResolver := checker.NewGoPackagesResolver(projectInfo.RootPath, projectInfo.Go.BuildTags)
	c := checker.New(relPath, program, moduleResolver, checker.CheckOptions{GoResolver: goResolver})
	c.Check()
	if c.HasErrors() {
		for _, diagnostic := range c.Diagnostics() {
			fmt.Println(diagnostic)
		}
		return nil, fmt.Errorf("type errors")
	}

	return &LoadResult{
		Module:      c.Module(),
		ProjectInfo: projectInfo,
	}, nil
}
