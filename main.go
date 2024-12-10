package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akonwi/ard/ast"
	"github.com/akonwi/ard/javascript"
	ts_ard "github.com/akonwi/tree-sitter-ard/bindings/go"
)

func main() {
	buildCmd := flag.NewFlagSet("build", flag.ExitOnError)

	if len(os.Args) < 2 {
		fmt.Println("Please provide a command")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build":
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
		diagnostics := astParser.GetDiagnostics()
		if len(diagnostics) > 0 {
			for _, diagnostic := range diagnostics {
				fmt.Printf(
					"[%d, %d] %s",
					diagnostic.Range.StartPoint.Row,
					diagnostic.Range.StartPoint.Column,
					diagnostic.Msg,
				)
			}
			os.Exit(1)
		}

		jsSource := javascript.GenerateJS(ast)

		buildDir := "./build"
		err = os.MkdirAll(buildDir, 0755)
		if err != nil {
			fmt.Printf("Error creating build directory: %v\n", err)
			os.Exit(1)
		}

		filename := filepath.Base(strings.TrimSuffix(inputPath, filepath.Ext(inputPath))) + ".js"
		outputPath := filepath.Join(buildDir, filename)

		err = os.WriteFile(outputPath, []byte(jsSource), 0644)
		if err != nil {
			fmt.Printf("Error writing file %s - %v\n", outputPath, err)
			os.Exit(1)
		}

		fmt.Printf("Successfully built to %s\n", outputPath)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
