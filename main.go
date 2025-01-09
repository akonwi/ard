package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/akonwi/ard/internal/ast"
	"github.com/akonwi/ard/internal/checker"
	"github.com/akonwi/ard/internal/vm"
	ts_ard "github.com/akonwi/tree-sitter-ard/bindings/go"
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

		tree, err := ts_ard.Parse(sourceCode)
		if err != nil {
			fmt.Println("Error parsing source code with tree-sitter")
			os.Exit(1)
		}

		astParser := ast.NewParser(sourceCode, tree)
		ast, err := astParser.Parse()
		if err != nil {
			fmt.Printf("Error parsing tree: %v\n", err)
			os.Exit(1)
			return
		}

		program, diagnostics := checker.Check(ast)
		if len(diagnostics) > 0 {
			for _, diagnostic := range diagnostics {
				fmt.Println(diagnostic)
			}
			os.Exit(1)
		}

		vm := vm.New(&program)
		if _, err := vm.Run(); err != nil {
			fmt.Printf("Runtime error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
