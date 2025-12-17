package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/version"
	"github.com/akonwi/ard/vm"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide a command")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(version.Get())
		os.Exit(0)
	case "check":
		{
			if len(os.Args) < 3 {
				fmt.Println("Expected filepath argument")
				os.Exit(1)
			}

			inputPath := os.Args[2]
			if !check(inputPath) {
				os.Exit(1)
			}

			fmt.Println("✅ No errors found")
			os.Exit(0)
		}
	case "run":
		{
			if len(os.Args) < 3 {
				fmt.Println("Expected filepath argument")
				os.Exit(1)
			}

			inputPath := os.Args[2]
			sourceCode, err := os.ReadFile(inputPath)
			if err != nil {
				fmt.Printf("Error reading file %s - %v\n", inputPath, err)
				os.Exit(1)
			}

			result := ast.Parse(sourceCode, inputPath)
			if len(result.Errors) > 0 {
				result.PrintErrors()
				os.Exit(1)
			}
			ast := result.Program

			workingDir := filepath.Dir(inputPath)
			moduleResolver, err := checker.NewModuleResolver(workingDir)
			if err != nil {
				log.Fatalf("Error initializing module resolver: %v\n", err)
			}

			// Get relative path for diagnostics
			relPath, err := filepath.Rel(workingDir, inputPath)
			if err != nil {
				relPath = inputPath // fallback to absolute path
			}

			c := checker.New(relPath, ast, moduleResolver)
			c.Check()
			if c.HasErrors() {
				for _, diagnostic := range c.Diagnostics() {
					fmt.Println(diagnostic)
				}
				os.Exit(1)
			}

			g := vm.NewRuntime(c.Module())
			if err := g.Run("main"); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func check(inputPath string) bool {
	sourceCode, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Printf("Error reading file %s - %v\n", inputPath, err)
		return false
	}

	result := ast.Parse(sourceCode, inputPath)
	if len(result.Errors) > 0 {
		result.PrintErrors()
		return false
	}
	ast := result.Program

	workingDir := filepath.Dir(inputPath)
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		log.Fatalf("Error initializing module resolver: %v\n", err)
	}

	// Get relative path for diagnostics
	relPath, err := filepath.Rel(workingDir, inputPath)
	if err != nil {
		relPath = inputPath // fallback to absolute path
	}

	c := checker.New(relPath, ast, moduleResolver)
	c.Check()
	if c.HasErrors() {
		for _, diagnostic := range c.Diagnostics() {
			fmt.Println(diagnostic)
		}
		return false
	}

	return true
}
