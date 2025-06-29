package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm"
)

func main() {
	buildCmd := flag.NewFlagSet("build", flag.ExitOnError)

	if len(os.Args) < 2 {
		fmt.Println("Please provide a command")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		buildCmd.Parse(os.Args[2:])

		if buildCmd.NArg() < 1 {
			fmt.Println("Expected filepath argument")
			os.Exit(1)
		}

		inputPath := buildCmd.Arg(0)
		sourceCode, err := os.ReadFile(inputPath)
		if err != nil {
			fmt.Printf("Error reading file %s - %v\n", inputPath, err)
			os.Exit(1)
		}

		ast, err := ast.Parse(sourceCode)
		if err != nil {
			log.Fatalf("Error parsing code: %v\n", err)
			return
		}

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

		module, diagnostics := checker.Check(ast, moduleResolver, relPath)
		if len(diagnostics) > 0 {
			for _, diagnostic := range diagnostics {
				fmt.Println(diagnostic)
			}
			os.Exit(1)
		}

		if err := vm.Run(module.Program()); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

	default:
		log.Fatalf("Unknown command: %s\n", os.Args[1])
	}
}
