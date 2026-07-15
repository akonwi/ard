package frontend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/diagnostics"
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

	projectInfo := moduleResolver.GetProjectInfo()
	relPath := inputPath
	if absInput, absErr := filepath.Abs(inputPath); absErr == nil {
		if projectRelative, relErr := filepath.Rel(projectInfo.RootPath, absInput); relErr == nil {
			relPath = projectRelative
		}
	}

	// The checker primes the resolver with the program's whole Go import
	// closure before binding imports, so all Go types share a single
	// go/types universe (ADR 0044).
	goResolver := checker.NewGoPackagesResolver(projectInfo.RootPath, projectInfo.Go.BuildTags)
	c := checker.New(relPath, program, moduleResolver, checker.CheckOptions{GoResolver: goResolver})
	c.Check()
	if c.HasErrors() {
		displayRoot, err := os.Getwd()
		if err != nil {
			displayRoot = projectInfo.RootPath
		}
		if err := diagnostics.RenderRelative(os.Stdout, c.Diagnostics(), projectInfo.RootPath, displayRoot); err != nil {
			return nil, fmt.Errorf("render diagnostics: %w", err)
		}
		return nil, fmt.Errorf("type errors")
	}

	return &LoadResult{
		Module:      c.Module(),
		ProjectInfo: projectInfo,
	}, nil
}
